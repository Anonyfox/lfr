package feedback

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/msg"
)

const dsn = "From: Mail Delivery Subsystem <mailer-daemon@googlemail.com>\r\n" +
	"To: bounce+abc@example.com\r\n" +
	"Subject: Delivery Status Notification (Failure)\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/report; report-type=delivery-status; boundary=\"B\"\r\n\r\n" +
	"--B\r\nContent-Type: text/plain\r\n\r\nYour message wasn't delivered.\r\n" +
	"--B\r\nContent-Type: message/delivery-status\r\n\r\n" +
	"Reporting-MTA: dns; googlemail.com\r\nArrival-Date: Mon, 19 Oct 2026 10:00:00 -0700\r\n\r\n" +
	"Final-Recipient: rfc822; gone@gmail.com\r\nAction: failed\r\nStatus: 5.1.1\r\n" +
	"Diagnostic-Code: smtp; 550-5.1.1 The email account that you tried to reach does not exist.\r\n\r\n" +
	"Final-Recipient: rfc822; full@gmail.com\r\nAction: delayed\r\nStatus: 4.2.2\r\n\r\n" +
	"--B\r\nContent-Type: text/rfc822-headers\r\n\r\nFrom: news@example.com\r\nMessage-ID: <tok123@example.com>\r\nSubject: hi\r\n\r\n" +
	"--B--\r\n"

func TestDSN(t *testing.T) {
	rcpts, orig, ok := DSN(msg.Parse([]byte(dsn)))
	if !ok || len(rcpts) != 2 || orig != "tok123@example.com" {
		t.Fatalf("%v %q %v", rcpts, orig, ok)
	}
	if !rcpts[0].Hard() || rcpts[0].Email != "gone@gmail.com" || rcpts[0].Status != "5.1.1" || rcpts[1].Hard() {
		t.Fatalf("%+v", rcpts)
	}
	if _, _, ok := DSN(msg.Parse([]byte("Subject: hi\r\n\r\nplain"))); ok {
		t.Fatal("plain message read as DSN")
	}
}

const arf = "From: staff@hotmail.com\r\nSubject: complaint\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/report; report-type=feedback-report; boundary=\"F\"\r\n\r\n" +
	"--F\r\nContent-Type: text/plain\r\n\r\nThis is an email abuse report.\r\n" +
	"--F\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: abuse\r\nUser-Agent: Hotmail\r\nVersion: 1\r\n\r\n" +
	"--F\r\nContent-Type: message/rfc822\r\n\r\nFrom: news@example.com\r\nTo: reader@hotmail.com\r\nMessage-ID: <o42@example.com>\r\n" +
	"List-Unsubscribe: <https://news.example.com/u/TOKEN>\r\nSubject: issue\r\n\r\nbody\r\n--F--\r\n"

func TestARF(t *testing.T) {
	c, ok := ARF(msg.Parse([]byte(arf)))
	if !ok || c.Type != "abuse" || c.Recipient != "reader@hotmail.com" || c.OriginalID != "o42@example.com" || !strings.Contains(c.ListUnsub, "/u/TOKEN") {
		t.Fatalf("%+v %v", c, ok)
	}
}

const dmarcXMLDoc = `<?xml version="1.0"?><feedback><report_metadata><org_name>google.com</org_name><report_id>123</report_id>
<date_range><begin>1760832000</begin><end>1760918399</end></date_range></report_metadata>
<policy_published><domain>example.com</domain><p>none</p></policy_published>
<record><row><source_ip>203.0.113.5</source_ip><count>40</count><policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>pass</spf></policy_evaluated></row><identifiers><header_from>example.com</header_from></identifiers></record>
<record><row><source_ip>192.0.2.66</source_ip><count>3</count><policy_evaluated><disposition>none</disposition><dkim>fail</dkim><spf>fail</spf></policy_evaluated></row><identifiers><header_from>example.com</header_from></identifiers></record>
</feedback>`

func gz(s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.Bytes()
}

func withAttachment(name, ct string, data []byte) []byte {
	return []byte("From: noreply-dmarc-support@google.com\r\nSubject: Report\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"Z\"\r\n\r\n--Z\r\nContent-Type: text/plain\r\n\r\nreport\r\n" +
		"--Z\r\nContent-Type: " + ct + "; name=\"" + name + "\"\r\nContent-Disposition: attachment; filename=\"" + name + "\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString(data) + "\r\n--Z--\r\n")
}

func TestDMARCReport(t *testing.T) {
	r, err := DMARC(msg.Parse(withAttachment("google.com!example.com!1!2.xml.gz", "application/gzip", gz(dmarcXMLDoc))))
	if err != nil || r.Org != "google.com" || r.ID != "123" || r.Domain != "example.com" || r.Pass != 40 || r.Fail != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := DMARC(msg.Parse([]byte("Subject: x\r\n\r\nnothing"))); err != ErrNoReport {
		t.Fatal(err)
	}
}

func TestTLSReport(t *testing.T) {
	doc := `{"organization-name":"Google Inc.","date-range":{"start-datetime":"2026-10-18T00:00:00Z","end-datetime":"2026-10-18T23:59:59Z"},
	"report-id":"2026-10-18T00:00:00Z_example.com","policies":[{"policy":{"policy-type":"sts","policy-domain":"example.com"},
	"summary":{"total-successful-session-count":12,"total-failure-session-count":1}}]}`
	r, err := TLSRPT(msg.Parse(withAttachment("report.json.gz", "application/tlsrpt+gzip", gz(doc))))
	if err != nil || r.Org != "Google Inc." || r.Domain != "example.com" || r.Pass != 12 || r.Fail != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestWebhooks(t *testing.T) {
	ev, _ := Webhook("application/json", []byte(`{"event":"bounce","rcpt":"x@y.org","bounce":"soft"}`))
	if len(ev) != 1 || ev[0].Hard {
		t.Fatalf("smtp2go soft %+v", ev)
	}
	ev, _ = Webhook("application/json", []byte(`{"event":"spam","email":"x@y.org"}`))
	if len(ev) != 1 || ev[0].Kind != "complaint" {
		t.Fatalf("smtp2go spam %+v", ev)
	}
	ev, _ = Webhook("application/x-www-form-urlencoded", []byte("event=hard_bounce&rcpt=z%40y.org"))
	if len(ev) != 1 || !ev[0].Hard || ev[0].Email != "z@y.org" {
		t.Fatalf("form %+v", ev)
	}
	ev, _ = Webhook("application/json", []byte(`[{"type":"unsubscribe","email":"u@y.org"},{"type":"open","email":"o@y.org"}]`))
	if len(ev) != 1 || ev[0].Kind != "unsubscribe" {
		t.Fatalf("list %+v", ev)
	}
}

// snsSigner signs SNS messages the way Amazon does, with a certificate
// served from a fake sns.<region>.amazonaws.com.
type snsSigner struct {
	key  *rsa.PrivateKey
	cert []byte
}

func newSNSSigner(t *testing.T) *snsSigner {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &snsSigner{key: key, cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (s *snsSigner) sign(m map[string]string, fields ...string) []byte {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f + "\n" + m[f] + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	m["SignatureVersion"] = "2"
	m["Signature"] = base64.StdEncoding.EncodeToString(sig)
	if m["SigningCertURL"] == "" {
		m["SigningCertURL"] = "https://sns.eu-central-1.amazonaws.com/SimpleNotificationService-x.pem"
	}
	out, _ := json.Marshal(m)
	return out
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSNS(t *testing.T) {
	s := newSNSSigner(t)
	fetched := 0
	v := &SNSVerifier{Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		fetched++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(s.cert))}, nil
	})}}
	ctx := context.Background()
	inner := `{"notificationType":"Bounce","mail":{"source":"bounce+tok@example.com"},"bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"Gone@Example.org","diagnosticCode":"550 5.1.1"}]}}`
	note := func() map[string]string {
		return map[string]string{"Type": "Notification", "Message": inner, "MessageId": "1", "Timestamp": "2026-10-20T10:00:00Z", "TopicArn": "arn:aws:sns:eu-central-1:1:lfr"}
	}
	fields := []string{"Message", "MessageId", "Timestamp", "TopicArn", "Type"}
	m, err := v.Verify(ctx, s.sign(note(), fields...))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := SESEvents(m)
	if err != nil || len(ev) != 1 || ev[0].Kind != "bounce" || !ev[0].Hard || ev[0].Email != "gone@example.org" || ev[0].Source != "bounce+tok@example.com" || m.TopicArn != "arn:aws:sns:eu-central-1:1:lfr" {
		t.Fatalf("%+v %v", ev, err)
	}
	// Tampering, foreign certificate hosts and unsigned messages fail.
	tampered := note()
	body := s.sign(tampered, fields...)
	body = bytes.Replace(body, []byte("Gone@"), []byte("Else@"), 1)
	if _, err := v.Verify(ctx, body); err == nil {
		t.Fatal("tampered message verified")
	}
	foreign := note()
	foreign["SigningCertURL"] = "https://sns.evil.s3.amazonaws.com.evil.org/x.pem"
	if _, err := v.Verify(ctx, s.sign(foreign, fields...)); err == nil {
		t.Fatal("certificate from a foreign host accepted")
	}
	bucket := note()
	bucket["SigningCertURL"] = "https://sns.bucket.s3.amazonaws.com/x.pem"
	if _, err := v.Verify(ctx, s.sign(bucket, fields...)); err == nil {
		t.Fatal("certificate from an S3 bucket accepted")
	}
	if _, err := v.Verify(ctx, []byte(`{"Type":"Notification","Message":"{}"}`)); err == nil {
		t.Fatal("unsigned message verified")
	}
	confirm := map[string]string{"Type": "SubscriptionConfirmation", "Message": "m", "MessageId": "2", "SubscribeURL": "https://evil.example/x", "Timestamp": "t", "Token": "k", "TopicArn": "arn"}
	if _, err := v.Verify(ctx, s.sign(confirm, "Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type")); err == nil {
		t.Fatal("non-SNS confirmation URL accepted")
	}
	if fetched != 1 {
		t.Fatalf("certificate fetched %d times, want 1 (cached)", fetched)
	}
}

func FuzzParsers(f *testing.F) {
	f.Add([]byte(dsn))
	f.Add([]byte(arf))
	f.Fuzz(func(t *testing.T, raw []byte) {
		p := msg.Parse(raw)
		DSN(p)
		ARF(p)
		DMARC(p)
		TLSRPT(p)
		Webhook("application/json", raw)
	})
}
