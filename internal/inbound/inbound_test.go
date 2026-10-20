package inbound

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/dnsbl"
	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/spam"
	"github.com/Anonyfox/lfr/internal/store"
)

type env struct {
	addr string
	mu   sync.Mutex
	got  []*Envelope
	p    float64
	dns  *dnsxtest.Fake
	cfg  config.Config
}

func start(t *testing.T, earlyTalker bool) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{dns: dnsxtest.New(), cfg: config.Config{DB: db}}
	e.cfg.Set(ctx, "smtp.early_talker", map[bool]string{true: "true", false: "false"}[earlyTalker])
	e.cfg.Set(ctx, "spam.dnsbl", "")
	e.cfg.Set(ctx, "smtp.max_conns_per_ip", "3")
	e.dns.SetTXT("_dmarc.strict.org", "v=DMARC1; p=reject")
	s := &Server{
		Hostname: func(context.Context) string { return "mail.example.com" },
		Config:   e.cfg, DNS: e.dns, DNSBL: &dnsbl.Checker{DNS: e.dns},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
		Accept: func(_ context.Context, rcpt string) (bool, error) {
			return strings.HasSuffix(strings.ToLower(rcpt), "@example.com") && !strings.HasPrefix(rcpt, "nobody"), nil
		},
		System: func(rcpt string) bool { return strings.HasPrefix(rcpt, "bounce+") },
		Classify: func(context.Context, spam.Input) spam.Verdict {
			e.mu.Lock()
			defer e.mu.Unlock()
			return spam.Verdict{P: e.p}
		},
		Deliver: func(_ context.Context, en *Envelope) error {
			e.mu.Lock()
			e.got = append(e.got, en)
			e.mu.Unlock()
			return nil
		},
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e.addr = l.Addr().String()
	go s.Serve(ctx, l)
	return e
}

func (e *env) send(from string, rcpts []string, data string) error {
	c, err := smtp.Dial(e.addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello("sender.example.org"); err != nil {
		return err
	}
	return c.SendMail(from, rcpts, strings.NewReader(data))
}

const body = "From: Ann <ann@sender.example.org>\r\nTo: jo@example.com\r\nSubject: hi\r\nMessage-ID: <1@sender.example.org>\r\n" +
	"Authentication-Results: mail.example.com; dmarc=pass header.from=forged\r\n\r\nhello\r\n"

func TestAcceptsAndTraces(t *testing.T) {
	e := start(t, false)
	if err := e.send("ann@sender.example.org", []string{"jo@example.com"}, body); err != nil {
		t.Fatal(err)
	}
	if len(e.got) != 1 {
		t.Fatal("nothing delivered")
	}
	raw := string(e.got[0].Raw)
	if !strings.HasPrefix(raw, "Return-Path: <ann@sender.example.org>\r\nAuthentication-Results: mail.example.com; spf=none") || !strings.Contains(raw, "Received: from sender.example.org ([127.0.0.1])") {
		t.Fatalf("trace:\n%s", raw)
	}
	if strings.Contains(raw, "header.from=forged") {
		t.Fatal("forged Authentication-Results kept")
	}
	if e.got[0].Parsed.Subject != "hi" || e.got[0].From != "ann@sender.example.org" {
		t.Fatalf("%+v", e.got[0])
	}
}

func TestRejections(t *testing.T) {
	e := start(t, false)
	err := e.send("a@x.org", []string{"nobody@example.com"}, body)
	if se, ok := err.(*smtp.SMTPError); !ok || se.Code != 550 {
		t.Fatalf("unknown rcpt: %v", err)
	}
	exe, _ := msg.Compose(msg.Draft{From: msg.Address{Email: "a@x.org"}, To: []msg.Address{{Email: "jo@example.com"}}, Subject: "invoice", Text: "see attached",
		Files: []msg.File{{Name: "invoice.pdf.exe", Data: []byte("MZ")}}})
	if err := e.send("a@x.org", []string{"jo@example.com"}, string(exe)); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("executable: %v", err)
	}
	spoof := strings.Replace(body, "ann@sender.example.org", "ceo@strict.org", 1)
	if err := e.send("ceo@strict.org", []string{"jo@example.com"}, spoof); err == nil || !strings.Contains(err.Error(), "DMARC") {
		t.Fatalf("dmarc reject: %v", err)
	}
	e.mu.Lock()
	e.p = 0.99
	e.mu.Unlock()
	if err := e.send("a@x.org", []string{"jo@example.com"}, body); err == nil || !strings.Contains(err.Error(), "spam") {
		t.Fatalf("spam: %v", err)
	}
	// Bounces skip spam scoring.
	if err := e.send("", []string{"bounce+tok@example.com"}, body); err != nil {
		t.Fatalf("system address: %v", err)
	}
	if len(e.got) != 1 {
		t.Fatalf("delivered %d", len(e.got))
	}
}

func TestDictionaryAttackDropsConnection(t *testing.T) {
	e := start(t, false)
	c, _ := smtp.Dial(e.addr)
	defer c.Close()
	c.Hello("bot.example")
	c.Mail("a@x.org", nil)
	var last error
	for i := 0; i < 6; i++ {
		last = c.Rcpt("nobody"+string(rune('a'+i))+"@example.com", nil)
	}
	if last == nil {
		t.Fatal("still accepting")
	}
	if err := c.Noop(); err == nil {
		t.Fatal("connection survived address guessing")
	}
}

func TestEarlyTalkerAndLimits(t *testing.T) {
	e := start(t, true)
	conn, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("EHLO rude\r\n"))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	conn.Close()
	if !strings.HasPrefix(line, "554") {
		t.Fatalf("early talker got %q", line)
	}
	// A patient client still gets in.
	if err := e.send("a@x.org", []string{"jo@example.com"}, body); err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	for i := 0; i < 3; i++ {
		c, _ := net.Dial("tcp", e.addr)
		bufio.NewReader(c).ReadString('\n') // the greeting: the server counts it now
		conns = append(conns, c)
	}
	c, _ := net.Dial("tcp", e.addr)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, _ = bufio.NewReader(c).ReadString('\n')
	if !strings.HasPrefix(line, "421") {
		t.Fatalf("fourth connection got %q", line)
	}
	for _, c := range conns {
		c.Close()
	}
}

func TestLoopsSpoofsAndTemporaryDMARC(t *testing.T) {
	e := start(t, false)
	code := func(err error) int {
		if se, ok := err.(*smtp.SMTPError); ok {
			return se.Code
		}
		return 0
	}
	looping := strings.Repeat("Received: from a by b; Tue, 20 Oct 2026 10:00:00 +0000\r\n", maxHops) + body
	if err := e.send("ann@sender.example.org", []string{"jo@example.com"}, looping); code(err) != 554 {
		t.Fatalf("loop: %v", err)
	}
	two := "From: x@attacker.example\r\n" + body
	if err := e.send("x@attacker.example", []string{"jo@example.com"}, two); code(err) != 550 {
		t.Fatalf("two From: %v", err)
	}
	// A DNS timeout for a p=reject domain defers instead of refusing.
	e.dns.Fail["strict.org"] = true
	spoof := strings.Replace(body, "ann@sender.example.org", "ceo@strict.org", 1)
	e.dns.SetTXT("_dmarc.strict.org", "v=DMARC1; p=reject")
	if err := e.send("ceo@strict.org", []string{"jo@example.com"}, spoof); code(err) != 451 {
		t.Fatalf("temperror: %v", err)
	}
}

func TestCRLFAndForgedResults(t *testing.T) {
	if got := string(CRLF([]byte("a\nb\rc\r\nd"))); got != "a\r\nb\r\nc\r\nd" {
		t.Fatalf("%q", got)
	}
	raw := []byte("Return-Path: <x@evil>\r\nAuthentication-Results:\r\n mail.example.com; dkim=pass\r\nAuthentication-Results: other.host; spf=pass\r\nSubject: s\r\n\r\nbody mail.example.com\r\n")
	got := string(stripForged(raw, "mail.example.com"))
	if strings.Contains(got, "Return-Path") || strings.Contains(got, "dkim=pass") || !strings.Contains(got, "other.host") || !strings.HasSuffix(got, "body mail.example.com\r\n") {
		t.Fatalf("%q", got)
	}
}

// After normalizing, no bare CR or LF is left, and stripping never panics.
func FuzzCRLF(f *testing.F) {
	f.Add([]byte("a\nb\rc\r\n\r\nd"))
	f.Add([]byte("Authentication-Results: mail.example.com; x\n\nbody"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		out := CRLF(raw)
		for i, c := range out {
			if c == '\n' && (i == 0 || out[i-1] != '\r') || c == '\r' && (i+1 == len(out) || out[i+1] != '\n') {
				t.Fatalf("bare line ending at %d in %q", i, out)
			}
		}
		stripForged(out, "mail.example.com")
	})
}
