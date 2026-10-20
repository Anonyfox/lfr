package outbound

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/smtptest"
)

var testMsg = []byte("From: a@example.com\r\nTo: b@example.org\r\nSubject: t\r\n\r\nhello\r\n")

func relayFor(t *testing.T, scheme string, s *smtptest.Server, pass string) *Relay {
	t.Helper()
	r, err := ParseRelay(scheme+"://user@"+s.Addr, pass, "mail.example.com")
	if err != nil {
		t.Fatal(err)
	}
	r.tls = &tls.Config{InsecureSkipVerify: true}
	return r
}

func TestRelaySTARTTLSAuthAndReuse(t *testing.T) {
	s := smtptest.Start(t, false, func(s *smtptest.Server) {
		s.User, s.Password = "user", "secret"
		s.Reject = func(rcpt string) error {
			if strings.HasPrefix(rcpt, "gone@") {
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"}
			}
			return nil
		}
	})
	r := relayFor(t, "smtp", s, "secret")
	defer r.Close()
	ctx := context.Background()
	if err := r.Send(ctx, "bounce@example.com", "b@example.org", testMsg); err != nil {
		t.Fatal(err)
	}
	err := r.Send(ctx, "bounce@example.com", "gone@example.org", testMsg)
	f := Classify(err)
	if f == nil || f.Temporary || !f.Bounce || f.Enhanced != "5.1.1" {
		t.Fatalf("rejection classified as %+v", f)
	}
	if err := r.Send(ctx, "bounce@example.com", "c@example.org", testMsg); err != nil {
		t.Fatal("after rejection:", err)
	}
	msgs := s.Messages()
	if len(msgs) != 2 || !msgs[0].TLS || msgs[0].User != "user" || msgs[0].Helo != "mail.example.com" || msgs[0].From != "bounce@example.com" {
		t.Fatalf("messages %+v", msgs)
	}
	r.mu.Lock()
	idle := len(r.idle)
	r.mu.Unlock()
	if idle != 1 {
		t.Fatalf("idle connections %d, want the one reused connection", idle)
	}
}

func TestRelayImplicitTLS(t *testing.T) {
	s := smtptest.Start(t, true, func(s *smtptest.Server) { s.User, s.Password = "user", "pw" })
	r := relayFor(t, "smtps", s, "pw")
	defer r.Close()
	if err := r.Send(context.Background(), "a@example.com", "b@example.org", testMsg); err != nil {
		t.Fatal(err)
	}
	if m := s.Messages(); len(m) != 1 || !m[0].TLS {
		t.Fatalf("messages %+v", m)
	}
}

func TestRelayBadPasswordIsTemporary(t *testing.T) {
	s := smtptest.Start(t, false, func(s *smtptest.Server) { s.User, s.Password = "user", "right" })
	r := relayFor(t, "smtp", s, "wrong")
	f := Classify(r.Send(context.Background(), "a@example.com", "b@example.org", testMsg))
	if f == nil || !f.Temporary || !strings.Contains(f.Message, "login failed") {
		t.Fatalf("got %+v", f)
	}
}

func TestParseRelay(t *testing.T) {
	r, err := ParseRelay("smtps://AKIA@email-smtp.eu-central-1.amazonaws.com:2465", "pw", "h")
	if err != nil || !r.implicit || r.addr != "email-smtp.eu-central-1.amazonaws.com:2465" || r.user != "AKIA" || r.plaintext {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = ParseRelay("smtp://u:p@mail.smtp2go.com", "", "h")
	if r.implicit || r.addr != "mail.smtp2go.com:587" || r.password != "p" {
		t.Fatalf("%+v", r)
	}
	if _, err := ParseRelay("http://x", "", "h"); err == nil {
		t.Fatal("accepted http")
	}
}

func directFor(s *smtptest.Server, dns *dnsxtest.Fake) *Direct {
	return &Direct{LocalName: "mail.example.com", DNS: dns, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, s.Addr)
	}}
}

func TestDirectOpportunisticTLS(t *testing.T) {
	s := smtptest.Start(t, false, nil)
	dns := dnsxtest.New()
	dns.SetMX("example.org", "mx2.example.org", 20)
	dns.SetMX("example.org", "mx1.example.org", 10)
	d := directFor(s, dns)
	if err := d.Send(context.Background(), "bounce@example.com", "b@example.org", testMsg); err != nil {
		t.Fatal(err)
	}
	if m := s.Messages(); len(m) != 1 || !m[0].TLS || m[0].Helo != "mail.example.com" {
		t.Fatalf("messages %+v", m)
	}
}

func TestDirectMTASTSEnforceRejectsBadCert(t *testing.T) {
	s := smtptest.Start(t, false, nil)
	dns := dnsxtest.New()
	dns.SetMX("example.org", "mx1.example.org", 10)
	d := directFor(s, dns)
	d.STS = &STSCache{DNS: dns}
	d.STS.cache = map[string]stsEntry{"example.org": {policy: &STSPolicy{Mode: "enforce", MX: []string{"*.example.org"}}, refresh: time.Now().Add(time.Hour)}}
	f := Classify(d.Send(context.Background(), "a@example.com", "b@example.org", testMsg))
	if f == nil || !f.Temporary || !strings.Contains(f.Message, "MTA-STS") {
		t.Fatalf("got %+v", f)
	}
	d.STS.cache["example.org"] = stsEntry{policy: &STSPolicy{Mode: "enforce", MX: []string{"other.net"}}, refresh: time.Now().Add(time.Hour)}
	f = Classify(d.Send(context.Background(), "a@example.com", "b@example.org", testMsg))
	if f == nil || !strings.Contains(f.Message, "not allowed") {
		t.Fatalf("got %+v", f)
	}
}

func TestDirectNoDomain(t *testing.T) {
	d := &Direct{LocalName: "h", DNS: dnsxtest.New()}
	f := Classify(d.Send(context.Background(), "a@example.com", "b@nowhere.invalid", testMsg))
	if f == nil || !f.Bounce {
		t.Fatalf("got %+v", f)
	}
}

func TestIsHardBounce(t *testing.T) {
	cases := []struct {
		code     int
		enhanced string
		msg      string
		want     bool
	}{
		{550, "5.1.1", "user unknown", true},
		{550, "5.7.1", "message rejected as spam", false},
		{550, "", "Requested action not taken: mailbox unavailable", true},
		{550, "", "Blocked by policy", false},
		{554, "", "transaction failed", false},
		{553, "", "", true},
	}
	for _, c := range cases {
		if got := IsHardBounce(c.code, c.enhanced, c.msg); got != c.want {
			t.Errorf("%d %s %q = %v", c.code, c.enhanced, c.msg, got)
		}
	}
}

func TestSTSPolicy(t *testing.T) {
	p := ParseSTSPolicy(strings.NewReader("version: STSv1\nmode: enforce\nmx: mail.example.com\nmx: *.example.net\nmax_age: 86400\n"))
	if !p.Enforce() || !p.Matches("mail.example.com") || !p.Matches("a.example.net") || p.Matches("a.b.example.net") || p.MaxAge != 24*time.Hour {
		t.Fatalf("%+v", p)
	}
	if ParseSTSPolicy(strings.NewReader("mode: enforce")) != nil {
		t.Fatal("accepted a policy without version")
	}
}

// A cached enforcing policy binds until max_age runs out, even when the
// record and policy host cannot be reached: blocking the fetch must not
// downgrade TLS (RFC 8461 §5.1).
func TestSTSCacheKeepsPolicyUntilMaxAge(t *testing.T) {
	c := &STSCache{DNS: dnsxtest.New()}
	p := &STSPolicy{Mode: "enforce", MX: []string{"mx.example.org"}}
	c.cache = map[string]stsEntry{"example.org": {policy: p, refresh: time.Now().Add(-time.Minute), valid: time.Now().Add(6 * 24 * time.Hour)}}
	if got := c.Get(context.Background(), "example.org"); got != p {
		t.Fatalf("policy dropped while still valid: %+v", got)
	}
	c.cache["example.org"] = stsEntry{policy: p, refresh: time.Now().Add(-time.Minute), valid: time.Now().Add(-time.Second)}
	if got := c.Get(context.Background(), "example.org"); got != nil {
		t.Fatalf("expired policy kept: %+v", got)
	}
}

// Only a refused RCPT says the address is gone; a refused MAIL or DATA is
// about the sender or the content, and at a relay a refused MAIL stops
// everything.
func TestRefusalStages(t *testing.T) {
	refuse := func(msg string) error {
		return &smtp.SMTPError{Code: 550, Message: msg}
	}
	dns := dnsxtest.New()
	dns.SetMX("example.org", "mx1.example.org", 10)
	for _, c := range []struct {
		name      string
		configure func(*smtptest.Server)
		bounce    bool
	}{
		{"rcpt", func(s *smtptest.Server) {
			s.Reject = func(string) error {
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"}
			}
		}, true},
		{"mail", func(s *smtptest.Server) { s.RejectMail = func(string) error { return refuse("no such user (sender)") } }, false},
		{"data", func(s *smtptest.Server) { s.RejectData = func() error { return refuse("recipient rejected: content") } }, false},
	} {
		s := smtptest.Start(t, false, c.configure)
		f := Classify(directFor(s, dns).Send(context.Background(), "a@example.com", "b@example.org", testMsg))
		if f == nil || f.Temporary || f.Bounce != c.bounce || f.Transport {
			t.Errorf("%s: %+v", c.name, f)
		}
	}
	s := smtptest.Start(t, false, func(s *smtptest.Server) { s.RejectMail = func(string) error { return refuse("sender not verified") } })
	r, err := ParseRelay("smtp://"+s.Addr, "", "mail.example.com")
	if err != nil {
		t.Fatal(err)
	}
	r.plaintext = true
	f := Classify(r.Send(context.Background(), "a@example.com", "b@example.org", testMsg))
	if f == nil || !f.Transport || f.Bounce {
		t.Errorf("relay mail stage: %+v", f)
	}
}

// A host that refuses the greeting does not end delivery: the next MX may
// accept.
func TestRefusedGreetingTriesNextMX(t *testing.T) {
	good := smtptest.Start(t, false, nil)
	bad, _ := net.Listen("tcp", "127.0.0.1:0")
	defer bad.Close()
	go func() {
		for {
			c, err := bad.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("554 no service here\r\n"))
			c.Close()
		}
	}()
	dns := dnsxtest.New()
	dns.SetMX("example.org", "mx1.example.org", 10)
	dns.SetMX("example.org", "mx2.example.org", 20)
	d := &Direct{LocalName: "mail.example.com", DNS: dns, Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		target := good.Addr
		if strings.HasPrefix(addr, "mx1.") {
			target = bad.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, target)
	}}
	if err := d.Send(context.Background(), "a@example.com", "b@example.org", testMsg); err != nil {
		t.Fatal(err)
	}
	if len(good.Messages()) != 1 {
		t.Fatal("second MX not tried")
	}
}
