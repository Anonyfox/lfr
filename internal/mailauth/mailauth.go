// Package mailauth authenticates arriving mail (SPF, DKIM, DMARC with
// alignment) and DKIM-signs leaving mail. It only computes verdicts;
// what happens to a message is the spam filter's and the SMTP server's call.
package mailauth

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"math/rand/v2"
	"net"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"golang.org/x/net/publicsuffix"

	"github.com/Anonyfox/lfr/internal/dnsx"
)

// DKIMResult is one verified signature.
type DKIMResult struct {
	Domain string `json:"domain"`
	Result string `json:"result"` // pass, fail, temperror, permerror
	Error  string `json:"error,omitempty"`
}

// Result is the authentication verdict of one message.
type Result struct {
	SPF         string       `json:"spf"`        // pass, fail, softfail, neutral, none, temperror, permerror
	SPFDomain   string       `json:"spf_domain"` // MAIL FROM domain, or HELO when empty
	NullSender  bool         `json:"null_sender,omitempty"`
	DKIM        []DKIMResult `json:"dkim,omitempty"`
	FromDomain  string       `json:"from_domain"`
	DMARC       string       `json:"dmarc"`        // pass, fail, none, temperror, permerror
	DMARCPolicy string       `json:"dmarc_policy"` // none, quarantine, reject; empty without a record
}

// DKIMPass reports whether any signature of domain (or, when relaxed, its
// organizational domain) passed.
func (r Result) DKIMPass(domain string) bool {
	for _, d := range r.DKIM {
		if d.Result == "pass" && Aligned(d.Domain, domain, false) {
			return true
		}
	}
	return false
}

// Verifier checks arriving messages.
type Verifier struct {
	DNS dnsx.Resolver
}

// Verify checks raw as received from ip, which greeted with helo and gave
// mailFrom as envelope sender. fromDomain is the domain of the From header.
func (v Verifier) Verify(ctx context.Context, raw []byte, ip net.IP, helo, mailFrom, fromDomain string) Result {
	r := Result{FromDomain: strings.ToLower(fromDomain)}

	r.SPFDomain = domainOf(mailFrom)
	if r.SPFDomain == "" {
		r.SPFDomain, r.NullSender = strings.ToLower(helo), true
	}
	res, _ := spf.CheckHostWithSender(ip, helo, mailFrom, spf.WithResolver(v.DNS), spf.WithContext(ctx))
	r.SPF = string(res)

	lookup := func(domain string) ([]string, error) { return v.DNS.LookupTXT(ctx, domain) }
	verifications, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{LookupTXT: lookup, MaxVerifications: 5})
	if err == nil {
		for _, ver := range verifications {
			d := DKIMResult{Domain: strings.ToLower(ver.Domain), Result: "pass"}
			if ver.Err != nil {
				d.Error = ver.Err.Error()
				switch {
				case dkim.IsTempFail(ver.Err):
					d.Result = "temperror"
				case dkim.IsPermFail(ver.Err):
					d.Result = "permerror"
				default:
					d.Result = "fail"
				}
			}
			r.DKIM = append(r.DKIM, d)
		}
	}

	r.DMARC, r.DMARCPolicy = v.dmarc(ctx, r, lookup)
	return r
}

func (v Verifier) dmarc(_ context.Context, r Result, lookup func(string) ([]string, error)) (string, string) {
	if r.FromDomain == "" {
		return "none", ""
	}
	org := OrgDomain(r.FromDomain)
	rec, err := dmarc.LookupWithOptions(r.FromDomain, &dmarc.LookupOptions{LookupTXT: lookup})
	sub := false
	if err == dmarc.ErrNoPolicy && org != r.FromDomain {
		rec, err = dmarc.LookupWithOptions(org, &dmarc.LookupOptions{LookupTXT: lookup})
		sub = true
	}
	switch {
	case err == dmarc.ErrNoPolicy:
		return "none", ""
	case err != nil && dmarc.IsTempFail(err):
		return "temperror", ""
	case err != nil:
		return "permerror", ""
	}
	policy := rec.Policy
	if sub && rec.SubdomainPolicy != "" {
		policy = rec.SubdomainPolicy
	}
	strictDKIM := rec.DKIMAlignment == dmarc.AlignmentStrict
	strictSPF := rec.SPFAlignment == dmarc.AlignmentStrict
	pass := r.SPF == "pass" && Aligned(r.SPFDomain, r.FromDomain, strictSPF)
	for _, d := range r.DKIM {
		if d.Result == "pass" && Aligned(d.Domain, r.FromDomain, strictDKIM) {
			pass = true
		}
	}
	if pass {
		return "pass", string(policy)
	}
	// A temporary error is not a failure: the sender retries, and a resolver
	// hiccup must not turn into a permanent rejection (RFC 7489 §6.6.2).
	if r.SPF == "temperror" {
		return "temperror", string(policy)
	}
	for _, d := range r.DKIM {
		if d.Result == "temperror" {
			return "temperror", string(policy)
		}
	}
	// pct below 100 applies the policy to that share of failing mail and
	// the next milder policy to the rest (RFC 7489 §6.6.4).
	if rec.Percent != nil && *rec.Percent < 100 && rand.IntN(100) >= *rec.Percent {
		switch policy {
		case dmarc.PolicyReject:
			policy = dmarc.PolicyQuarantine
		case dmarc.PolicyQuarantine:
			policy = dmarc.PolicyNone
		}
	}
	return "fail", string(policy)
}

// Aligned reports DMARC identifier alignment of a with b.
func Aligned(a, b string, strict bool) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return !strict && OrgDomain(a) == OrgDomain(b)
}

// OrgDomain is the organizational domain (public suffix plus one label).
func OrgDomain(d string) string {
	o, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(d))
	if err != nil {
		return strings.ToLower(d)
	}
	return o
}

func domainOf(email string) string {
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		return strings.ToLower(email[i+1:])
	}
	return ""
}

// Header renders the verdict as an Authentication-Results field value.
func (r Result) Header(host string) string {
	identity := "smtp.mailfrom"
	if r.NullSender {
		identity = "smtp.helo"
	}
	parts := []string{host, fmt.Sprintf("spf=%s %s=%s", r.SPF, identity, r.SPFDomain)}
	if len(r.DKIM) == 0 {
		parts = append(parts, "dkim=none")
	}
	for _, d := range r.DKIM {
		parts = append(parts, fmt.Sprintf("dkim=%s header.d=%s", d.Result, d.Domain))
	}
	parts = append(parts, fmt.Sprintf("dmarc=%s header.from=%s", r.DMARC, r.FromDomain))
	return strings.Join(parts, "; ")
}

// SignedHeaders are the fields DKIM signatures cover. Fields that change what
// a reader sees or does are listed twice: a field listed once covers only its
// last instance, so a replayed message could gain a second Subject or
// List-Unsubscribe above it; the extra entry signs "no further instance"
// (oversigning). Listing an absent field still locks it.
var SignedHeaders = signed(false)

// RelayHeaders leave out the fields relays rewrite: Amazon SES replaces
// Message-ID and Date and asks senders not to sign them, and SES and SMTP2GO
// add their own Feedback-ID. A signature covering them would fail on arrival.
var RelayHeaders = signed(true)

func signed(relay bool) []string {
	var out []string
	twice := []string{"From", "Reply-To", "Sender", "To", "Cc", "Subject", "Date", "Message-ID", "In-Reply-To", "References",
		"MIME-Version", "Content-Type", "Content-Transfer-Encoding", "List-Id", "List-Unsubscribe", "List-Unsubscribe-Post"}
	for _, h := range twice {
		if relay && (h == "Date" || h == "Message-ID") {
			continue
		}
		out = append(out, h, h)
	}
	out = append(out, "Form-Sub")
	if !relay {
		out = append(out, "Feedback-ID")
	}
	return out
}

// Key is one signing key with its selector.
type Key struct {
	Selector string
	Signer   crypto.Signer
}

// SignHeaders signs like Sign, covering the given header fields.
func SignHeaders(raw []byte, domain string, keys []Key, now time.Time, headers []string) ([]byte, error) {
	out := raw
	for _, k := range keys {
		var buf bytes.Buffer
		buf.Grow(len(out) + 1024)
		err := dkim.Sign(&buf, bytes.NewReader(out), &dkim.SignOptions{
			Domain:                 domain,
			Selector:               k.Selector,
			Signer:                 k.Signer,
			Hash:                   crypto.SHA256,
			HeaderCanonicalization: dkim.CanonicalizationRelaxed,
			BodyCanonicalization:   dkim.CanonicalizationRelaxed,
			HeaderKeys:             headers,
			Expiration:             now.Add(30 * 24 * time.Hour),
		})
		if err != nil {
			return nil, fmt.Errorf("dkim sign %s: %w", domain, err)
		}
		out = buf.Bytes()
	}
	return out, nil
}
