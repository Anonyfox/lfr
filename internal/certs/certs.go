// Package certs provides TLS certificates from Let's Encrypt for every host
// lfr serves (mail host, list websites, MTA-STS hosts), renewing them
// automatically, with a self-signed fallback so STARTTLS keeps working before
// DNS is in place.
package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Manager hands out certificates.
type Manager struct {
	Dir      string
	Email    func(ctx context.Context) string
	DirURL   func(ctx context.Context) string
	Allowed  func(ctx context.Context, host string) bool
	Hostname func(ctx context.Context) string

	mu       sync.Mutex
	ac       *autocert.Manager
	acKey    string
	fallback map[string]*tls.Certificate
	failed   map[string]time.Time // hosts whose last issuance failed, to respect ACME rate limits
}

// retryAfter is how long a failed issuance is not retried.
const retryAfter = 30 * time.Minute

func (m *Manager) autocert(ctx context.Context) *autocert.Manager {
	email, dir := m.Email(ctx), m.DirURL(ctx)
	key := email + "|" + dir
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ac != nil && m.acKey == key {
		return m.ac
	}
	m.ac = &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache(m.Dir),
		Email:  email,
		HostPolicy: func(ctx context.Context, host string) error {
			if m.Allowed(ctx, host) {
				return nil
			}
			return errors.New("host not served by lfr: " + host)
		},
		Client:      &acme.Client{DirectoryURL: dir},
		RenewBefore: 30 * 24 * time.Hour,
	}
	m.acKey = key
	return m.ac
}

// GetCertificate serves HTTPS and SMTP STARTTLS. Clients without SNI (most
// mail servers) get the mail host's certificate. When no real certificate
// can be had for an allowed host, a self-signed one keeps TLS available.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ctx := context.Background()
	if hello.Context() != nil {
		ctx = hello.Context()
	}
	name := strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))
	if name == "" {
		name = m.Hostname(ctx)
		h := *hello
		h.ServerName = name
		hello = &h
	}
	if !m.Allowed(ctx, name) {
		return nil, errors.New("unknown host " + name)
	}
	m.mu.Lock()
	failedAt, failed := m.failed[name]
	m.mu.Unlock()
	if failed && time.Since(failedAt) < retryAfter {
		if cert, err := m.cached(ctx, hello); err == nil {
			return cert, nil
		}
		return m.selfSigned(name), nil
	}
	actx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cert, err := m.autocert(actx).GetCertificate(hello)
	if err == nil {
		return cert, nil
	}
	m.mu.Lock()
	if m.failed == nil {
		m.failed = map[string]time.Time{}
	}
	m.failed[name] = time.Now()
	m.mu.Unlock()
	return m.selfSigned(name), nil
}

// cached returns a certificate autocert already holds, without contacting the CA.
func (m *Manager) cached(ctx context.Context, hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	st := m.Inspect(hello.ServerName)
	if !st.Valid {
		return nil, errors.New("none cached")
	}
	return m.autocert(ctx).GetCertificate(hello)
}

// Forget clears remembered failures, e.g. after DNS was fixed.
func (m *Manager) Forget() {
	m.mu.Lock()
	m.failed = nil
	m.mu.Unlock()
}

func (m *Manager) selfSigned(host string) *tls.Certificate {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fallback == nil {
		m.fallback = map[string]*tls.Certificate{}
	}
	if c, ok := m.fallback[host]; ok && time.Now().Before(c.Leaf.NotAfter.Add(-time.Hour)) {
		return c
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	leaf, _ := x509.ParseCertificate(der)
	c := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
	m.fallback[host] = c
	return c
}

// SMTPCertificate is GetCertificate for STARTTLS: mail servers send all
// kinds of names (or none), and a failed handshake makes some deliver in clear
// text, so any name it does not serve gets the mail host's certificate.
func (m *Manager) SMTPCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	ctx := context.Background()
	if hello.Context() != nil {
		ctx = hello.Context()
	}
	if hello.ServerName == "" || !m.Allowed(ctx, strings.ToLower(hello.ServerName)) {
		h := *hello
		h.ServerName = m.Hostname(ctx)
		hello = &h
	}
	return m.GetCertificate(hello)
}

// TLSConfig is the server configuration for HTTPS.
func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1", acme.ALPNProto},
	}
}

// Challenge answers ACME HTTP-01 challenges and passes everything else to next.
// It works on port 80 and, behind a proxy that upgrades to HTTPS, on 443 too.
func (m *Manager) Challenge(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			m.autocert(r.Context()).HTTPHandler(nil).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// State is the certificate status of one host.
type State struct {
	Host    string    `json:"host"`
	Valid   bool      `json:"valid"`
	Expires time.Time `json:"expires,omitzero"`
	Issuer  string    `json:"issuer,omitempty"`
}

// Inspect reports the cached certificate of host without requesting one.
func (m *Manager) Inspect(host string) State {
	st := State{Host: host}
	for _, name := range []string{host, host + "+rsa"} {
		b, err := os.ReadFile(filepath.Join(m.Dir, name))
		if err != nil {
			continue
		}
		for len(b) > 0 {
			var block []byte
			block, b = pemBlock(b)
			if block == nil {
				break
			}
			c, err := x509.ParseCertificate(block)
			if err != nil {
				continue
			}
			if c.IsCA {
				continue
			}
			st.Valid = time.Now().Before(c.NotAfter)
			st.Expires = c.NotAfter.UTC()
			st.Issuer = c.Issuer.CommonName
			return st
		}
	}
	return st
}
