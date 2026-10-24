package engine

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/dnsbl"
)

// Status is the overview of the running server.
func (e *Engine) Status(ctx context.Context) (api.Status, error) {
	s := api.Status{Version: e.Opts.Version, Hostname: e.Hostname(ctx), Uptime: time.Since(e.started).Round(time.Second).String()}
	s.IPv4, _ = e.Config.Get(ctx, "ipv4")
	info := e.relayInfo(ctx)
	s.Transport = info.Mode
	if info.URL != "" {
		s.Transport += " " + info.URL
	}
	e.DB.R.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM domains), (SELECT count(*) FROM mailboxes),
		(SELECT count(*) FROM messages WHERE folder = 'inbox' AND seen = 0), (SELECT count(*) FROM lists),
		(SELECT count(*) FROM members WHERE status = 'confirmed')`).Scan(&s.Domains, &s.Mailboxes, &s.Unread, &s.Lists, &s.Members)
	q, err := e.Queue.Stats(ctx)
	if err != nil {
		return s, err
	}
	s.Queue = q
	e.mu.Lock()
	s.Listening = append([]string{}, e.listening...)
	e.mu.Unlock()
	switch {
	case s.IPv4 == "":
		s.Next = append(s.Next, "lfr config ipv4 <the server's public IPv4>")
	case s.Domains == 0:
		s.Next = append(s.Next, "lfr domain add example.com, then publish the records it prints")
	case s.Mailboxes == 0:
		s.Next = append(s.Next, "lfr mailbox add you@<domain>, then lfr doctor")
	}
	if rows, err := e.DB.R.QueryContext(ctx, "SELECT slug, paused FROM lists WHERE paused != '' ORDER BY slug"); err == nil {
		for rows.Next() {
			var slug, why string
			rows.Scan(&slug, &why)
			s.Paused = append(s.Paused, slug+": "+why)
			s.Next = append(s.Next, "list "+slug+" is paused and sends nothing: "+why)
		}
		rows.Close()
	}
	if missed, err := e.Issues.Missed(ctx); err == nil {
		for _, w := range missed {
			s.Next = append(s.Next, fmt.Sprintf("issue %s/%s missed its slot (%s) while the server was down and waits: lfr issue send %s/%s sends it now, --at picks a new time, lfr issue cancel drops it",
				w.List, w.Slug, w.Scheduled.Local().Format("Mon 2006-01-02 15:04"), w.List, w.Slug))
		}
	}
	if q.Down != "" {
		s.Next = append([]string{"outgoing mail is stuck since " + q.DownSince + ": " + q.Down + "; it waits and goes out once lfr relay passes"}, s.Next...)
	}
	if q.Failed24 > 0 || q.Retrying > 0 {
		s.Next = append(s.Next, "lfr queue shows failing deliveries")
	}
	if len(s.Next) == 0 {
		s.Next = append(s.Next, "lfr doctor checks DNS, certificates and deliverability")
	}
	return s, nil
}

// Doctor runs every check an operator needs before trusting the setup.
func (e *Engine) Doctor(ctx context.Context) []api.Check {
	var out []api.Check
	add := func(c api.Check) { out = append(out, c) }
	host := e.Hostname(ctx)
	ip4, _ := e.Config.Get(ctx, "ipv4")
	relayURL, _ := e.Config.Get(ctx, "relay.url")
	direct := relayURL == ""

	if ip4 == "" {
		add(api.Check{Name: "ipv4", Status: "fail", Detail: "public IPv4 not configured", Fix: "lfr config ipv4 203.0.113.7"})
	}
	addrs, err := e.DNS.LookupIPAddr(ctx, host)
	switch {
	case err != nil:
		add(api.Check{Name: "hostname", Status: "fail", Detail: host + " does not resolve: " + err.Error(), Fix: "add an A record " + host + " → " + ip4 + " (DNS only)"})
	case ip4 != "" && !containsIP(addrs, ip4):
		add(api.Check{Name: "hostname", Status: "fail", Detail: fmt.Sprintf("%s resolves to %v, not %s", host, ipList(addrs), ip4), Fix: "point the A record of " + host + " at " + ip4})
	default:
		add(api.Check{Name: "hostname", Status: "ok", Detail: host + " → " + ipList(addrs)})
	}
	if ip4 != "" {
		names, err := e.DNS.LookupAddr(ctx, ip4)
		ok := false
		for _, n := range names {
			if strings.EqualFold(strings.TrimSuffix(n, "."), host) {
				ok = true
			}
		}
		sev := "warn"
		if direct {
			sev = "fail"
		}
		switch {
		case ok:
			add(api.Check{Name: "reverse dns", Status: "ok", Detail: ip4 + " → " + host})
		case err != nil || len(names) == 0:
			add(api.Check{Name: "reverse dns", Status: sev, Detail: ip4 + " has no PTR record", Fix: "name the server " + host + " at your hosting provider (DigitalOcean: rename the droplet)"})
		default:
			add(api.Check{Name: "reverse dns", Status: sev, Detail: fmt.Sprintf("%s → %s, not %s", ip4, strings.Join(names, ", "), host), Fix: "name the server " + host + " at your hosting provider"})
		}
		lists := []string{"zen.spamhaus.org", "bl.spamcop.net", "b.barracudacentral.org", "psbl.surriel.com"}
		hits := (&dnsbl.Checker{DNS: e.DNS}).Check(ctx, net.ParseIP(ip4), lists)
		if len(hits) > 0 {
			sev := "warn"
			fix := "request delisting at each list; with a relay this only affects mail you receive"
			if direct {
				sev, fix = "fail", "request delisting, or rebuild the server to get a fresh IP before sending"
			}
			add(api.Check{Name: "blocklists", Status: sev, Detail: ip4 + " is listed on " + strings.Join(hits, ", "), Fix: fix})
		} else {
			add(api.Check{Name: "blocklists", Status: "ok", Detail: ip4 + " is on none of " + strings.Join(lists, ", ")})
		}
	}
	add(e.testTransport(ctx))
	if !direct {
		info := e.relayInfo(ctx)
		if info.SPF == "" {
			add(api.Check{Name: "relay spf", Status: "skip", Detail: "the relay's SPF mechanism is not set; DMARC passes through lfr's DKIM signature, and SPF only counts when the relay sends with your domain in the envelope", Fix: "lfr relay <url> --spf include:amazonses.com (or your relay's) when you set up a custom MAIL FROM domain"})
		}
		add(e.relayWebhookCheck(ctx, info.Webhooks["generic"]))
	}

	ds, _ := e.Domains.List(ctx)
	if len(ds) == 0 {
		add(api.Check{Name: "domains", Status: "warn", Detail: "no domains yet", Fix: "lfr domain add example.com"})
	}
	hostsChecked := map[string]bool{host: true}
	for _, d := range ds {
		info, err := e.domainInfo(ctx, d, true)
		if err != nil {
			add(api.Check{Name: "dns " + d.Name, Status: "fail", Detail: err.Error()})
			continue
		}
		var missing []string
		for _, c := range info.Checks {
			if c.Status != "ok" {
				missing = append(missing, c.Record.Type+" "+c.Record.Name+" ("+c.Status+")")
			}
		}
		if len(missing) == 0 {
			add(api.Check{Name: "dns " + d.Name, Status: "ok", Detail: fmt.Sprintf("all %d records published", len(info.Checks))})
		} else {
			add(api.Check{Name: "dns " + d.Name, Status: "fail", Detail: strings.Join(missing, "; "), Fix: "lfr domain dns " + d.Name + " --check shows each record to publish"})
		}
		hostsChecked["mta-sts."+d.Name] = true
		hs, _ := e.Lists.Hosts(ctx, d.ID)
		for _, h := range hs {
			hostsChecked[h] = true
		}
	}
	var hosts []string
	for h := range hostsChecked {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		st := e.Certs.Inspect(h)
		switch {
		case st.Valid && time.Until(st.Expires) > 14*24*time.Hour:
			add(api.Check{Name: "tls " + h, Status: "ok", Detail: fmt.Sprintf("certificate from %s until %s", st.Issuer, st.Expires.Format("2006-01-02"))})
		case st.Valid:
			add(api.Check{Name: "tls " + h, Status: "warn", Detail: "certificate expires " + st.Expires.Format("2006-01-02") + " and was not renewed yet", Fix: "check that " + h + " still points here and port 80/443 are open"})
		default:
			add(api.Check{Name: "tls " + h, Status: "warn", Detail: "no certificate yet; one is requested on the first HTTPS or STARTTLS connection", Fix: "open https://" + h + "/healthz once its DNS points here"})
		}
	}

	// What other mail servers saw in the last two weeks.
	var dmarcFail, tlsFail int
	e.DB.R.QueryRowContext(ctx, `SELECT coalesce(sum(fail) FILTER (WHERE kind = 'dmarc'), 0), coalesce(sum(fail) FILTER (WHERE kind = 'tlsrpt'), 0)
		FROM reports WHERE end_at > ?`, e.Opts.Now().AddDate(0, 0, -14).Unix()).Scan(&dmarcFail, &tlsFail)
	switch {
	case tlsFail > 0:
		add(api.Check{Name: "reports", Status: "warn", Detail: fmt.Sprintf("%d TLS sessions to your mail host failed in the last 14 days (TLS reports)", tlsFail), Fix: "lfr reports shows why"})
	case dmarcFail > 0:
		add(api.Check{Name: "reports", Status: "warn", Detail: fmt.Sprintf("%d messages as your domains failed DMARC in the last 14 days", dmarcFail), Fix: "lfr reports shows from where: your own services need DKIM, the rest is spoofing"})
	}

	var fs syscall.Statfs_t
	if err := syscall.Statfs(e.Opts.Data, &fs); err == nil {
		free := int64(fs.Bavail) * int64(fs.Bsize)
		switch {
		case free < 512<<20:
			add(api.Check{Name: "disk", Status: "fail", Detail: fmt.Sprintf("%d MB free in %s", free>>20, e.Opts.Data), Fix: "free space or grow the disk; mail is refused when it runs out"})
		case free < 2<<30:
			add(api.Check{Name: "disk", Status: "warn", Detail: fmt.Sprintf("%d MB free", free>>20)})
		default:
			add(api.Check{Name: "disk", Status: "ok", Detail: fmt.Sprintf("%.1f GB free", float64(free)/(1<<30))})
		}
	}
	if q, err := e.Queue.Stats(ctx); err == nil {
		switch {
		case q.Failed24 > 0:
			add(api.Check{Name: "queue", Status: "warn", Detail: fmt.Sprintf("%d deliveries failed in the last 24 hours", q.Failed24), Fix: "lfr queue --status failed"})
		case q.Retrying > 0:
			add(api.Check{Name: "queue", Status: "warn", Detail: fmt.Sprintf("%d deliveries are being retried", q.Retrying), Fix: "lfr queue"})
		default:
			add(api.Check{Name: "queue", Status: "ok", Detail: fmt.Sprintf("%d queued, %d sent in 24h", q.Queued, q.Sent24h)})
		}
	}
	if paused, _ := e.Lists.All(ctx); len(paused) > 0 {
		for _, l := range paused {
			if l.Paused != "" {
				add(api.Check{Name: "list " + l.Slug, Status: "fail", Detail: "paused: " + l.Paused, Fix: "lfr list set " + l.Slug + " --resume"})
			}
		}
	}
	return out
}

func containsIP(addrs []net.IPAddr, ip string) bool {
	for _, a := range addrs {
		if a.IP.String() == ip {
			return true
		}
	}
	return false
}

func ipList(addrs []net.IPAddr) string {
	var s []string
	for _, a := range addrs {
		s = append(s, a.IP.String())
	}
	return strings.Join(s, ", ")
}

// relayWebhookCheck distinguishes an unused endpoint from one that received valid notifications.
func (e *Engine) relayWebhookCheck(ctx context.Context, url string) api.Check {
	c := api.Check{Name: "relay webhooks", Status: "skip", Detail: "point bounce and complaint notifications at " + url + " (see lfr relay)"}
	v, _ := e.DB.Setting(ctx, "relay.last_hook", "")
	at, provider, _ := strings.Cut(v, " ")
	if when, err := time.Parse(time.RFC3339, at); err == nil {
		c.Status = "ok"
		c.Detail = provider + ": last notification " + when.Format(time.RFC3339)
	}
	return c
}
