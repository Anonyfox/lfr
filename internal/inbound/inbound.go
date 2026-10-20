// Package inbound is the SMTP server on port 25: it receives mail for lfr's
// domains, refuses abuse as early and cheaply as possible, authenticates and
// scores what remains, and hands accepted messages to the engine.
package inbound

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-smtp"

	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/dnsbl"
	"github.com/Anonyfox/lfr/internal/dnsx"
	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/spam"
)

// Envelope is an accepted message.
type Envelope struct {
	From    string
	Rcpts   []string
	Raw     []byte // with lfr's Received and Authentication-Results fields
	Parsed  *msg.Parsed
	Auth    mailauth.Result
	Verdict spam.Verdict
	IP      net.IP
	Helo    string
	TLS     bool
}

// Server receives mail.
type Server struct {
	Hostname func(ctx context.Context) string
	Config   config.Config
	DNS      dnsx.Resolver
	DNSBL    *dnsbl.Checker
	Log      *slog.Logger
	TLS      *tls.Config
	Now      func() time.Time

	// Accept decides at RCPT time whether lfr takes mail for an address.
	Accept func(ctx context.Context, rcpt string) (bool, error)
	// System reports addresses lfr handles itself (bounces, reports), which skip spam scoring.
	System func(rcpt string) bool
	// Classify scores a message; KnownContact reports earlier correspondence.
	Classify     func(ctx context.Context, in spam.Input) spam.Verdict
	KnownContact func(ctx context.Context, email string) bool
	// Local reports whether lfr serves a domain.
	Local func(ctx context.Context, domain string) bool
	// Deliver stores or processes an accepted message.
	Deliver func(ctx context.Context, e *Envelope) error

	srv  *smtp.Server
	gate gate
	ctx  context.Context
	busy chan struct{} // bounds messages held in memory at once
}

// concurrentData is how many messages are read and checked at the same time;
// with the size limit it caps the memory incoming mail can take.
const concurrentData = 4

// Serve accepts connections on l until ctx ends.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	s.ctx = ctx
	s.busy = make(chan struct{}, concurrentData)
	s.srv = smtp.NewServer(s)
	s.srv.Domain = s.Hostname(ctx)
	s.srv.ReadTimeout = 5 * time.Minute // RFC 5321 §4.5.3.2.7
	s.srv.WriteTimeout = 5 * time.Minute
	s.srv.MaxMessageBytes = int64(s.Config.Int(ctx, "smtp.max_size"))
	s.srv.MaxRecipients = 50
	s.srv.MaxLineLength = 4000
	s.srv.EnableSMTPUTF8 = true
	s.srv.TLSConfig = s.TLS
	s.srv.ErrorLog = errorLog{s.Log}
	gl := &gatedListener{Listener: l, s: s}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.srv.Shutdown(sctx)
	}()
	err := s.srv.Serve(gl)
	if ctx.Err() != nil || errors.Is(err, smtp.ErrServerClosed) {
		return nil
	}
	return err
}

type errorLog struct{ l *slog.Logger }

func (e errorLog) Printf(format string, v ...any) { e.l.Debug("smtp: " + fmt.Sprintf(format, v...)) }
func (e errorLog) Println(v ...any)               { e.l.Debug("smtp: " + fmt.Sprint(v...)) }

// gate enforces connection limits per server and per IP.
type gate struct {
	mu     sync.Mutex
	total  int
	perIP  map[string]int
	recent map[string][]time.Time
}

func (g *gate) enter(ip string, maxTotal, maxPerIP, perHour int, now time.Time) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.perIP == nil {
		g.perIP, g.recent = map[string]int{}, map[string][]time.Time{}
	}
	if g.total >= maxTotal {
		return false, "server busy"
	}
	if g.perIP[ip] >= maxPerIP {
		return false, "too many connections from your address"
	}
	cut := now.Add(-time.Hour)
	times := g.recent[ip]
	for len(times) > 0 && times[0].Before(cut) {
		times = times[1:]
	}
	if len(times) >= perHour {
		g.recent[ip] = times
		return false, "too many connections from your address in the last hour"
	}
	g.recent[ip] = append(times, now)
	if len(g.recent) > 20000 {
		for k, v := range g.recent {
			if len(v) == 0 || v[len(v)-1].Before(cut) {
				delete(g.recent, k)
			}
		}
	}
	g.total++
	g.perIP[ip]++
	return true, ""
}

func (g *gate) leave(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.total--
	if g.perIP[ip]--; g.perIP[ip] <= 0 {
		delete(g.perIP, ip)
	}
}

type gatedListener struct {
	net.Listener
	s *Server
}

func (l *gatedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		ip := remoteIP(c)
		ctx := l.s.ctx
		ok, why := l.s.gate.enter(ip, l.s.Config.Int(ctx, "smtp.max_conns"), l.s.Config.Int(ctx, "smtp.max_conns_per_ip"), l.s.Config.Int(ctx, "smtp.rate_per_ip"), time.Now())
		if !ok {
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			io.WriteString(c, "421 4.7.0 "+why+", try again later\r\n")
			c.Close()
			l.s.Log.Info("smtp refused", "ip", ip, "reason", why)
			continue
		}
		gc := &gatedConn{Conn: c, release: func() { l.s.gate.leave(ip) }}
		if l.s.Config.Bool(ctx, "smtp.early_talker") {
			gc.earlyCheck = true
		}
		return gc, nil
	}
}

// gatedConn releases its gate slot on close and, before the greeting,
// waits briefly to catch clients that talk first: real mail servers wait
// for the banner, many spam bots do not.
type gatedConn struct {
	net.Conn
	release    func()
	once       sync.Once
	earlyCheck bool
	greeted    bool
	early      bool
}

func (c *gatedConn) Write(p []byte) (int, error) {
	if !c.greeted {
		c.greeted = true
		if c.earlyCheck {
			c.Conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var one [1]byte
			n, _ := c.Conn.Read(one[:])
			c.Conn.SetReadDeadline(time.Time{})
			if n > 0 {
				c.early = true
				io.WriteString(c.Conn, "554 5.7.1 Talking before the greeting is not allowed\r\n")
				c.Conn.Close()
				return 0, errors.New("early talker")
			}
		}
	}
	return c.Conn.Write(p)
}

func (c *gatedConn) Close() error {
	c.once.Do(c.release)
	return c.Conn.Close()
}

func remoteIP(c net.Conn) string {
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return c.RemoteAddr().String()
	}
	return host
}

func underlying(c net.Conn) net.Conn {
	if tc, ok := c.(*tls.Conn); ok {
		return tc.NetConn()
	}
	return c
}

// NewSession implements smtp.Backend.
func (s *Server) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip := net.ParseIP(remoteIP(underlying(c.Conn())))
	return &session{s: s, conn: c, ip: ip}, nil
}

type session struct {
	s       *Server
	conn    *smtp.Conn
	ip      net.IP
	from    string
	rcpts   []string
	unknown int
}

func (x *session) Reset()        { x.from, x.rcpts = "", nil }
func (x *session) Logout() error { return nil }

func (x *session) Mail(from string, _ *smtp.MailOptions) error {
	if x.conn.Hostname() == "" {
		return &smtp.SMTPError{Code: 503, EnhancedCode: smtp.EnhancedCode{5, 5, 1}, Message: "say HELO first"}
	}
	x.from = strings.ToLower(from)
	return nil
}

func (x *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	ctx := x.s.ctx
	ok, err := x.s.Accept(ctx, to)
	if err != nil {
		x.s.Log.Error("rcpt lookup", "rcpt", to, "err", err)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "temporary failure, try again later"}
	}
	if !ok {
		x.unknown++
		if x.unknown >= 5 {
			// Address guessing: drop the connection.
			x.s.Log.Info("smtp dictionary attack", "ip", x.ip)
			x.conn.Close()
		}
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user here"}
	}
	x.rcpts = append(x.rcpts, strings.ToLower(to))
	return nil
}

var errSpam = &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "message refused as spam"}

// maxHops is the number of Received fields from which a message counts as
// looping (RFC 5321 §6.3).
const maxHops = 50

func (x *session) Data(r io.Reader) error {
	select {
	case x.s.busy <- struct{}{}:
		defer func() { <-x.s.busy }()
	case <-time.After(time.Minute):
		io.Copy(io.Discard, r)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 2}, Message: "busy, try again later"}
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// The clock for checks and delivery starts after the transfer, which may
	// take minutes for a large message on a slow link.
	ctx, cancel := context.WithTimeout(x.s.ctx, 2*time.Minute)
	defer cancel()
	raw = CRLF(raw)
	if len(x.rcpts) == 0 {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 5, 1}, Message: "no valid recipients"}
	}
	helo := x.conn.Hostname()
	_, isTLS := x.conn.TLSConnectionState()
	p := msg.Parse(raw)
	if len(p.Header.Values("Received")) >= maxHops {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 4, 6}, Message: "too many hops, mail loop"}
	}
	if name := spam.Executable(p); name != "" {
		x.s.Log.Info("smtp refused executable", "ip", x.ip, "file", name)
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "executable attachments are refused"}
	}
	fromDomain := ""
	if at := strings.LastIndexByte(p.From.Email, '@'); at >= 0 {
		fromDomain = p.From.Email[at+1:]
	}
	auth := mailauth.Verifier{DNS: x.s.DNS}.Verify(ctx, raw, x.ip, helo, x.from, fromDomain)
	system := true
	for _, rc := range x.rcpts {
		if x.s.System == nil || !x.s.System(rc) {
			system = false
		}
	}
	e := &Envelope{From: x.from, Rcpts: x.rcpts, Parsed: p, Auth: auth, IP: x.ip, Helo: helo, TLS: isTLS}
	var ptr string
	if !system {
		// One From with one address: with more, readers may see an address
		// DMARC never checked (RFC 7489 §6.6.1).
		if from := p.Header.Values("From"); len(from) != 1 || strings.Count(from[0], "@") != 1 || p.From.Email == "" {
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "a message needs exactly one From address"}
		}
		if auth.DMARC == "temperror" && auth.DMARCPolicy == "reject" {
			return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 7, 5}, Message: "DMARC check failed temporarily, try again later"}
		}
		if auth.DMARC == "fail" && auth.DMARCPolicy == "reject" {
			x.s.Log.Info("smtp refused by DMARC", "ip", x.ip, "from", fromDomain)
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "DMARC policy of " + fromDomain + " says to reject this message"}
		}
		ptrName, confirmed := x.reverse(ctx)
		ptr = ptrName
		in := spam.Input{IP: x.ip, Helo: helo, PTR: ptrName, PTRConfirmed: confirmed, Auth: auth, Parsed: p, Received: x.s.Now()}
		in.OwnDomain = x.s.Local != nil && fromDomain != "" && x.s.Local(ctx, fromDomain)
		in.DNSBL = x.s.DNSBL.Check(ctx, x.ip, x.s.Config.List(ctx, "spam.dnsbl"))
		if x.s.KnownContact != nil && p.From.Email != "" && auth.DMARC == "pass" {
			in.KnownContact = x.s.KnownContact(ctx, p.From.Email)
		}
		e.Verdict = x.s.Classify(ctx, in)
		if e.Verdict.P >= x.s.Config.Float(ctx, "spam.reject") {
			x.s.Log.Info("smtp refused spam", "ip", x.ip, "from", p.From.Email, "p", e.Verdict.P, "hits", strings.Join(e.Verdict.Hits, " "))
			return errSpam
		}
	}
	e.Raw = x.trace(raw, helo, ptr, isTLS, auth)
	if err := x.s.Deliver(ctx, e); err != nil {
		x.s.Log.Error("deliver", "err", err)
		return &smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 3, 0}, Message: "temporary failure, try again later"}
	}
	return nil
}

// reverse returns the client's PTR name and whether it resolves back.
func (x *session) reverse(ctx context.Context) (string, bool) {
	names, err := x.s.DNS.LookupAddr(ctx, x.ip.String())
	if err != nil || len(names) == 0 {
		return "", false
	}
	name := strings.TrimSuffix(names[0], ".")
	addrs, err := x.s.DNS.LookupIPAddr(ctx, name)
	if err != nil {
		return name, false
	}
	for _, a := range addrs {
		if a.IP.Equal(x.ip) {
			return name, true
		}
	}
	return name, false
}

// trace prepends Received and Authentication-Results, removing any
// Authentication-Results for our host name that the sender forged.
func (x *session) trace(raw []byte, helo, ptr string, isTLS bool, auth mailauth.Result) []byte {
	host := x.s.Hostname(x.s.ctx)
	with := "ESMTP"
	if isTLS {
		with = "ESMTPS"
	}
	literal := x.ip.String()
	if x.ip.To4() == nil {
		literal = "IPv6:" + literal // RFC 5321 §4.1.3
	}
	from := helo
	if ptr != "" {
		from += " (" + ptr + " [" + literal + "])"
	} else {
		from += " ([" + literal + "])"
	}
	var b bytes.Buffer
	b.Grow(len(raw) + 512)
	// The final delivery records the envelope sender (RFC 5321 §4.4).
	fmt.Fprintf(&b, "Return-Path: <%s>\r\n", sanitize(x.from))
	fmt.Fprintf(&b, "Authentication-Results: %s\r\n", auth.Header(host))
	fmt.Fprintf(&b, "Received: from %s\r\n\tby %s (lfr) with %s; %s\r\n", sanitize(from), host, with, x.s.Now().Format(time.RFC1123Z))
	b.Write(stripForged(raw, host))
	return b.Bytes()
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// stripForged removes Authentication-Results fields whose authserv-id is
// host (RFC 8601 §5), and any Return-Path, which only the final delivery
// writes. raw has CRLF line endings.
func stripForged(raw []byte, host string) []byte {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		end = len(raw) - 2
		if end < 0 {
			return raw
		}
	}
	lower := bytes.ToLower(raw[:end])
	if !bytes.Contains(lower, []byte("authentication-results:")) && !bytes.Contains(lower, []byte("return-path:")) {
		return raw
	}
	var out bytes.Buffer
	out.Grow(len(raw))
	skip := false
	for _, l := range bytes.SplitAfter(raw[:end+2], []byte("\n")) {
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') {
			if !skip {
				out.Write(l)
			}
			continue
		}
		skip = false
		name, value, ok := strings.Cut(string(l), ":")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "return-path":
			skip = ok
		case "authentication-results":
			// The authserv-id is the first token of the unfolded value.
			id, _, _ := strings.Cut(unfold(raw[:end+2], l, value), ";")
			id = strings.TrimSpace(id)
			if i := strings.IndexAny(id, " \t"); i >= 0 {
				id = id[:i]
			}
			skip = ok && strings.EqualFold(id, host)
		}
		if !skip {
			out.Write(l)
		}
	}
	out.Write(raw[end+2:])
	return out.Bytes()
}

// unfold returns the value of the field starting with line within header,
// with its continuation lines joined.
func unfold(header, line []byte, value string) string {
	i := bytes.Index(header, line)
	if i < 0 {
		return value
	}
	rest := header[i+len(line):]
	var b strings.Builder
	b.WriteString(strings.TrimRight(value, "\r\n"))
	for _, l := range bytes.SplitAfter(rest, []byte("\n")) {
		if len(l) == 0 || l[0] != ' ' && l[0] != '\t' {
			break
		}
		b.WriteString(" ")
		b.WriteString(strings.TrimSpace(string(l)))
	}
	return b.String()
}

// CRLF turns every bare LF and bare CR into CRLF, so a message cannot carry
// line endings that a later hop reads differently (RFC 5321 §2.3.8; the
// SMTP smuggling class of attacks), and parsing sees one header boundary.
func CRLF(raw []byte) []byte {
	bare := false
	for i, c := range raw {
		if c == '\n' && (i == 0 || raw[i-1] != '\r') || c == '\r' && (i+1 == len(raw) || raw[i+1] != '\n') {
			bare = true
			break
		}
	}
	if !bare {
		return raw
	}
	out := make([]byte, 0, len(raw)+len(raw)/40)
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case c == '\r' && i+1 < len(raw) && raw[i+1] == '\n':
			out = append(out, '\r', '\n')
			i++
		case c == '\r' || c == '\n':
			out = append(out, '\r', '\n')
		default:
			out = append(out, c)
		}
	}
	return out
}
