package mailauth

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDMARCPolicyInheritanceAndDKIMKeyFailures(t *testing.T) {
	dns, keys, raw := setup(t)
	signed, err := SignHeaders(raw, "example.com", keys, time.Now(), SignedHeaders)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, policy, from, mailfrom string
		unsigned                     bool
		result, wantPolicy           string
	}{
		{"relaxed subdomain", "v=DMARC1; p=reject; sp=quarantine", "news.example.com", "bounce@other.org", false, "pass", "quarantine"},
		{"strict subdomain", "v=DMARC1; p=reject; sp=quarantine; adkim=s", "news.example.com", "bounce@other.org", false, "fail", "quarantine"},
		{"zero percent reject", "v=DMARC1; p=reject; pct=0", "example.com", "bounce@other.org", true, "fail", "quarantine"},
		{"zero percent quarantine", "v=DMARC1; p=quarantine; pct=0", "example.com", "bounce@other.org", true, "fail", "none"},
		{"malformed policy", "v=DMARC1; p=invalid", "example.com", "bounce@other.org", true, "permerror", ""},
		{"no From domain", "v=DMARC1; p=reject", "", "", true, "none", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dns.SetTXT("_dmarc.example.com", tc.policy)
			message := signed
			if tc.unsigned {
				message = raw
			}
			r := Verifier{DNS: dns}.Verify(context.Background(), message, net.ParseIP("192.0.2.1"), "example.com", tc.mailfrom, tc.from)
			if r.DMARC != tc.result || r.DMARCPolicy != tc.wantPolicy {
				t.Fatalf("%+v", r)
			}
			if tc.mailfrom == "" && !strings.Contains(r.Header("mx.example"), "smtp.helo=example.com") {
				t.Fatal("null sender did not authenticate HELO")
			}
		})
	}
	dns.SetTXT("_dmarc.example.com", "v=DMARC1; p=reject")
	for _, tc := range []struct {
		name      string
		temporary bool
		want      string
	}{{"missing keys", false, "permerror"}, {"DNS failure", true, "temperror"}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{"s1r._domainkey.example.com", "s1e._domainkey.example.com"} {
				delete(dns.TXT, key)
				dns.Fail[key] = tc.temporary
			}
			r := Verifier{DNS: dns}.Verify(context.Background(), signed, net.ParseIP("192.0.2.1"), "h", "bounce@other.org", "example.com")
			if len(r.DKIM) != 2 {
				t.Fatalf("signatures missing: %+v", r)
			}
			for _, d := range r.DKIM {
				if d.Result != tc.want || d.Error == "" {
					t.Fatalf("key failure: %+v", r)
				}
			}
			if tc.temporary && r.DMARC != "temperror" {
				t.Fatalf("temporary key failure rejected permanently: %+v", r)
			}
		})
	}
}
