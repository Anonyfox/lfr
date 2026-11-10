package engine

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
)

func TestPlainHTTPRedirectsOnlyServedHosts(t *testing.T) {
	h := start(t)
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	// No listeners use this option after startup; it controls HTTP policy.
	// Exercise the handler directly to avoid binding public HTTP/TLS ports.
	for _, tc := range []struct {
		name, target, remote string
		headers              bool
		status               int
		location             string
	}{
		{"mail host", "http://mail.example.com:80/lists/plain/?page=2", "192.0.2.1:1234", false, 308, "https://mail.example.com/lists/plain/?page=2"},
		{"mta sts", "http://mta-sts.example.com/.well-known/mta-sts.txt", "192.0.2.1:1234", false, 308, "https://mta-sts.example.com/.well-known/mta-sts.txt"},
		{"unserved host", "http://attacker.org/", "192.0.2.1:1234", false, 404, ""},
		{"health probe", "http://mail.example.com/healthz", "192.0.2.1:1234", false, 200, ""},
		{"forged proxy headers", "http://mail.example.com/", "192.0.2.1:1234", true, 308, "https://mail.example.com/"},
		{"trusted proxy", "http://mail.example.com/", "173.245.48.1:1234", true, 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.target, nil)
			req.RemoteAddr = tc.remote
			if tc.headers {
				req.Header.Set("CF-Connecting-IP", "198.51.100.2")
				req.Header.Set("CF-Ray", "abc")
			}
			rec := httptest.NewRecorder()
			h.e.plainHTTP(rec, req)
			if rec.Code != tc.status || rec.Header().Get("Location") != tc.location {
				t.Fatalf("%d %v %s", rec.Code, rec.Header(), rec.Body.String())
			}
		})
	}
	policy, ok := h.e.mtaSTS(context.Background(), "example.com")
	if !ok || !strings.Contains(policy, "mx: mail.example.com") {
		t.Fatalf("policy: %q %v", policy, ok)
	}
	if _, ok := h.e.mtaSTS(context.Background(), "attacker.org"); ok {
		t.Fatal("foreign policy served")
	}
}
