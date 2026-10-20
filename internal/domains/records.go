package domains

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Instance is what the DNS records of every domain point at.
type Instance struct {
	Hostname string // mail host name, e.g. mail.example.com
	IPv4     string
	IPv6     string
	RelaySPF string // e.g. "include:amazonses.com", empty for direct delivery
	STSMode  string // MTA-STS mode: testing or enforce
}

// Record is one DNS record to publish.
type Record struct {
	Type     string `json:"type"`
	Name     string `json:"name"` // fully qualified, without trailing dot
	Value    string `json:"value"`
	Priority int    `json:"priority,omitempty"`
	Purpose  string `json:"purpose"`
	Proxy    bool   `json:"proxy_allowed"` // may sit behind the Cloudflare proxy
}

// Reserved local parts that lfr answers itself on every domain.
const (
	DMARCReports = "dmarc-reports"
	TLSReports   = "tls-reports"
)

// MTASTSPolicy is the policy file served at https://mta-sts.<domain>/.well-known/mta-sts.txt.
func MTASTSPolicy(inst Instance) string {
	mode := inst.STSMode
	if mode == "" {
		mode = "testing"
	}
	return "version: STSv1\nmode: " + mode + "\nmx: " + inst.Hostname + "\nmax_age: 604800\n"
}

// MTASTSID changes whenever the policy does, as RFC 8461 §3.1 requires;
// senders refetch the policy only when the id changes.
func MTASTSID(inst Instance) string {
	sum := sha256.Sum256([]byte(MTASTSPolicy(inst)))
	return hex.EncodeToString(sum[:8])
}

// SPF is the SPF record value of a domain.
func SPF(inst Instance) string {
	parts := []string{"v=spf1", "a:" + inst.Hostname}
	parts = append(parts, strings.Fields(inst.RelaySPF)...)
	return strings.Join(append(parts, "~all"), " ")
}

// Records lists everything d needs in DNS. hosts are the web hosts of the
// domain's lists; they point at this server too.
func Records(d *Domain, inst Instance, hosts []string) []Record {
	var out []Record
	h := inst.Hostname
	if h == d.Name || strings.HasSuffix(h, "."+d.Name) {
		if inst.IPv4 != "" {
			out = append(out, Record{Type: "A", Name: h, Value: inst.IPv4, Purpose: "mail host address; must match the server's reverse DNS (PTR)"})
		}
		if inst.IPv6 != "" {
			out = append(out, Record{Type: "AAAA", Name: h, Value: inst.IPv6, Purpose: "mail host IPv6 address"})
		}
		if h != d.Name {
			out = append(out, Record{Type: "TXT", Name: h, Value: "v=spf1 a -all", Purpose: "SPF for the mail host's own name, checked on its HELO and on bounces it sends (RFC 7208 §2.3)"})
		}
	}
	out = append(out,
		Record{Type: "MX", Name: d.Name, Value: h, Priority: 10, Purpose: "receive mail for " + d.Name},
		Record{Type: "TXT", Name: d.Name, Value: SPF(inst), Purpose: "SPF: who may send for " + d.Name},
		Record{Type: "TXT", Name: d.RSASelector() + "._domainkey." + d.Name, Value: "v=DKIM1; k=rsa; p=" + d.RSAPublic(), Purpose: "DKIM signature key (RSA)"},
		Record{Type: "TXT", Name: d.EdSelector() + "._domainkey." + d.Name, Value: "v=DKIM1; k=ed25519; p=" + d.EdPublic(), Purpose: "DKIM signature key (Ed25519)"},
	)
	if n := d.Next; n != nil {
		out = append(out,
			Record{Type: "TXT", Name: n.RSASelector() + "._domainkey." + d.Name, Value: "v=DKIM1; k=rsa; p=" + n.RSAPublic(), Purpose: "next DKIM key (RSA); lfr domain rotate " + d.Name + " switches to it once published"},
			Record{Type: "TXT", Name: n.EdSelector() + "._domainkey." + d.Name, Value: "v=DKIM1; k=ed25519; p=" + n.EdPublic(), Purpose: "next DKIM key (Ed25519); lfr domain rotate " + d.Name + " switches to it once published"},
		)
	}
	out = append(out,
		Record{Type: "TXT", Name: "_dmarc." + d.Name, Value: "v=DMARC1; p=none; rua=mailto:" + DMARCReports + "@" + d.Name, Purpose: "DMARC policy; move to p=quarantine after a few clean weeks of reports (lfr reports)"},
		Record{Type: "TXT", Name: "_mta-sts." + d.Name, Value: "v=STSv1; id=" + MTASTSID(inst), Purpose: "MTA-STS: senders must use TLS to reach this domain"},
		Record{Type: "CNAME", Name: "mta-sts." + d.Name, Value: h, Purpose: "serves the MTA-STS policy over HTTPS"},
		Record{Type: "TXT", Name: "_smtp._tls." + d.Name, Value: "v=TLSRPTv1; rua=mailto:" + TLSReports + "@" + d.Name, Purpose: "TLS reports from other mail servers"},
	)
	sort.Strings(hosts)
	for _, host := range hosts {
		if inst.IPv4 != "" {
			out = append(out, Record{Type: "A", Name: host, Value: inst.IPv4, Purpose: "website of a list", Proxy: true})
		}
		if inst.IPv6 != "" {
			out = append(out, Record{Type: "AAAA", Name: host, Value: inst.IPv6, Purpose: "website of a list", Proxy: true})
		}
	}
	return out
}

// Zone renders records in BIND zone file syntax, which Cloudflare and most
// DNS hosts can import. Long TXT values are split into 255-byte strings.
func Zone(records []Record) string {
	var b strings.Builder
	for _, r := range records {
		value := r.Value
		switch r.Type {
		case "TXT":
			value = quoteTXT(r.Value)
		case "MX":
			value = fmt.Sprintf("%d %s.", r.Priority, r.Value)
		case "CNAME":
			value = r.Value + "."
		}
		fmt.Fprintf(&b, "%s.\t300\tIN\t%s\t%s\n", r.Name, r.Type, value)
	}
	return b.String()
}

func quoteTXT(s string) string {
	var parts []string
	for len(s) > 255 {
		parts = append(parts, s[:255])
		s = s[255:]
	}
	parts = append(parts, s)
	for i, p := range parts {
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return strings.Join(parts, " ")
}
