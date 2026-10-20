package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/inbound"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/msg"
)

type sink struct {
	mu   sync.Mutex
	msgs []sent
	fail func(rcpt string) error
}

type sent struct {
	from, rcpt string
	raw        []byte
}

func (s *sink) Send(_ context.Context, from, rcpt string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		if err := s.fail(rcpt); err != nil {
			return err
		}
	}
	s.msgs = append(s.msgs, sent{from, rcpt, append([]byte(nil), raw...)})
	return nil
}
func (s *sink) Close()       {}
func (s *sink) Name() string { return "sink" }

func (s *sink) wait(t *testing.T, rcpt string, n int) []sent {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		var got []sent
		for _, m := range s.msgs {
			if m.rcpt == rcpt {
				got = append(got, m)
			}
		}
		s.mu.Unlock()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited for %d messages to %s, got %d", n, rcpt, len(got))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type harness struct {
	e    *Engine
	c    ctl.Client
	sink *sink
	dns  *dnsxtest.Fake
	smtp string
}

func start(t *testing.T) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	// Unix socket paths are short-limited: avoid the long default temp dir.
	dir, err := os.MkdirTemp("/tmp", "lfr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if _, err := Setup(ctx, SetupOptions{Data: dir, Hostname: "mail.example.com", IPv4: "203.0.113.7"}); err != nil {
		t.Fatal(err)
	}
	h := &harness{sink: &sink{}, dns: dnsxtest.New()}
	h.dns.SetIP("mail.example.com", "203.0.113.7")
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	h.smtp = l.Addr().String()
	l.Close()
	e, err := Open(ctx, Options{Data: dir, SMTPAddr: h.smtp, Version: "test", DNS: h.dns, Transport: h.sink,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	e.Config.Set(ctx, "smtp.early_talker", "false")
	e.Config.Set(ctx, "spam.dnsbl", "")
	h.e = e
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("run: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("engine did not stop")
		}
		e.Close()
	})
	h.c = ctl.Client{Path: SocketPath(dir)}
	deadline := time.Now().Add(5 * time.Second)
	for h.c.Call(ctx, api.OpStatus, nil, nil) != nil {
		if time.Now().After(deadline) {
			t.Fatal("server did not come up")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return h
}

func (h *harness) call(t *testing.T, op string, args, out any) {
	t.Helper()
	if err := h.c.Call(context.Background(), op, args, out); err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

// publish puts a domain's records into the fake DNS.
func (h *harness) publish(t *testing.T, info api.DomainInfo) {
	for _, r := range info.Records {
		switch r.Type {
		case "TXT":
			h.dns.SetTXT(r.Name, r.Value)
		case "MX":
			h.dns.SetMX(r.Name, r.Value, uint16(r.Priority))
		case "A":
			h.dns.SetIP(r.Name, r.Value)
		case "CNAME":
			h.dns.CNAME[r.Name] = r.Value
		}
	}
}

func (h *harness) web(method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.RemoteAddr = "198.51.100.20:5555"
	rec := httptest.NewRecorder()
	h.e.Web.ServeHTTP(rec, req)
	return rec
}

func (h *harness) smtpSend(from string, to []string, data string) error {
	c, err := smtp.Dial(h.smtp)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello("mx.sender.org"); err != nil {
		return err
	}
	return c.SendMail(from, to, strings.NewReader(data))
}

var linkRE = regexp.MustCompile(`https://[^\s"<>]+/confirm/[A-Za-z0-9_-]+`)

func TestNewsletterEndToEnd(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	if len(dom.Records) < 8 || !strings.Contains(dom.Zone, "IN\tMX\t10 mail.example.com.") {
		t.Fatalf("records %+v", dom)
	}
	h.publish(t, dom)
	var checked api.DomainInfo
	h.call(t, api.OpDomainDNS, api.DomainDNS{Name: "example.com", Check: true}, &checked)
	for _, c := range checked.Checks {
		if c.Status != "ok" {
			t.Errorf("record %s %s: %s %v", c.Record.Type, c.Record.Name, c.Status, c.Found)
		}
	}

	name, host, tracking, addrLine, privacy := "Bakers Weekly", "news.example.com", true, "Bakers, Street 1, Berlin", "https://example.com/privacy"
	var l lists.List
	h.call(t, api.OpListAdd, api.ListSet{Slug: "bakers", ListFields: api.ListFields{Name: &name, From: strPtr("news@example.com"), Host: &host, Tracking: &tracking, Address: &addrLine, PrivacyURL: &privacy}}, &l)
	var boxes []mailbox.Mailbox
	h.call(t, api.OpMailboxList, api.Name{Name: "example.com"}, &boxes)
	if len(boxes) != 2 { // postmaster and the list's sender
		t.Fatalf("mailboxes %+v", boxes)
	}

	// A reader signs up on the website and confirms.
	r := h.web("POST", "https://news.example.com/subscribe", url.Values{"email": {"Reader@Mail.org"}}.Encode())
	if r.Code != 200 {
		t.Fatalf("signup %d %s", r.Code, r.Body.String())
	}
	confirm := h.sink.wait(t, "reader@mail.org", 1)[0]
	link := linkRE.FindString(string(msg.Parse(confirm.raw).Text))
	if link == "" || !strings.HasPrefix(confirm.from, "bounce+") {
		t.Fatalf("confirmation mail:\n%s", confirm.raw)
	}
	if r := h.web("POST", link, ""); r.Code != 200 {
		t.Fatalf("confirm %d", r.Code)
	}
	var added api.SubAddResult
	h.call(t, api.OpSubAdd, api.SubAdd{List: "bakers", Emails: []string{"ann@other.org", "gone@other.org"}, Confirmed: true, Source: "migration", Consent: "Old signup form"}, &added)
	if added.Added != 2 {
		t.Fatalf("added %+v", added)
	}

	// The agent writes and sends an issue.
	var w issues.Issue
	h.call(t, api.OpIssueCreate, api.IssueDraft{List: "bakers", Markdown: "---\npreheader: Feed it\n---\n# Starter rescue\n\nIt is hungry, not dead. [Read more](https://example.com/guide).\n"}, &w)
	var findings []issues.Finding
	h.call(t, api.OpIssueLint, api.ID{ID: w.ID}, &findings)
	if issues.Blocking(findings) {
		t.Fatalf("lint %+v", findings)
	}
	h.call(t, api.OpIssueSend, api.IssueSend{IssueRef: api.IssueRef{ID: w.ID}, Force: true}, &w)
	got := h.sink.wait(t, "reader@mail.org", 2)[1]
	p := msg.Parse(got.raw)
	if p.Subject != "Starter rescue" || !strings.Contains(p.Header.Get("List-Unsubscribe"), "https://news.example.com/u/") {
		t.Fatalf("issue mail:\n%s", got.raw)
	}
	res := mailauth.Verifier{DNS: h.dns}.Verify(ctx, got.raw, net.ParseIP("203.0.113.7"), "mail.example.com", got.from, "example.com")
	if res.DMARC != "pass" || res.SPF != "pass" || len(res.DKIM) != 2 || res.DKIM[0].Result != "pass" || res.DKIM[1].Result != "pass" {
		t.Fatalf("authentication of our own mail: %+v", res)
	}
	h.sink.wait(t, "ann@other.org", 1)
	gone := h.sink.wait(t, "gone@other.org", 1)[0]

	// gone@other.org bounces: a DSN arrives at the VERP address.
	dsn := "From: MAILER-DAEMON@other.org\r\nTo: " + gone.from + "\r\nSubject: Undelivered\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=B\r\n\r\n--B\r\nContent-Type: text/plain\r\n\r\nfailed\r\n" +
		"--B\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; other.org\r\n\r\nFinal-Recipient: rfc822; gone@other.org\r\nAction: failed\r\nStatus: 5.1.1\r\n\r\n--B--\r\n"
	if err := h.smtpSend("", []string{gone.from}, dsn); err != nil {
		t.Fatal("dsn:", err)
	}
	var contact lists.Contact
	h.call(t, api.OpContactShow, api.Name{Name: "gone@other.org"}, &contact)
	if !strings.HasPrefix(contact.Suppressed, "hard bounce") {
		t.Fatalf("contact %+v", contact)
	}

	// Ann unsubscribes with one click from her mail client.
	annMsg := msg.Parse(h.sink.wait(t, "ann@other.org", 1)[0].raw)
	unsub := strings.Trim(strings.Split(annMsg.Header.Get("List-Unsubscribe"), ",")[0], "<> ")
	if r := h.web("POST", unsub, "List-Unsubscribe=One-Click"); r.Code != 200 {
		t.Fatalf("one-click %d", r.Code)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var st issues.Stats
		h.call(t, api.OpIssueStats, api.ID{ID: w.ID}, &st)
		if st.Sent == 2 && st.Bounced == 1 && st.Unsubscribed == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stats %+v", st)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The scheduler marks it sent within seconds.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		h.call(t, api.OpIssueShow, api.ID{ID: w.ID}, &w)
		if w.Status == "sent" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("issue status %s", w.Status)
		}
	}
	if r := h.web("GET", "https://news.example.com/issues/starter-rescue", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "hungry, not dead") {
		t.Fatalf("archive page %d", r.Code)
	}
}

func strPtr(s string) *string { return &s }

func TestMailboxEndToEnd(t *testing.T) {
	h := start(t)
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.publish(t, dom)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "jo@example.com", Name: "Jo"}, nil)
	h.call(t, api.OpAliasAdd, api.Alias{Alias: "hello@example.com", Target: "jo@example.com"}, nil)

	if err := h.smtpSend("ann@sender.org", []string{"nobody@example.com"}, "Subject: x\r\n\r\ny\r\n"); err == nil {
		t.Fatal("unknown recipient accepted")
	}
	in := "From: Ann <ann@sender.org>\r\nTo: hello@example.com\r\nSubject: Sourdough question\r\nMessage-ID: <q1@sender.org>\r\nDate: Mon, 19 Oct 2026 10:00:00 +0000\r\n\r\nMy starter smells of acetone.\r\n"
	if err := h.smtpSend("ann@sender.org", []string{"hello+x@example.com"}, in); err != nil {
		t.Fatal(err)
	}
	var list []mailbox.Summary
	h.call(t, api.OpMailList, api.MailList{Mailbox: "jo@example.com"}, &list)
	if len(list) != 1 || list[0].Subject != "Sourdough question" {
		t.Fatalf("inbox %+v", list)
	}
	h.call(t, api.OpMailList, api.MailList{Search: "acetone"}, &list)
	if len(list) != 1 {
		t.Fatal("search")
	}
	var m mailbox.Message
	h.call(t, api.OpMailRead, api.MailRead{ID: list[0].ID}, &m)
	if !strings.Contains(m.Text, "acetone") || !strings.Contains(string(m.Verdict), `"auth"`) {
		t.Fatalf("read %+v", m)
	}
	var s api.Sent
	h.call(t, api.OpMailReply, api.MailReply{ID: m.ID, Markdown: "Feed it **twice** a day."}, &s)
	reply := msg.Parse(h.sink.wait(t, "ann@sender.org", 1)[0].raw)
	if reply.Subject != "Re: Sourdough question" || reply.InReplyTo != "q1@sender.org" || !strings.Contains(reply.HTML, "<strong") {
		t.Fatalf("reply %+v", reply)
	}
	// Ann is now a known contact; her next mail scores lower. Moving it to junk trains the filter.
	var moved map[string]any
	h.call(t, api.OpMailMove, api.MailMove{IDs: []int64{m.ID}, Folder: "spam"}, &moved)
	if moved["trained"].(float64) != 1 {
		t.Fatalf("move %+v", moved)
	}
	var spamStats map[string]any
	h.call(t, api.OpSpamStats, nil, &spamStats)
	if spamStats["stats"].(map[string]any)["trained_spam"].(float64) != 1 {
		t.Fatalf("spam stats %+v", spamStats)
	}
	var sent api.Sent
	h.call(t, api.OpMailSend, api.MailSend{From: "jo@example.com", To: []string{"bob@x.org"}, Subject: "Hi", Text: "plain"}, &sent)
	h.sink.wait(t, "bob@x.org", 1)

	err := h.c.Call(context.Background(), api.OpMailSend, api.MailSend{From: "ceo@example.com", To: []string{"x@y.org"}, Subject: "x", Text: "y"}, nil)
	var ce *ctl.Error
	if !errors.As(err, &ce) || ce.Code != ctl.CodeInvalid {
		t.Fatalf("send as unknown sender: %v", err)
	}

	var st api.Status
	h.call(t, api.OpStatus, nil, &st)
	if st.Domains != 1 || st.Mailboxes != 2 || len(st.Listening) == 0 {
		t.Fatalf("status %+v", st)
	}
	var checks []api.Check
	h.call(t, api.OpDoctor, nil, &checks)
	if len(checks) < 5 {
		t.Fatalf("doctor %+v", checks)
	}
}

func TestWatchStreamsEvents(t *testing.T) {
	h := start(t)
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "jo@example.com"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 4)
	go h.c.Stream(ctx, api.OpWatch, api.Watch{Kinds: []string{"received"}}, func(m json.RawMessage) error {
		got <- string(m)
		return nil
	})
	// The stream may not be subscribed yet when the first mail arrives:
	// keep sending until one shows up.
	deadline := time.After(5 * time.Second)
	for received := false; !received; {
		h.smtpSend("a@b.org", []string{"jo@example.com"}, "From: a@b.org\r\nSubject: ping\r\n\r\nx\r\n")
		select {
		case ev := <-got:
			if !strings.Contains(ev, `"kind":"received"`) || !strings.Contains(ev, "ping") {
				t.Fatal(ev)
			}
			received = true
		case <-time.After(200 * time.Millisecond):
		case <-deadline:
			t.Fatal("no event")
		}
	}
	cancel()
	_ = http.StatusOK
}

func TestReplyUnsubscribe(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.call(t, api.OpListAdd, api.ListSet{Slug: "bakers", ListFields: api.ListFields{Name: strPtr("Bakers"), From: strPtr("news@example.com")}}, nil)
	h.call(t, api.OpSubAdd, api.SubAdd{List: "bakers", Emails: []string{"ann@sender.org", "bob@sender.org"}, Confirmed: true, Consent: "Old signup form"}, nil)
	reply := func(from, subject, body, dmarc string) {
		raw := "From: " + from + "\r\nTo: news@example.com\r\nSubject: " + subject + "\r\n\r\n" + body + "\r\n"
		h.e.replyUnsubscribe(ctx, "news@example.com", &inbound.Envelope{Parsed: msg.Parse([]byte(raw)), Auth: mailauth.Result{DMARC: dmarc}})
	}
	status := func(email string) string {
		m, err := h.e.Lists.Find(ctx, "bakers", email)
		if err != nil {
			t.Fatal(err)
		}
		return m.Status
	}
	reply("ann@sender.org", "Re: Starter rescue", "Unsubscribe\r\n\r\n> quoted issue", "fail")
	if status("ann@sender.org") != "confirmed" {
		t.Fatal("unauthenticated reply unsubscribed")
	}
	// Unproven, the reader still gets their own link, once.
	reply("ann@sender.org", "Re: Starter rescue", "unsubscribe", "none")
	got := h.sink.wait(t, "ann@sender.org", 1)
	if len(got) != 1 || !strings.Contains(string(got[0].raw), "/u/") {
		t.Fatalf("unsubscribe link mail: %d", len(got))
	}
	reply("ann@sender.org", "Re: Starter rescue", "Loved it, thanks! I will not unsubscribe.", "pass")
	if status("ann@sender.org") != "confirmed" {
		t.Fatal("a normal reply unsubscribed")
	}
	reply("ann@sender.org", "Re: Starter rescue", "Unsubscribe.\r\n\r\n> quoted issue", "pass")
	reply("bob@sender.org", "AW: abmelden", "", "pass")
	if status("ann@sender.org") != "unsubscribed" || status("bob@sender.org") != "unsubscribed" {
		t.Fatal("reply unsubscribe ignored")
	}
}

func TestFormSub(t *testing.T) {
	for in, want := range map[string]string{"198.51.100.7": "v=1; ip4=198.51.x.x", "": "v=1; ip=none", "2001:db8:1:2::5": "v=1; ip6=2001:db8:1::x", "::ffff:192.0.2.1": "v=1; ip4=192.0.x.x"} {
		if got := formSub(in); got != want {
			t.Errorf("formSub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIssueImages(t *testing.T) {
	h := start(t)
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.call(t, api.OpListAdd, api.ListSet{Slug: "bakers", ListFields: api.ListFields{Name: strPtr("Bakers"), From: strPtr("news@example.com"), Host: strPtr("news.example.com")}}, nil)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\x0cIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
	md := "# Crumb\n\n![An open crumb](images/crumb.png)\n\nLook at the holes.\n"
	files := []issues.File{{Path: "images/crumb.png", Data: png}}
	var res struct {
		Issue issues.Issue `json:"issue"`
	}
	h.call(t, api.OpIssuePut, api.IssueDraft{List: "bakers", Slug: "crumb", Markdown: md, Media: &files}, &res)
	var p api.Preview
	h.call(t, api.OpIssuePreview, api.IssueRef{ID: res.Issue.ID}, &p)
	re := regexp.MustCompile(`https://news\.example\.com/media/([0-9a-f]+)\.png`)
	m := re.FindStringSubmatch(p.HTML)
	if m == nil {
		t.Fatalf("image not resolved: %s", p.HTML)
	}
	r := h.web("GET", m[0], "")
	if r.Code != 200 || r.Header().Get("Content-Type") != "image/png" || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") || r.Body.Len() != len(png) {
		t.Fatalf("media %d %v", r.Code, r.Header())
	}
	if r := h.web("GET", "https://news.example.com/media/"+m[1]+".gif", ""); r.Code != 404 {
		t.Fatal("wrong extension served")
	}
	// WebP is refused; a referenced image never uploaded is a lint error.
	webp := []issues.File{{Path: "a.webp", Data: []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")}}
	if err := h.c.Call(context.Background(), api.OpIssuePut, api.IssueDraft{List: "bakers", Slug: "w", Markdown: "# W\n\n![x](a.webp)\n", Media: &webp}, nil); err == nil || !strings.Contains(err.Error(), "Outlook") {
		t.Fatalf("webp accepted: %v", err)
	}
	h.call(t, api.OpIssuePut, api.IssueDraft{List: "bakers", Slug: "missing", Markdown: "# M\n\n![x](gone.jpg)\n"}, &res)
	var fs []issues.Finding
	h.call(t, api.OpIssueLint, api.IssueRef{ID: res.Issue.ID}, &fs)
	found := false
	for _, f := range fs {
		found = found || f.Rule == "images.missing"
	}
	if !found {
		t.Fatalf("missing image not flagged: %+v", fs)
	}
}

// Forwarding never loops: bounces, automatic mail and mail this mailbox
// forwarded before stay in the inbox, and a forward that fails for good
// leaves a delivery notice there.
func TestForwardingGuards(t *testing.T) {
	h := start(t)
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "jo@example.com"}, nil)
	far := "far@elsewhere.org"
	h.call(t, api.OpMailboxSet, api.MailboxSet{Address: "jo@example.com", Forward: &far}, nil)
	h.e.Config.Set(context.Background(), "spam.junk", "0.97") // bare test mail scores high
	send := func(from, extra string) {
		t.Helper()
		data := "From: ann@sender.org\r\nTo: jo@example.com\r\nSubject: s\r\n" + extra + "\r\nbody\r\n"
		if err := h.smtpSend(from, []string{"jo@example.com"}, data); err != nil {
			t.Fatal(err)
		}
	}
	send("ann@sender.org", "")
	h.sink.wait(t, far, 1)
	send("", "")
	send("ann@sender.org", "Auto-Submitted: auto-replied\r\n")
	send("ann@sender.org", "X-Forwarded-For: other@example.net, jo@example.com\r\n")
	time.Sleep(300 * time.Millisecond)
	if got := len(h.sink.wait(t, far, 1)); got != 1 {
		t.Fatalf("forwarded %d messages, want only the first", got)
	}
	h.sink.mu.Lock()
	h.sink.fail = func(rcpt string) error {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"}
	}
	h.sink.mu.Unlock()
	send("ann@sender.org", "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		var list []mailbox.Summary
		h.call(t, api.OpMailList, api.MailList{Mailbox: "jo@example.com"}, &list)
		found := false
		for _, m := range list {
			if strings.Contains(m.Subject, "Undelivered mail to "+far) {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no delivery notice in %+v", list)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Feedback acts only on mail lfr sent: a relay webhook about an unknown
// address changes nothing, one about a reader unsubscribes and suppresses
// them; a complaint report counts only when its sender is authenticated.
// A backup holds the database and every message.
func TestFeedbackAndBackup(t *testing.T) {
	h := start(t)
	var dom api.DomainInfo
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, &dom)
	h.publish(t, dom)
	h.call(t, api.OpListAdd, api.ListSet{Slug: "bakers", ListFields: api.ListFields{Name: strPtr("Bakers"), From: strPtr("news@example.com"), Address: strPtr("Bakers, Street 1, Berlin")}}, nil)
	h.call(t, api.OpSubAdd, api.SubAdd{List: "bakers", Emails: []string{"ann@other.org", "bob@other.org", "cy@other.org"}, Confirmed: true, Consent: "Old signup form"}, nil)
	var w issues.Issue
	h.call(t, api.OpIssueCreate, api.IssueDraft{List: "bakers", Markdown: "# Rye\n\nBake it 15 minutes longer.\n"}, &w)
	h.call(t, api.OpIssueSend, api.IssueSend{IssueRef: api.IssueRef{ID: w.ID}, Force: true}, &w)
	cyMail := h.sink.wait(t, "cy@other.org", 1)[0]
	h.sink.wait(t, "ann@other.org", 1)
	h.sink.wait(t, "bob@other.org", 1)

	var relay api.RelayInfo
	h.call(t, api.OpRelayShow, nil, &relay)
	hook := relay.Webhooks["generic"]
	post := func(body string) {
		req := httptest.NewRequest("POST", hook, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.e.Web.ServeHTTP(rec, req)
		if rec.Code != 204 {
			t.Fatalf("webhook %d", rec.Code)
		}
	}
	contact := func(email string) lists.Contact {
		var c lists.Contact
		h.c.Call(context.Background(), api.OpContactShow, api.Name{Name: email}, &c)
		return c
	}
	post(`{"type":"complaint","email":"stranger@other.org"}`)
	if c := contact("stranger@other.org"); c.Suppressed != "" {
		t.Fatal("a webhook about mail lfr never sent suppressed an address")
	}
	post(`{"type":"complaint","email":"bob@other.org"}`)
	if c := contact("bob@other.org"); c.Suppressed != "complaint" {
		t.Fatalf("relay complaint ignored: %+v", c)
	}

	// An abuse report quoting Cy's issue: unauthenticated it is ignored.
	arf := func(from string) string {
		return "From: " + from + "\r\nTo: postmaster@example.com\r\nSubject: Abuse report\r\nMIME-Version: 1.0\r\n" +
			"Content-Type: multipart/report; report-type=feedback-report; boundary=B\r\n\r\n--B\r\nContent-Type: text/plain\r\n\r\nspam\r\n" +
			"--B\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: abuse\r\nVersion: 1\r\n\r\n" +
			"--B\r\nContent-Type: text/rfc822-headers\r\n\r\nMessage-ID: " + msg.Parse(cyMail.raw).Header.Get("Message-ID") + "\r\nTo: cy@other.org\r\n\r\n--B--\r\n"
	}
	if err := h.smtpSend("fbl@forged.example", []string{"postmaster@example.com"}, arf("fbl@forged.example")); err != nil {
		t.Fatal(err)
	}
	if c := contact("cy@other.org"); c.Suppressed != "" {
		t.Fatal("an unauthenticated complaint report suppressed a reader")
	}
	h.dns.SetTXT("fbl.example", "v=spf1 ip4:127.0.0.1 -all")
	h.dns.SetTXT("_dmarc.fbl.example", "v=DMARC1; p=reject")
	if err := h.smtpSend("fbl@fbl.example", []string{"postmaster@example.com"}, arf("fbl@fbl.example")); err != nil {
		t.Fatal(err)
	}
	if c := contact("cy@other.org"); c.Suppressed != "complaint" {
		t.Fatalf("authenticated complaint report ignored: %+v", c)
	}

	dir := filepath.Join(t.TempDir(), "backup")
	var b api.BackupResult
	h.call(t, api.OpBackup, api.Backup{Dir: dir}, &b)
	if _, err := os.Stat(filepath.Join(dir, BackupDone)); err != nil || b.Blobs == 0 {
		t.Fatalf("backup %+v %v", b, err)
	}
}

// Mail between the server's own domains is delivered on the spot: it never
// touches the relay and never lands in junk for lack of published DNS.
func TestLocalDelivery(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	h.call(t, api.OpDomainAdd, api.Name{Name: "other.example"}, nil)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "jo@example.com"}, nil)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "ann@other.example"}, nil)
	var sent api.Sent
	h.call(t, api.OpMailSend, api.MailSend{From: "jo@example.com", To: []string{"ann@other.example", "nobody@other.example"}, Subject: "Intern", Text: "Hallo Ann"}, &sent)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var list []mailbox.Summary
		h.call(t, api.OpMailList, api.MailList{Mailbox: "ann@other.example"}, &list)
		if len(list) == 1 && list[0].Folder == "inbox" && list[0].Subject == "Intern" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("local mail not in the inbox: %+v", list)
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.sink.mu.Lock()
	n := len(h.sink.msgs)
	h.sink.mu.Unlock()
	if n != 0 {
		t.Fatalf("local mail went through the relay (%d messages)", n)
	}
	_ = ctx
}

// An issue sent to a list nobody confirmed yet would be spent on nobody.
func TestSendToEmptyListRefused(t *testing.T) {
	h := start(t)
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	h.call(t, api.OpListAdd, api.ListSet{Slug: "news", ListFields: api.ListFields{Name: strPtr("News"), From: strPtr("news@example.com"), Address: strPtr("News, Street 1, Berlin")}}, nil)
	var w issues.Issue
	h.call(t, api.OpIssueCreate, api.IssueDraft{List: "news", Markdown: "# Hello\n\nFirst issue.\n"}, &w)
	err := h.c.Call(context.Background(), api.OpIssueSend, api.IssueSend{IssueRef: api.IssueRef{ID: w.ID}, Force: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "no confirmed readers") {
		t.Fatalf("send to an empty list: %v", err)
	}
	h.call(t, api.OpIssueShow, api.IssueRef{ID: w.ID}, &w)
	if w.Status != "draft" {
		t.Fatalf("status %s", w.Status)
	}
}
