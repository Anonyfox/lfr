package feedback

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SNS is an Amazon SNS message whose signature checked out.
type SNS struct {
	Type         string
	TopicArn     string
	SubscribeURL string
	Message      string
}

// snsHost is where Amazon SNS serves signing certificates and confirmation
// endpoints; nothing else is ever fetched.
var snsHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

func snsURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && p.Scheme == "https" && p.User == nil && snsHost.MatchString(p.Hostname()) && p.Port() == ""
}

// SNSVerifier checks SNS message signatures against Amazon's signing
// certificates, as AWS documents for HTTPS subscribers.
type SNSVerifier struct {
	Client *http.Client

	mu    sync.Mutex
	certs map[string]*x509.Certificate
}

// Verify parses body as an SNS message and checks its signature.
func (v *SNSVerifier) Verify(ctx context.Context, body []byte) (*SNS, error) {
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, errors.New("not an SNS message")
	}
	var fields []string
	switch m["Type"] {
	case "Notification":
		fields = []string{"Message", "MessageId", "Subject", "Timestamp", "TopicArn", "Type"}
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		fields = []string{"Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type"}
	default:
		return nil, fmt.Errorf("SNS message type %q", m["Type"])
	}
	var b strings.Builder
	for _, f := range fields {
		val, ok := m[f]
		if !ok && f == "Subject" {
			continue // the only optional field
		}
		b.WriteString(f + "\n" + val + "\n")
	}
	var hash crypto.Hash
	var sum []byte
	switch m["SignatureVersion"] {
	case "1":
		s := sha1.Sum([]byte(b.String()))
		hash, sum = crypto.SHA1, s[:]
	case "2":
		s := sha256.Sum256([]byte(b.String()))
		hash, sum = crypto.SHA256, s[:]
	default:
		return nil, fmt.Errorf("SNS signature version %q", m["SignatureVersion"])
	}
	sig, err := base64.StdEncoding.DecodeString(m["Signature"])
	if err != nil {
		return nil, errors.New("SNS signature is not base64")
	}
	cert, err := v.cert(ctx, m["SigningCertURL"])
	if err != nil {
		return nil, err
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("SNS certificate holds no RSA key")
	}
	if err := rsa.VerifyPKCS1v15(pub, hash, sum, sig); err != nil {
		return nil, errors.New("SNS signature does not verify")
	}
	if m["SubscribeURL"] != "" && !snsURL(m["SubscribeURL"]) {
		return nil, errors.New("subscription URL is not an Amazon SNS endpoint")
	}
	return &SNS{Type: m["Type"], TopicArn: m["TopicArn"], SubscribeURL: m["SubscribeURL"], Message: m["Message"]}, nil
}

func (v *SNSVerifier) cert(ctx context.Context, certURL string) (*x509.Certificate, error) {
	if !snsURL(certURL) || !strings.HasSuffix(strings.ToLower(certURL), ".pem") {
		return nil, errors.New("SNS signing certificate is not on an Amazon SNS host")
	}
	v.mu.Lock()
	c, ok := v.certs[certURL]
	v.mu.Unlock()
	if ok && time.Now().Before(c.NotAfter) {
		return c, nil
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SNS certificate: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SNS certificate: HTTP %d", resp.StatusCode)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("SNS certificate is not PEM")
	}
	c, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("SNS certificate: %w", err)
	}
	if now := time.Now(); now.Before(c.NotBefore) || now.After(c.NotAfter) {
		return nil, errors.New("SNS certificate is not valid now")
	}
	v.mu.Lock()
	if v.certs == nil {
		v.certs = map[string]*x509.Certificate{}
	}
	v.certs[certURL] = c
	v.mu.Unlock()
	return c, nil
}

// SESEvents reads the SES notification inside a verified SNS message.
func SESEvents(s *SNS) ([]Event, error) {
	var inner map[string]any
	if err := json.Unmarshal([]byte(s.Message), &inner); err != nil {
		return nil, err
	}
	return ses(inner), nil
}
