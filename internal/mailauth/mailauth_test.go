package mailauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/msg"
)

func setup(t *testing.T) (*dnsxtest.Fake, []Key, []byte) {
	t.Helper()
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	epub, ek, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&rk.PublicKey)
	dns := dnsxtest.New()
	dns.SetTXT("s1r._domainkey.example.com", "v=DKIM1; k=rsa; p="+base64.StdEncoding.EncodeToString(der))
	dns.SetTXT("s1e._domainkey.example.com", "v=DKIM1; k=ed25519; p="+base64.StdEncoding.EncodeToString(epub))
	dns.SetTXT("example.com", "v=spf1 ip4:203.0.113.5 -all")
	dns.SetTXT("_dmarc.example.com", "v=DMARC1; p=reject")
	raw, err := msg.Compose(msg.Draft{
		From: msg.Address{Email: "news@example.com"}, To: []msg.Address{{Email: "a@b.org"}},
		Subject: "hi", Text: "hello", HTML: "<p>hello</p>",
		Header: [][2]string{{"List-Unsubscribe", "<https://example.com/u/x>"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return dns, []Key{{"s1e", ek}, {"s1r", rk}}, raw
}

func TestSignVerifyPass(t *testing.T) {
	dns, keys, raw := setup(t)
	signed, err := SignHeaders(raw, "example.com", keys, time.Now(), SignedHeaders)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(signed), "DKIM-Signature:") != 2 {
		t.Fatal("want two signatures")
	}
	r := Verifier{DNS: dns}.Verify(context.Background(), signed, net.ParseIP("203.0.113.5"), "mail.example.com", "bounce@example.com", "example.com")
	if r.SPF != "pass" || len(r.DKIM) != 2 || r.DKIM[0].Result != "pass" || r.DKIM[1].Result != "pass" || r.DMARC != "pass" || r.DMARCPolicy != "reject" {
		t.Fatalf("result %+v", r)
	}
	if !r.DKIMPass("example.com") {
		t.Fatal("DKIMPass")
	}
	if h := r.Header("mx.test"); !strings.Contains(h, "dmarc=pass header.from=example.com") {
		t.Fatal(h)
	}
}

func TestDMARCViaDKIMOnlyThroughRelay(t *testing.T) {
	dns, keys, raw := setup(t)
	signed, _ := SignHeaders(raw, "example.com", keys, time.Now(), SignedHeaders)
	// A relay uses its own envelope sender and IP: SPF is about the relay, DKIM still aligns.
	dns.SetTXT("relay.net", "v=spf1 ip4:198.51.100.1 -all")
	r := Verifier{DNS: dns}.Verify(context.Background(), signed, net.ParseIP("198.51.100.1"), "out.relay.net", "b123@relay.net", "example.com")
	if r.SPF != "pass" || r.DMARC != "pass" {
		t.Fatalf("result %+v", r)
	}
}

func TestTamperedAndSpoofed(t *testing.T) {
	dns, keys, raw := setup(t)
	signed, _ := SignHeaders(raw, "example.com", keys, time.Now(), SignedHeaders)
	tampered := []byte(strings.Replace(string(signed), "Subject: hi", "Subject: HI", 1))
	r := Verifier{DNS: dns}.Verify(context.Background(), tampered, net.ParseIP("192.0.2.66"), "evil", "x@example.com", "example.com")
	if r.SPF != "fail" || r.DMARC != "fail" || r.DKIMPass("example.com") {
		t.Fatalf("result %+v", r)
	}
}

func TestNoPolicyAndTempError(t *testing.T) {
	dns, _, raw := setup(t)
	r := Verifier{DNS: dns}.Verify(context.Background(), raw, net.ParseIP("192.0.2.1"), "h", "a@other.org", "other.org")
	if r.DMARC != "none" || r.SPF != "none" {
		t.Fatalf("result %+v", r)
	}
	dns.Fail["_dmarc.flaky.org"] = true
	r = Verifier{DNS: dns}.Verify(context.Background(), raw, net.ParseIP("192.0.2.1"), "h", "a@flaky.org", "flaky.org")
	if r.DMARC != "temperror" {
		t.Fatalf("result %+v", r)
	}
}

func TestAligned(t *testing.T) {
	if !Aligned("mail.example.co.uk", "example.co.uk", false) || Aligned("mail.example.co.uk", "example.co.uk", true) || Aligned("a.com", "b.com", false) {
		t.Fatal("alignment")
	}
}
