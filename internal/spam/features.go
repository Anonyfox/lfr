// Package spam scores arriving mail. Rule features (authentication, DNS
// blocklists, header and link anomalies), a Bayesian token classifier and
// sender reputation are combined by logistic weights. Every time the operator
// moves a message between inbox and junk, the Bayes tokens, the reputation
// and the weights learn from it.
package spam

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
)

// Input is everything known about an arriving message.
type Input struct {
	IP           net.IP
	Helo         string
	PTR          string // first reverse DNS name, "" if none
	PTRConfirmed bool   // the PTR name resolves back to IP
	DNSBL        []string
	Auth         mailauth.Result
	Parsed       *msg.Parsed
	KnownContact bool // the sender is someone lfr sent mail to before
	OwnDomain    bool // the From domain is one lfr serves, so lfr signed every genuine mail from it
	Received     time.Time
}

var hrefRE = regexp.MustCompile(`(?is)<a\s[^>]*href\s*=\s*["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)

var shorteners = map[string]bool{
	"bit.ly": true, "tinyurl.com": true, "t.co": true, "goo.gl": true, "ow.ly": true, "is.gd": true,
	"buff.ly": true, "rebrand.ly": true, "cutt.ly": true, "shorturl.at": true, "rb.gy": true, "t.ly": true,
}

// scamPhrases are the stock lines of prize, crypto, pharma and payment
// scams, in English and German. One alone proves little; three are typical.
var scamPhrases = regexp.MustCompile(`you(?:'ve| have)? won|claim (?:your|it|the|now)|act now|limited time|100% free|risk[- ]free|winner|lottery|inheritance|wire transfer|bitcoin|crypto(?:currency)? (?:investment|profit)|casino|viagra|cialis|dear friend|urgent(?:ly)? (?:reply|response|attention)|verify your account|account (?:will be )?suspended|sie haben gewonnen|gewinn abholen|jetzt handeln|dringend|konto (?:wurde )?gesperrt`)

// subjectBait is a subject with a money amount or repeated exclamation marks.
var subjectBait = regexp.MustCompile(`[$€£]\s?\d[\d,.]*|\d[\d,.]*\s?(?:[$€£]|dollars?|euro)|!!`)

var riskyExt = []string{".zip", ".rar", ".7z", ".html", ".htm", ".shtml", ".iso", ".img", ".svg", ".one", ".lnk", ".xlsm", ".docm"}

// Executable reports attachments no legitimate mail carries; lfr refuses them outright.
func Executable(p *msg.Parsed) string {
	for _, a := range p.Attachments {
		// Windows ignores trailing dots and spaces: "invoice.exe. " runs.
		n := strings.TrimRight(strings.ToLower(a.Name), ". \t")
		for _, ext := range []string{".exe", ".scr", ".com", ".bat", ".cmd", ".pif", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".msi", ".hta", ".cpl", ".jar", ".ps1", ".reg"} {
			if strings.HasSuffix(n, ext) {
				return a.Name
			}
		}
	}
	return ""
}

// Features turns an input into named values, mostly 0 or 1.
func Features(in Input) map[string]float64 {
	f := map[string]float64{}
	set := func(name string, on bool) {
		if on {
			f[name] = 1
		}
	}
	a := in.Auth
	set("spf_pass", a.SPF == "pass")
	set("spf_fail", a.SPF == "fail")
	set("spf_softfail", a.SPF == "softfail")
	set("spf_none", a.SPF == "none" || a.SPF == "neutral" || a.SPF == "permerror")
	aligned, failed := false, false
	for _, d := range a.DKIM {
		if d.Result == "pass" && mailauth.Aligned(d.Domain, a.FromDomain, false) {
			aligned = true
		}
		if d.Result == "fail" || d.Result == "permerror" {
			failed = true
		}
	}
	set("dkim_aligned", aligned)
	set("dkim_fail", failed && !aligned)
	set("dkim_none", len(a.DKIM) == 0)
	set("dmarc_pass", a.DMARC == "pass")
	set("dmarc_fail", a.DMARC == "fail")
	set("dmarc_none", a.DMARC == "none")
	if n := len(in.DNSBL); n > 0 {
		f["dnsbl"] = min(1, 0.5+0.25*float64(n-1))
	}
	set("no_ptr", in.PTR == "")
	set("ptr_unconfirmed", in.PTR != "" && !in.PTRConfirmed)
	h := strings.Trim(in.Helo, "[]")
	set("helo_bad", h == "" || net.ParseIP(h) != nil || !strings.Contains(h, "."))
	set("known_contact", in.KnownContact)
	// Mail in the name of a domain lfr serves always carries lfr's own
	// signature; without one it is forged (the "boss asks to pay" fraud).
	set("own_domain_forged", in.OwnDomain && !in.Auth.DKIMPass(in.Auth.FromDomain))

	p := in.Parsed
	if p == nil {
		return f
	}
	set("no_message_id", p.MessageID == "")
	set("no_date", p.Date.IsZero())
	if !p.Date.IsZero() && !in.Received.IsZero() {
		d := in.Received.Sub(p.Date)
		set("date_skew", d > 72*time.Hour || d < -24*time.Hour)
	}
	set("subject_empty", strings.TrimSpace(p.Subject) == "")
	set("subject_caps", shouting(p.Subject))
	set("subject_bait", subjectBait.MatchString(p.Subject))
	// Until Bayes has seen enough mail, a few phrases that almost only
	// scams use carry the content signal.
	seen := map[string]bool{}
	for _, m := range scamPhrases.FindAllString(strings.ToLower(p.Subject+"\n"+p.Text), 50) {
		seen[strings.Join(strings.Fields(m), " ")] = true
	}
	if len(seen) > 0 {
		f["scam_phrases"] = min(1, float64(len(seen))/3)
	}
	plain := false
	for _, part := range p.Parts {
		if part.Type == "text/plain" && part.Disposition != "attachment" {
			plain = true
		}
	}
	set("html_only", p.HTML != "" && !plain)
	links, mismatch, short := 0, false, false
	for _, m := range hrefRE.FindAllStringSubmatch(p.HTML, 500) {
		links++
		href := hostOf(m[1])
		if shorteners[href] {
			short = true
		}
		text := strings.TrimSpace(stripTags(m[2]))
		if shown := hostOf(text); shown != "" && strings.Contains(text, ".") && href != "" && mailauth.OrgDomain(shown) != mailauth.OrgDomain(href) {
			mismatch = true
		}
	}
	for _, w := range strings.Fields(p.Text) {
		if host := hostOf(w); shorteners[host] {
			short = true
		}
	}
	set("many_links", links > 25)
	set("link_mismatch", mismatch)
	set("shortener", short)
	name := strings.ToLower(p.From.Name)
	if strings.Contains(name, "@") {
		if at := strings.LastIndexByte(name, '@'); at >= 0 {
			if words := strings.Fields(name[at+1:]); len(words) > 0 {
				shownDomain := strings.Trim(words[0], "<>\"'()")
				set("from_spoof", shownDomain != "" && mailauth.OrgDomain(shownDomain) != mailauth.OrgDomain(a.FromDomain))
			}
		}
	}
	risky := false
	for _, att := range p.Attachments {
		n := strings.ToLower(att.Name)
		for _, ext := range riskyExt {
			if strings.HasSuffix(n, ext) {
				risky = true
			}
		}
	}
	set("risky_attachment", risky)
	return f
}

func shouting(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	return letters >= 8 && upper*10 >= letters*7
}

func hostOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		if strings.HasPrefix(s, "www.") {
			s = "http://" + s
		} else {
			return ""
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

func stripTags(s string) string { return tagRE.ReplaceAllString(s, "") }
