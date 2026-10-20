package cli

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/lists"
)

func runSubAdd(c *call) error {
	if err := need(c, 2); err != nil {
		return err
	}
	var res api.SubAddResult
	if err := c.do(api.OpSubAdd, api.SubAdd{List: c.args[0], Emails: c.args[1:], Confirmed: c.has("confirmed"), Reconfirm: c.has("reconfirm"), Source: c.str("source"), Consent: c.str("consent")}, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		c.printf("%d added, %d already there, %d asked to confirm again, %d confirmation mails queued, %d kept off (left, complained, bounced or erased)\n", res.Added, res.Existing, res.Reconfirm, res.Confirmation, res.OptedOut)
		if len(res.Invalid) > 0 {
			c.printf("invalid: %s\n", strings.Join(res.Invalid, ", "))
		}
	})
}

func runSubList(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	a := api.SubList{List: c.args[0], Status: c.str("status"), Search: c.str("search")}
	var err error
	if a.Limit, err = intFlag(c, "limit"); err != nil {
		return err
	}
	if c.has("after") {
		if a.After, err = id(c.str("after")); err != nil {
			return err
		}
	}
	var ms []lists.Member
	if err := c.do(api.OpSubList, a, &ms); err != nil {
		return err
	}
	return c.result(ms, func() {
		var rows [][]string
		for _, m := range ms {
			note := m.Source
			if m.Suppressed != "" {
				note = "suppressed: " + m.Suppressed
			}
			rows = append(rows, []string{strconv.FormatInt(m.ID, 10), m.Email, m.Status, m.SignupAt.Local().Format("2006-01-02"), cut(note, 40)})
		}
		table(c.env.Stdout, []string{"id", "email", "status", "since", "source"}, rows)
		if len(ms) > 0 && len(ms) == max(a.Limit, 100) {
			c.next("more: lfr reader ls %s --after %d", a.List, ms[len(ms)-1].ID)
		}
	})
}

func runSubRemove(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	var m lists.Member
	if err := c.do(api.OpSubRemove, api.SubRemove{List: c.args[0], Email: c.args[1]}, &m); err != nil {
		return err
	}
	return c.result(m, func() { c.printf("%s unsubscribed from %s\n", m.Email, m.List) })
}

func runSubImport(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	csv, err := readArg(c, c.args[1])
	if err != nil {
		return err
	}
	var res api.SubAddResult
	if err := c.do(api.OpSubImport, api.SubImport{List: c.args[0], CSV: csv, Confirmed: c.has("confirmed"), Reconfirm: c.has("reconfirm"), Source: c.str("source"), Consent: c.str("consent")}, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		c.printf("%d added, %d already there, %d asked to confirm again, %d confirmation mails queued, %d kept off (left, complained, bounced or erased), %d invalid\n", res.Added, res.Existing, res.Reconfirm, res.Confirmation, res.OptedOut, len(res.Invalid))
		if len(res.Invalid) > 0 {
			c.printf("invalid: %s\n", strings.Join(res.Invalid[:min(20, len(res.Invalid))], ", "))
		}
	})
}

func runSubExport(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var out api.CSV
	if err := c.do(api.OpSubExport, api.Name{Name: c.args[0]}, &out); err != nil {
		return err
	}
	if p := c.str("out"); p != "" {
		if err := os.WriteFile(p, []byte(out.CSV), 0o600); err != nil {
			return err
		}
		return c.result(map[string]any{"file": p, "rows": out.Rows}, func() { c.printf("Wrote %d members to %s\n", out.Rows, p) })
	}
	if c.json {
		return c.result(out, nil)
	}
	c.printf("%s", out.CSV)
	return nil
}

func runContactShow(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var ct lists.Contact
	if err := c.do(api.OpContactShow, api.Name{Name: c.args[0]}, &ct); err != nil {
		return err
	}
	return c.result(ct, func() {
		kv(c.env.Stdout, "email", ct.Email, "name", ct.Name, "since", when(ct.Created), "suppressed", ct.Suppressed,
			"activity", fmt.Sprintf("%d sent, %d opens, %d clicks", ct.Sent, ct.Opens, ct.Clicks))
		for _, m := range ct.Lists {
			c.printf("  %-20s %-12s signed up %s via %s", m.List, m.Status, when(m.SignupAt), m.Source)
			if !m.ConfirmAt.IsZero() {
				c.printf(", confirmed %s from %s", when(m.ConfirmAt), m.ConfirmIP)
			}
			c.printf("\n")
		}
	})
}

func runSuppress(c *call) error {
	emails := append([]string{}, c.args...)
	if c.has("csv") {
		text, err := readArg(c, c.str("csv"))
		if err != nil {
			return err
		}
		more, err := csvEmails(strings.NewReader(text))
		if err != nil {
			return usage("%s: %v", c.str("csv"), err)
		}
		emails = append(emails, more...)
	}
	if len(emails) == 0 {
		return usage("name addresses, or --csv <file>")
	}
	var res api.SubAddResult
	if err := c.do(api.OpSuppress, api.Suppress{Emails: emails, Reason: c.str("reason")}, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		if len(emails) == 1 && len(res.Invalid) == 0 {
			c.printf("%s receives nothing from any list now\n", emails[0])
			return
		}
		c.printf("%s blocked on every list, %d invalid\n", count(res.Added, "address", "addresses"), len(res.Invalid))
	})
}

// csvEmails reads the email column of a CSV export (header email, e-mail or
// email address; else the first column).
func csvEmails(r io.Reader) ([]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	col, first := 0, true
	var out []string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if first {
			first = false
			header := false
			for i, h := range rec {
				switch strings.ToLower(strings.TrimSpace(h)) {
				case "email", "e-mail", "email address", "mail":
					col, header = i, true
				}
			}
			if header {
				continue
			}
		}
		if col < len(rec) && strings.TrimSpace(rec[col]) != "" {
			out = append(out, strings.TrimSpace(rec[col]))
		}
	}
}

func runUnsuppress(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if err := c.do(api.OpUnsuppress, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"unsuppressed": c.args[0]}, func() { c.printf("%s can receive mail again\n", c.args[0]) })
}

func runForget(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if !c.has("yes") {
		return usage("this erases %s and its whole history; repeat with --yes", c.args[0])
	}
	if err := c.do(api.OpContactForget, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"forgotten": c.args[0]}, func() { c.printf("Erased %s\n", c.args[0]) })
}

// --- issues ---

func runReaderAdd(c *call) error {
	if c.has("csv") {
		if err := exactly(c, 1); err != nil {
			return usage("lfr reader add <list> --csv <file>")
		}
		c.args = append(c.args, c.str("csv"))
		return runSubImport(c)
	}
	return runSubAdd(c)
}

func runReaderList(c *call) error {
	if c.has("csv") {
		return runSubExport(c)
	}
	return runSubList(c)
}
