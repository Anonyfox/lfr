package certs

import (
	"context"
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestFallbackAndPolicy(t *testing.T) {
	m := &Manager{
		Dir:      t.TempDir(),
		Email:    func(context.Context) string { return "" },
		DirURL:   func(context.Context) string { return "http://127.0.0.1:1/directory" }, // unreachable
		Allowed:  func(_ context.Context, h string) bool { return h == "mail.example.com" || h == "news.example.com" },
		Hostname: func(context.Context) string { return "mail.example.com" },
	}
	c, err := m.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil || c.Leaf.Subject.CommonName != "mail.example.com" {
		t.Fatalf("no-SNI fallback %v %v", c, err)
	}
	again, _ := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "MAIL.example.com"})
	if again != c {
		t.Fatal("self-signed certificate not reused")
	}
	if _, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: "evil.example.org"}); err == nil {
		t.Fatal("certificate for a foreign host")
	}
	if st := m.Inspect("news.example.com"); st.Valid {
		t.Fatal("no certificate cached yet")
	}
	h := m.Challenge(nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://news.example.com/.well-known/acme-challenge/unknown", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown challenge %d", rec.Code)
	}
}

func TestSMTPCertificateFallsBackToMailHost(t *testing.T) {
	m := &Manager{Dir: t.TempDir(), Email: func(context.Context) string { return "" },
		DirURL:   func(context.Context) string { return "http://127.0.0.1:1/directory" },
		Allowed:  func(_ context.Context, h string) bool { return h == "mail.example.com" },
		Hostname: func(context.Context) string { return "mail.example.com" }}
	c, err := m.SMTPCertificate(&tls.ClientHelloInfo{ServerName: "example.com"})
	if err != nil || c.Leaf.Subject.CommonName != "mail.example.com" {
		t.Fatalf("%v %v", c, err)
	}
}
