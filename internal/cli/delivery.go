package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/outbound"
)

func printRelay(c *call, r api.RelayInfo) {
	kv(c.env.Stdout, "mode", r.Mode, "relay", r.URL, "spf", r.SPF)
	c.printf("\nBounce and complaint webhooks (point your relay's notifications here):\n")
	for _, k := range sortedKeys(r.Webhooks) {
		c.printf("  %-8s %s\n", k, r.Webhooks[k])
	}
}

func runRelaySet(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	pass := c.str("password")
	if !c.has("password") {
		b, err := io.ReadAll(io.LimitReader(c.env.Stdin, 4096))
		if err != nil {
			return err
		}
		pass = strings.TrimSpace(string(b))
	}
	var r api.RelayInfo
	if err := c.do(api.OpRelaySet, api.RelaySet{URL: c.args[0], Password: pass, SPF: c.str("spf")}, &r); err != nil {
		return err
	}
	return c.result(r, func() { printRelay(c, r) })
}

func runRelayShow(c *call) error {
	var r api.RelayInfo
	if err := c.do(api.OpRelayShow, nil, &r); err != nil {
		return err
	}
	return c.result(r, func() { printRelay(c, r) })
}

func runRelayTest(c *call) error {
	var ch api.Check
	if err := c.do(api.OpRelayTest, nil, &ch); err != nil {
		return err
	}
	err := c.result(ch, func() {
		c.printf("%s %s\n", paint(c.env.Stdout, ch.Status), ch.Detail)
		if ch.Fix != "" {
			c.printf("fix: %s\n", ch.Fix)
		}
	})
	if err == nil && ch.Status != "ok" {
		return &silentErr{}
	}
	return err
}

func runRelayOff(c *call) error {
	var r api.RelayInfo
	if err := c.do(api.OpRelaySet, api.RelaySet{}, &r); err != nil {
		return err
	}
	return c.result(r, func() {
		c.printf("Delivering directly to recipients' mail servers.\n")
		c.next("lfr relay checks that outbound port 25 is open")
	})
}

func runQueue(c *call) error {
	a := api.QueueList{Status: c.str("status")}
	var err error
	if a.Limit, err = intFlag(c, "limit"); err != nil {
		return err
	}
	var res struct {
		Stats   outbound.Status  `json:"stats"`
		Entries []outbound.Entry `json:"entries"`
	}
	if err := c.do(api.OpQueueList, a, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		s := res.Stats
		c.printf("%d queued (%d retrying), last 24h: %d sent, %d failed, %d bounced\n\n", s.Queued, s.Retrying, s.Sent24h, s.Failed24, s.Bounce24)
		var rows [][]string
		for _, e := range res.Entries {
			rows = append(rows, []string{strconv.FormatInt(e.ID, 10), e.Kind, e.Rcpt, e.Status, strconv.Itoa(e.Attempts), when(e.Next), cut(e.Error, 60)})
		}
		if len(rows) > 0 {
			table(c.env.Stdout, []string{"id", "kind", "to", "status", "tries", "next", "last error"}, rows)
		}
	})
}

func runQueueRetry(c *call) error {
	var n int64
	if len(c.args) == 1 {
		var err error
		if n, err = id(c.args[0]); err != nil {
			return err
		}
	} else if len(c.args) > 1 {
		return usage("one ID at most")
	}
	var res map[string]int
	if err := c.do(api.OpQueueRetry, api.ID{ID: n}, &res); err != nil {
		return err
	}
	return c.result(res, func() { c.printf("Retrying %d deliveries now\n", res["retrying"]) })
}

func runQueueDrop(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	n, err := id(c.args[0])
	if err != nil {
		return err
	}
	if err := c.do(api.OpQueueDrop, api.ID{ID: n}, nil); err != nil {
		return err
	}
	return c.result(map[string]int64{"dropped": n}, func() { c.printf("Dropped %d\n", n) })
}

func runSpamStats(c *call) error {
	var res struct {
		Stats   map[string]any     `json:"stats"`
		Weights map[string]float64 `json:"weights"`
	}
	if err := c.do(api.OpSpamStats, nil, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		kv(c.env.Stdout, "trained spam", fmt.Sprint(res.Stats["trained_spam"]), "trained ham", fmt.Sprint(res.Stats["trained_ham"]),
			"bayes active", fmt.Sprint(res.Stats["bayes_active"]), "tokens", fmt.Sprint(res.Stats["tokens"]), "reputation keys", fmt.Sprint(res.Stats["reputation_keys"]))
		c.printf("\nweights:\n")
		for _, k := range sortedKeys(res.Weights) {
			c.printf("  %-18s %6.2f\n", k, res.Weights[k])
		}
		c.next("teach it: lfr mail mv <id> spam for missed spam, lfr mail mv <id> inbox for wrongly junked mail")
	})
}

func runSpamReset(c *call) error {
	if !c.has("yes") {
		return usage("this forgets everything the filter learned; repeat with --yes")
	}
	if err := c.do(api.OpSpamReset, nil, nil); err != nil {
		return err
	}
	return c.result(map[string]bool{"reset": true}, func() { c.printf("Spam filter reset to its starting rules\n") })
}

func runReports(c *call) error {
	a := api.Reports{Kind: c.str("kind"), Domain: c.str("domain")}
	var err error
	if a.Limit, err = intFlag(c, "limit"); err != nil {
		return err
	}
	var rs []api.Report
	if err := c.do(api.OpReports, a, &rs); err != nil {
		return err
	}
	return c.result(rs, func() {
		var rows [][]string
		var failures []string
		dmarcFails, tlsFails := false, false
		for _, r := range rs {
			rows = append(rows, []string{r.Kind, r.Domain, r.Org, r.Begin.UTC().Format("2006-01-02"), strconv.Itoa(r.Pass), strconv.Itoa(r.Fail)})
			if r.Fail == 0 {
				continue
			}
			// What failed and where: the reports say it, and the operator acts on it.
			var details []map[string]any
			json.Unmarshal(r.Rows, &details)
			for _, d := range details {
				switch r.Kind {
				case "dmarc":
					if d["dkim"] == "pass" || d["spf"] == "pass" {
						continue
					}
					dmarcFails = true
					failures = append(failures, fmt.Sprintf("  %s %s: %v messages from %v failed DKIM and SPF", r.Domain, r.Org, d["count"], d["ip"]))
				case "tlsrpt":
					fds, _ := d["failure-details"].([]any)
					for _, fd := range fds {
						m, _ := fd.(map[string]any)
						tlsFails = true
						failures = append(failures, fmt.Sprintf("  %s %s: %v TLS sessions to %v failed: %v", r.Domain, r.Org, m["failed-session-count"], m["receiving-mx-hostname"], m["result-type"]))
					}
				}
			}
		}
		if len(rows) == 0 {
			c.printf("No reports yet. Large providers send DMARC reports daily once your _dmarc record is live.\n")
			return
		}
		table(c.env.Stdout, []string{"kind", "domain", "reporter", "day", "pass", "fail"}, rows)
		if len(failures) > 0 {
			c.printf("\nFailures:\n%s\n", strings.Join(failures, "\n"))
		}
		switch {
		case tlsFails:
			c.next("TLS failures mean senders could not reach you securely; lfr doctor checks the certificates")
		case dmarcFails:
			c.next("mail from addresses that are not this server or its relay is someone else using your domain; keep p=none until only those fail, then p=quarantine stops them")
		default:
			c.next("after a few weeks of zero failures, tighten DMARC to p=quarantine")
		}
	})
}

func runRelay(c *call) error {
	switch {
	case len(c.args) > 1:
		return usage("one relay URL, or off")
	case len(c.args) == 0:
		if err := runRelayShow(c); err != nil || c.json {
			return err
		}
		c.printf("\n")
		return runRelayTest(c)
	case c.args[0] == "off":
		return runRelayOff(c)
	}
	if err := runRelaySet(c); err != nil {
		return err
	}
	if c.json {
		return nil
	}
	c.printf("\n")
	if err := runRelayTest(c); err != nil {
		return err
	}
	c.next("point the relay's bounce and complaint notifications at the webhook URL for it above")
	return nil
}
