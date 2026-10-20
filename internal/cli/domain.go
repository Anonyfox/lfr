package cli

import (
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
)

func printRecords(c *call, d api.DomainInfo, zone bool) {
	if zone {
		c.printf("%s", d.Zone)
		return
	}
	for i, r := range d.Records {
		proxy := "DNS only"
		if r.Proxy {
			proxy = "DNS only, or proxied once HTTPS works"
		}
		c.printf("%2d. %-5s %s\n", i+1, r.Type, r.Name)
		if r.Type == "MX" {
			c.printf("    priority %d  %s\n", r.Priority, r.Value)
		} else {
			c.printf("    %s\n", r.Value)
		}
		c.printf("    (%s; %s)\n\n", r.Purpose, proxy)
	}
	for _, n := range d.Notes {
		c.printf("%s\n", n)
	}
}

func runDomainAdd(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var d api.DomainInfo
	if err := c.do(api.OpDomainAdd, api.Name{Name: c.args[0]}, &d); err != nil {
		return err
	}
	return c.result(d, func() {
		c.printf("Serving %s. Ask the domain owner to publish these DNS records:\n\n", d.Name)
		printRecords(c, d, false)
		c.next("once published: lfr domain dns %s --check (postmaster@%s and abuse@%s exist already)", d.Name, d.Name, d.Name)
	})
}

func runDomainList(c *call) error {
	var ds []api.DomainInfo
	if err := c.do(api.OpDomainList, nil, &ds); err != nil {
		return err
	}
	return c.result(ds, func() {
		var rows [][]string
		for _, d := range ds {
			rows = append(rows, []string{d.Name, d.Selector, d.Catchall, d.Created.Local().Format("2006-01-02")})
		}
		table(c.env.Stdout, []string{"domain", "dkim selector", "catch-all", "added"}, rows)
		if len(ds) == 0 {
			c.next("lfr domain add example.com")
		}
	})
}

func runDomainDNS(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var d api.DomainInfo
	if err := c.do(api.OpDomainDNS, api.DomainDNS{Name: c.args[0], Check: c.has("check")}, &d); err != nil {
		return err
	}
	bad := 0
	err := c.result(d, func() {
		if !c.has("check") {
			printRecords(c, d, c.has("zone"))
			return
		}
		for _, ch := range d.Checks {
			status := "ok"
			if ch.Status != "ok" {
				status = "fail"
				bad++
			}
			c.printf("%-5s %-5s %-40s %s\n", paint(c.env.Stdout, status), ch.Record.Type, ch.Record.Name, ch.Status)
			if ch.Status != "ok" {
				c.printf("      want: %s\n", ch.Record.Value)
				if len(ch.Found) > 0 {
					c.printf("      found: %s\n", strings.Join(ch.Found, " | "))
				}
			}
		}
		if bad == 0 {
			c.next("DNS is complete; lfr doctor checks the rest")
		} else {
			c.next("publish the %d records marked fail (DNS changes can take minutes), then check again", bad)
		}
	})
	if err == nil && bad > 0 {
		return &silentErr{}
	}
	return err
}

func runDomainRotate(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var d api.DomainInfo
	if err := c.do(api.OpDomainRotate, api.Name{Name: c.args[0]}, &d); err != nil {
		return err
	}
	return c.result(d, func() {
		if d.Next != "" {
			c.printf("New DKIM keys for %s under selector %s; they sign nothing yet. Publish the two \"next DKIM key\" records below, then run lfr domain rotate %s again to switch.\n\n", d.Name, d.Next, d.Name)
			printRecords(c, d, false)
			return
		}
		c.printf("%s now signs with selector %s. Keep the old _domainkey records published for a week, then remove them.\n", d.Name, d.Selector)
	})
}

func runDomainRemove(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if !c.has("yes") {
		return usage("this deletes every mailbox, message, list and issue of %s; repeat with --yes", c.args[0])
	}
	if err := c.do(api.OpDomainRemove, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"removed": c.args[0]}, func() { c.printf("Removed %s\n", c.args[0]) })
}

// --- mailboxes ---

func runDomainSet(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if !c.has("catchall") {
		return usage("nothing to change; --catchall <mailbox|off>")
	}
	target := c.str("catchall")
	if err := c.do(api.OpCatchall, api.Catchall{Domain: c.args[0], Target: target}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"domain": c.args[0], "catchall": target}, func() {
		if target == "off" {
			c.printf("Unknown addresses of %s are refused again\n", c.args[0])
		} else {
			c.printf("Unknown addresses of %s go to %s (expect more spam)\n", c.args[0], target)
		}
	})
}
