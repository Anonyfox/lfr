package engine

import (
	"context"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/dnsx"
	"github.com/Anonyfox/lfr/internal/domains"
)

// checkRecords compares expected records with what DNS answers.
func (e *Engine) checkRecords(ctx context.Context, recs []domains.Record) []api.RecordCheck {
	out := make([]api.RecordCheck, 0, len(recs))
	for _, r := range recs {
		c := api.RecordCheck{Record: r, Status: "missing"}
		switch r.Type {
		case "A", "AAAA":
			addrs, err := e.DNS.LookupIPAddr(ctx, r.Name)
			if err == nil {
				for _, a := range addrs {
					c.Found = append(c.Found, a.IP.String())
				}
				c.Status = "wrong"
				if containsIP(addrs, r.Value) || r.Proxy && len(addrs) > 0 {
					c.Status = "ok" // proxied hosts answer with the proxy's addresses
				}
			}
		case "MX":
			mxs, err := e.DNS.LookupMX(ctx, r.Name)
			if err == nil {
				c.Status = "wrong"
				for _, mx := range mxs {
					h := strings.TrimSuffix(strings.ToLower(mx.Host), ".")
					c.Found = append(c.Found, h)
					if h == r.Value {
						c.Status = "ok"
					}
				}
			}
		case "CNAME":
			cname, err := e.DNS.LookupCNAME(ctx, r.Name)
			if err == nil {
				got := strings.TrimSuffix(strings.ToLower(cname), ".")
				c.Found = []string{got}
				c.Status = "wrong"
				if got == r.Value {
					c.Status = "ok"
				} else if addrs, err := e.DNS.LookupIPAddr(ctx, r.Name); err == nil {
					// An A record pointing at the server works as well as the CNAME.
					if want, err := e.DNS.LookupIPAddr(ctx, r.Value); err == nil && len(want) > 0 && containsIP(addrs, want[0].IP.String()) {
						c.Status = "ok"
					}
				}
			}
		case "TXT":
			txts, err := e.DNS.LookupTXT(ctx, r.Name)
			if err == nil {
				c.Found = txts
				c.Status = e.txtStatus(ctx, r, txts)
			}
		}
		out = append(out, c)
	}
	return out
}

// txtStatus decides whether published TXT records satisfy an expected one.
// Records are compared as tags and terms, never as substrings: policies may be
// stricter than lfr's suggestion, keys and ids must match exactly, and two
// records of one kind cancel each other (RFC 7208 §4.5, RFC 7489 §6.6.3,
// RFC 8461 §3.1).
func (e *Engine) txtStatus(ctx context.Context, r domains.Record, txts []string) string {
	kind := strings.TrimSpace(strings.SplitN(r.Value, ";", 2)[0])
	if strings.HasPrefix(r.Value, "v=spf1") {
		kind = "v=spf1"
	}
	var found []string
	for _, t := range txts {
		t = strings.TrimSpace(t)
		if t == kind || strings.HasPrefix(t, kind+";") || strings.HasPrefix(t, kind+" ") {
			found = append(found, t)
		}
	}
	switch {
	case len(found) == 0:
		return "missing"
	case len(found) > 1:
		return "wrong"
	}
	got := found[0]
	switch kind {
	case "v=spf1":
		terms := map[string]bool{}
		for _, t := range strings.Fields(got)[1:] {
			terms[strings.TrimPrefix(strings.ToLower(t), "+")] = true
		}
		for _, t := range strings.Fields(r.Value)[1:] {
			if t == "~all" || t == "-all" {
				continue
			}
			if !terms[strings.ToLower(t)] {
				return "wrong"
			}
		}
		if terms["all"] || terms["?all"] || !terms["~all"] && !terms["-all"] && !hasRedirect(terms) {
			return "wrong" // authorizes everyone, or ends without a verdict
		}
		if spfLookups(ctx, e.DNS, got, 0) > 10 {
			return "wrong" // receivers answer permerror past ten lookups
		}
		return "ok"
	case "v=DKIM1":
		want, have := tags(r.Value), tags(got)
		if have["p"] != want["p"] || strings.ToLower(defaultTo(have["k"], "rsa")) != want["k"] {
			return "wrong" // a missing k= means RSA, so an Ed25519 key without it never verifies
		}
		return "ok"
	case "v=DMARC1":
		switch strings.ToLower(tags(got)["p"]) {
		case "none", "quarantine", "reject":
			return "ok"
		}
		return "wrong"
	case "v=STSv1":
		if tags(got)["id"] != tags(r.Value)["id"] {
			return "wrong" // senders keep the old policy until the id changes
		}
		return "ok"
	}
	return "ok"
}

func hasRedirect(terms map[string]bool) bool {
	for t := range terms {
		if strings.HasPrefix(t, "redirect=") {
			return true
		}
	}
	return false
}

// tags parses a tag=value; list (DKIM, DMARC, MTA-STS), ignoring whitespace.
func tags(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.Join(strings.Fields(v), "")
	}
	return out
}

func defaultTo(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// spfLookups counts the DNS lookups an SPF record costs a receiver,
// following include and redirect (RFC 7208 §4.6.4).
func spfLookups(ctx context.Context, r dnsx.Resolver, record string, depth int) int {
	if depth > 10 {
		return 11
	}
	n := 0
	for _, t := range strings.Fields(record)[1:] {
		t = strings.ToLower(strings.TrimLeft(t, "+-~?"))
		name, target, _ := strings.Cut(t, ":")
		if strings.HasPrefix(t, "redirect=") {
			name, target = "redirect", strings.TrimPrefix(t, "redirect=")
		}
		name, _, _ = strings.Cut(name, "/")
		switch name {
		case "a", "mx", "ptr", "exists":
			n++
		case "include", "redirect":
			n++
			txts, err := r.LookupTXT(ctx, target)
			if err != nil {
				continue
			}
			for _, txt := range txts {
				if strings.HasPrefix(txt, "v=spf1") {
					n += spfLookups(ctx, r, txt, depth+1)
				}
			}
		}
	}
	return n
}
