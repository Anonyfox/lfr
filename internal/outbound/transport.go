package outbound

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/Anonyfox/lfr/internal/dnsx"
)

// Transport hands one message to the next hop.
type Transport interface {
	Send(ctx context.Context, from, rcpt string, raw []byte) error
	Close()
	Name() string
}

// Failure is a delivery error with its consequence.
type Failure struct {
	Temporary bool   // try again later
	Transport bool   // the relay itself is unreachable or refuses our login: nothing can go out
	Bounce    bool   // the address does not exist: suppress it
	Code      int    // SMTP reply code, 0 for network errors
	Enhanced  string // e.g. 5.1.1
	Message   string
	Handshake bool // refused before MAIL FROM: another MX may still accept
}

func (f *Failure) Error() string {
	if f.Code == 0 {
		return f.Message
	}
	if f.Enhanced != "" {
		return fmt.Sprintf("%d %s %s", f.Code, f.Enhanced, f.Message)
	}
	return fmt.Sprintf("%d %s", f.Code, f.Message)
}

// Classify turns any send error into a Failure.
func Classify(err error) *Failure {
	if err == nil {
		return nil
	}
	var f *Failure
	if errors.As(err, &f) {
		return f
	}
	var se *smtp.SMTPError
	if errors.As(err, &se) {
		f := &Failure{Code: se.Code, Message: strings.TrimSpace(se.Message)}
		if se.EnhancedCode != smtp.NoEnhancedCode && se.EnhancedCode != (smtp.EnhancedCode{}) {
			f.Enhanced = fmt.Sprintf("%d.%d.%d", se.EnhancedCode[0], se.EnhancedCode[1], se.EnhancedCode[2])
		}
		f.Temporary = se.Code < 500
		f.Bounce = !f.Temporary && IsHardBounce(f.Code, f.Enhanced, f.Message)
		return f
	}
	return &Failure{Temporary: true, Message: err.Error()}
}

// transact runs one MAIL, RCPT, DATA exchange and classifies a refusal by
// the stage it came in: only a refused RCPT says anything about the
// recipient's address (RFC 3463). A refused MAIL is about the sender; at a
// relay it means our account or sender is not accepted at all, which no
// retry of this message fixes. A refused DATA is about the content.
func transact(c *smtp.Client, from, rcpt string, raw []byte, relay bool) error {
	stage := func(name string, err error) error {
		if err == nil {
			return nil
		}
		f := Classify(err)
		if f.Code == 0 {
			return f
		}
		switch name {
		case "rcpt":
		case "mail":
			f.Bounce = false
			f.Transport = relay && !f.Temporary
		default:
			f.Bounce = false
		}
		return f
	}
	if err := c.Mail(from, nil); err != nil {
		return stage("mail", err)
	}
	if err := c.Rcpt(rcpt, nil); err != nil {
		return stage("rcpt", err)
	}
	w, err := c.Data()
	if err != nil {
		return stage("data", err)
	}
	if _, err := w.Write(raw); err != nil {
		w.Close()
		return stage("data", err)
	}
	return stage("data", w.Close())
}

// IsHardBounce decides whether a permanent failure means the address is
// gone (stop sending to it) rather than a policy or content rejection.
func IsHardBounce(code int, enhanced, message string) bool {
	switch enhanced {
	case "5.1.1", "5.1.2", "5.1.3", "5.1.6", "5.1.10", "5.2.1":
		return true
	}
	if enhanced != "" {
		return false
	}
	m := strings.ToLower(message)
	for _, policy := range []string{"spam", "block", "policy", "reputation", "rejected due", "denied", "dmarc", "spf", "dkim"} {
		if strings.Contains(m, policy) {
			return false
		}
	}
	for _, gone := range []string{"no such user", "user unknown", "does not exist", "unknown user", "mailbox unavailable", "invalid recipient", "recipient rejected", "no mailbox", "address rejected", "not found"} {
		if strings.Contains(m, gone) {
			return true
		}
	}
	return code == 550 || code == 551 || code == 553
}

// Relay submits everything to one smarthost with authentication, keeping a
// few connections open between messages.
type Relay struct {
	addr      string
	implicit  bool // TLS from the first byte (smtps://), else STARTTLS
	plaintext bool // only for a relay on the loopback interface
	user      string
	password  string
	localName string
	tls       *tls.Config
	dial      func(ctx context.Context, network, addr string) (net.Conn, error)

	mu   sync.Mutex
	idle []*pooled
}

type pooled struct {
	c    *smtp.Client
	used time.Time
}

// ParseRelay builds a Relay from smtps://user@host:port or smtp://user@host:port.
func ParseRelay(rawURL, password, localName string) (*Relay, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("relay url: %v", err)
	}
	r := &Relay{localName: localName, password: password}
	switch u.Scheme {
	case "smtps":
		r.implicit = true
	case "smtp":
	default:
		return nil, fmt.Errorf("relay url: scheme must be smtps:// (TLS) or smtp:// (STARTTLS), got %q", u.Scheme)
	}
	host, port := u.Hostname(), u.Port()
	if host == "" {
		return nil, errors.New("relay url: no host")
	}
	if port == "" {
		port = map[bool]string{true: "465", false: "587"}[r.implicit]
	}
	r.addr = net.JoinHostPort(host, port)
	if u.User != nil {
		r.user = u.User.Username()
		if p, ok := u.User.Password(); ok && r.password == "" {
			r.password = p
		}
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() || host == "localhost" {
		r.plaintext = !r.implicit
	}
	r.tls = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	d := &net.Dialer{Timeout: 30 * time.Second}
	r.dial = d.DialContext
	return r, nil
}

// Name describes the relay for status output.
func (r *Relay) Name() string { return "relay " + r.addr }

func (r *Relay) connect(ctx context.Context) (*smtp.Client, error) {
	conn, err := r.dial(ctx, "tcp", r.addr)
	if err != nil {
		return nil, &Failure{Temporary: true, Transport: true, Message: "connect to relay: " + err.Error()}
	}
	if r.implicit {
		tc := tls.Client(conn, r.tls)
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, &Failure{Temporary: true, Transport: true, Message: "relay TLS: " + err.Error()}
		}
		conn = tc
	} else {
		up, secure, err := upgradeConn(ctx, conn, r.localName, r.tls)
		if err != nil {
			conn.Close()
			return nil, &Failure{Temporary: true, Transport: true, Message: "relay STARTTLS: " + err.Error()}
		}
		if !secure && !r.plaintext {
			conn.Close()
			return nil, &Failure{Temporary: true, Transport: true, Message: "relay offers no STARTTLS; refusing to send credentials in clear text (use smtps://)"}
		}
		conn = up
	}
	c := smtp.NewClient(conn)
	c.CommandTimeout = 2 * time.Minute
	c.SubmissionTimeout = 5 * time.Minute
	if err := c.Hello(r.localName); err != nil {
		c.Close()
		return nil, &Failure{Temporary: true, Transport: true, Message: "relay greeting: " + err.Error()}
	}
	if r.user != "" {
		var mech sasl.Client
		if c.SupportsAuth("PLAIN") {
			mech = sasl.NewPlainClient("", r.user, r.password)
		} else if c.SupportsAuth("LOGIN") {
			mech = sasl.NewLoginClient(r.user, r.password)
		} else {
			c.Close()
			return nil, &Failure{Temporary: true, Transport: true, Message: "relay supports neither AUTH PLAIN nor LOGIN"}
		}
		if err := c.Auth(mech); err != nil {
			c.Close()
			// A rejected login is the operator's to fix; retrying keeps the mail.
			return nil, &Failure{Temporary: true, Transport: true, Message: "relay login failed: " + err.Error()}
		}
	}
	return c, nil
}

func (r *Relay) get(ctx context.Context) (*smtp.Client, error) {
	r.mu.Lock()
	for len(r.idle) > 0 {
		p := r.idle[len(r.idle)-1]
		r.idle = r.idle[:len(r.idle)-1]
		if time.Since(p.used) < 50*time.Second {
			r.mu.Unlock()
			if p.c.Noop() == nil {
				return p.c, nil
			}
			p.c.Close()
			r.mu.Lock()
			continue
		}
		p.c.Close()
	}
	r.mu.Unlock()
	return r.connect(ctx)
}

func (r *Relay) put(c *smtp.Client) {
	if c.Reset() != nil {
		c.Close()
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.idle) >= 8 {
		c.Close()
		return
	}
	r.idle = append(r.idle, &pooled{c: c, used: time.Now()})
}

// Relayed reports that mail goes through a smarthost, which may rewrite
// some header fields (see mailauth.RelayHeaders).
func (r *Relay) Relayed() bool { return true }

// Send submits one message.
func (r *Relay) Send(ctx context.Context, from, rcpt string, raw []byte) error {
	c, err := r.get(ctx)
	if err != nil {
		return err
	}
	if err := transact(c, from, rcpt, raw, true); err != nil {
		f := Classify(err)
		var se *smtp.SMTPError
		if errors.As(err, &se) {
			r.put(c) // the session survived a rejected command
		} else {
			c.Close()
		}
		return f
	}
	r.put(c)
	return nil
}

// Ping connects, logs in and says goodbye, to test the configuration.
func (r *Relay) Ping(ctx context.Context) error {
	c, err := r.connect(ctx)
	if err != nil {
		return err
	}
	return c.Quit()
}

// Close drops idle connections.
func (r *Relay) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.idle {
		p.c.Quit()
	}
	r.idle = nil
}

// Direct delivers to each recipient domain's MX on port 25, using TLS when
// offered and required when the domain publishes an enforcing MTA-STS policy.
type Direct struct {
	LocalName string
	DNS       dnsx.Resolver
	STS       *STSCache
	Port      string // "25"; tests use another
	Dial      func(ctx context.Context, network, addr string) (net.Conn, error)
	// IPv6 allows connections over IPv6. Off unless the server's IPv6 is
	// configured and has its own PTR: Gmail refuses IPv6 senders without
	// reverse DNS (550 5.7.25), and SPF covers only published addresses.
	IPv6 bool
}

// Name describes the transport.
func (d *Direct) Name() string { return "direct (MX, port 25)" }

// Close is a no-op; direct connections are not pooled.
func (d *Direct) Close() {}

// Send delivers one message to rcpt's mail servers, trying each in preference order.
func (d *Direct) Send(ctx context.Context, from, rcpt string, raw []byte) error {
	at := strings.LastIndexByte(rcpt, '@')
	if at < 0 {
		return &Failure{Bounce: true, Code: 553, Message: "invalid recipient " + rcpt}
	}
	domain := rcpt[at+1:]
	hosts, err := d.mxHosts(ctx, domain)
	if err != nil {
		return err
	}
	var policy *STSPolicy
	if d.STS != nil {
		policy = d.STS.Get(ctx, domain)
	}
	var last error = &Failure{Temporary: true, Message: "no mail server reachable for " + domain}
	for _, h := range hosts {
		if policy != nil && policy.Enforce() && !policy.Matches(h) {
			last = &Failure{Temporary: true, Message: fmt.Sprintf("MX %s not allowed by %s's MTA-STS policy", h, domain)}
			continue
		}
		err := d.sendTo(ctx, h, policy != nil && policy.Enforce(), from, rcpt, raw)
		if err == nil {
			return nil
		}
		f := Classify(err)
		if !f.Temporary && !f.Handshake {
			return f // a definitive answer from the domain's server
		}
		last = f
	}
	return last
}

func (d *Direct) mxHosts(ctx context.Context, domain string) ([]string, error) {
	mxs, err := d.DNS.LookupMX(ctx, domain)
	if err != nil && !dnsx.NotFound(err) {
		return nil, &Failure{Temporary: true, Message: "MX lookup for " + domain + ": " + err.Error()}
	}
	if len(mxs) == 1 && mxs[0].Host == "." {
		return nil, &Failure{Bounce: true, Code: 556, Enhanced: "5.1.10", Message: domain + " accepts no mail (null MX)"}
	}
	if len(mxs) == 0 {
		// RFC 5321 5.1: no MX means the domain itself is the mail host.
		if _, err := d.DNS.LookupIPAddr(ctx, domain); err != nil {
			if dnsx.NotFound(err) {
				return nil, &Failure{Bounce: true, Code: 550, Enhanced: "5.1.2", Message: "domain " + domain + " does not exist"}
			}
			return nil, &Failure{Temporary: true, Message: "address lookup for " + domain + ": " + err.Error()}
		}
		return []string{domain}, nil
	}
	sort.SliceStable(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	out := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		out = append(out, strings.TrimSuffix(strings.ToLower(mx.Host), "."))
	}
	return out, nil
}

func (d *Direct) sendTo(ctx context.Context, host string, requireTLS bool, from, rcpt string, raw []byte) error {
	port := d.Port
	if port == "" {
		port = "25"
	}
	dial := d.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second}).DialContext
	}
	connect := func(cfg *tls.Config) (*smtp.Client, bool, error) {
		network := "tcp4"
		if d.IPv6 {
			network = "tcp"
		}
		conn, err := dial(ctx, network, net.JoinHostPort(host, port))
		if err != nil {
			return nil, false, &Failure{Temporary: true, Message: "connect " + host + ": " + err.Error()}
		}
		up, secure, err := upgradeConn(ctx, conn, d.LocalName, cfg)
		if err != nil {
			conn.Close()
			return nil, false, err
		}
		c := smtp.NewClient(up)
		c.CommandTimeout, c.SubmissionTimeout = 5*time.Minute, 10*time.Minute
		if err := c.Hello(d.LocalName); err != nil {
			c.Close()
			return nil, false, err
		}
		return c, secure, nil
	}
	c, secure, err := connect(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		var f *Failure
		if errors.As(err, &f) || requireTLS {
			if requireTLS && !errors.As(err, &f) {
				return &Failure{Temporary: true, Message: "TLS with " + host + " failed and MTA-STS requires it: " + err.Error()}
			}
			return err
		}
		// Opportunistic TLS: an unverifiable certificate still beats clear text.
		c, secure, err = connect(&tls.Config{ServerName: host, InsecureSkipVerify: true})
		if err != nil {
			return err
		}
	}
	defer c.Close()
	if requireTLS && !secure {
		return &Failure{Temporary: true, Message: host + " offers no STARTTLS but MTA-STS requires TLS"}
	}
	if err := transact(c, from, rcpt, raw, false); err != nil {
		return err
	}
	c.Quit()
	return nil
}

// Probe checks that outbound port 25 is reachable, the classic VPS block.
func Probe(ctx context.Context, host string) error {
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, "25"))
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	buf := make([]byte, 3)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("no greeting: %v", err)
	}
	if code, _ := strconv.Atoi(string(buf)); code != 220 {
		return fmt.Errorf("unexpected greeting %q", buf)
	}
	return nil
}
