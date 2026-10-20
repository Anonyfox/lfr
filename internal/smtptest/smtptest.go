// Package smtptest runs a recording SMTP server for tests: STARTTLS or
// implicit TLS with a throwaway certificate, optional AUTH PLAIN, and
// scripted rejections.
package smtptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// Message is one received message.
type Message struct {
	From string
	To   []string
	Data []byte
	Helo string
	TLS  bool
	User string
}

// Server records what it receives.
type Server struct {
	Addr     string
	User     string // when set, AUTH PLAIN with Password is required
	Password string
	// Reject returns an error for a recipient, e.g. &smtp.SMTPError{Code: 550, ...}.
	Reject func(rcpt string) error
	// RejectMail and RejectData refuse at MAIL FROM and after DATA.
	RejectMail func(from string) error
	RejectData func() error

	mu       sync.Mutex
	messages []Message
	srv      *smtp.Server
}

// Cert makes a self-signed certificate for host.
func Cert(host string) tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// Start runs a server on a random local port until the test ends. With
// implicit set, TLS starts at connect time; otherwise STARTTLS is offered.
func Start(t testing.TB, implicit bool, configure func(*Server)) *Server {
	t.Helper()
	s := &Server{}
	if configure != nil {
		configure(s)
	}
	s.srv = smtp.NewServer(s)
	s.srv.Domain = "relay.test"
	s.srv.AllowInsecureAuth = false
	s.srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{Cert("relay.test")}}
	var l net.Listener
	var err error
	if implicit {
		l, err = tls.Listen("tcp", "127.0.0.1:0", s.srv.TLSConfig)
	} else {
		l, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = l.Addr().String()
	go s.srv.Serve(l)
	t.Cleanup(func() { s.srv.Close() })
	return s
}

// Messages returns everything received so far.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// NewSession implements smtp.Backend.
func (s *Server) NewSession(c *smtp.Conn) (smtp.Session, error) {
	_, isTLS := c.TLSConnectionState()
	return &session{s: s, helo: c.Hostname(), conn: c, tls: isTLS}, nil
}

type session struct {
	s    *Server
	conn *smtp.Conn
	helo string
	tls  bool
	user string
	from string
	to   []string
}

func (x *session) AuthMechanisms() []string {
	if x.s.User == "" {
		return nil
	}
	return []string{sasl.Plain}
}

func (x *session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, user, pass string) error {
		if user != x.s.User || pass != x.s.Password {
			return errors.New("bad credentials")
		}
		x.user = user
		return nil
	}), nil
}

func (x *session) Mail(from string, _ *smtp.MailOptions) error {
	if x.s.User != "" && x.user == "" {
		return smtp.ErrAuthRequired
	}
	if x.s.RejectMail != nil {
		if err := x.s.RejectMail(from); err != nil {
			return err
		}
	}
	x.from = from
	return nil
}

func (x *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if x.s.Reject != nil {
		if err := x.s.Reject(to); err != nil {
			return err
		}
	}
	x.to = append(x.to, to)
	return nil
}

func (x *session) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if x.s.RejectData != nil {
		if err := x.s.RejectData(); err != nil {
			return err
		}
	}
	_, isTLS := x.conn.TLSConnectionState()
	x.s.mu.Lock()
	x.s.messages = append(x.s.messages, Message{From: x.from, To: x.to, Data: b, Helo: strings.ToLower(x.conn.Hostname()), TLS: isTLS || x.tls, User: x.user})
	x.s.mu.Unlock()
	return nil
}

func (x *session) Reset()        { x.from, x.to = "", nil }
func (x *session) Logout() error { return nil }
