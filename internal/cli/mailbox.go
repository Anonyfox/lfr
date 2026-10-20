package cli

import (
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/mailbox"
)

func runMailboxAdd(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var m mailbox.Mailbox
	if err := c.do(api.OpMailboxAdd, api.MailboxAdd{Address: c.args[0], Name: c.str("name")}, &m); err != nil {
		return err
	}
	return c.result(m, func() {
		c.printf("Created %s\n", m.Address)
		c.next("lfr mail ls %s; lfr mail send --from %s --to ... --subject ... --md -", m.Address, m.Address)
	})
}

func runMailboxList(c *call) error {
	domain := ""
	if len(c.args) > 0 {
		domain = c.args[0]
	}
	var ms []mailbox.Mailbox
	if err := c.do(api.OpMailboxList, api.Name{Name: domain}, &ms); err != nil {
		return err
	}
	return c.result(ms, func() {
		var rows [][]string
		for _, m := range ms {
			extra := strings.Join(m.Aliases, ", ")
			if m.Catchall {
				extra = strings.TrimPrefix(extra+", catch-all", ", ")
			}
			if m.Forward != "" {
				extra = strings.TrimPrefix(extra+", → "+m.Forward, ", ")
			}
			rows = append(rows, []string{m.Address, strconv.Itoa(m.Unread), strconv.Itoa(m.Total), m.Name, extra})
		}
		table(c.env.Stdout, []string{"mailbox", "unread", "messages", "name", "also"}, rows)
	})
}

func runMailboxSet(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if !c.has("name") && !c.has("forward") {
		return usage("give --name and/or --forward")
	}
	var m mailbox.Mailbox
	if err := c.do(api.OpMailboxSet, api.MailboxSet{Address: c.args[0], Name: c.opt("name"), Forward: c.opt("forward")}, &m); err != nil {
		return err
	}
	return c.result(m, func() { kv(c.env.Stdout, "mailbox", m.Address, "name", m.Name, "forward", m.Forward) })
}

func runMailboxRename(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	var m mailbox.Mailbox
	if err := c.do(api.OpMailboxRename, api.Rename{From: c.args[0], To: c.args[1]}, &m); err != nil {
		return err
	}
	return c.result(m, func() { c.printf("%s is now %s\n", c.args[0], m.Address) })
}

func runMailboxRemove(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	// An alias goes without ceremony; a mailbox takes its mail with it.
	if err := c.do(api.OpAliasRemove, api.Name{Name: c.args[0]}, nil); err == nil {
		return c.result(map[string]string{"removed": c.args[0]}, func() { c.printf("Removed alias %s\n", c.args[0]) })
	} else if !isNotFound(err) {
		return err
	}
	if !c.has("yes") {
		return usage("this deletes %s with all its mail; repeat with --yes", c.args[0])
	}
	if err := c.do(api.OpMailboxRemove, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"removed": c.args[0]}, func() { c.printf("Removed %s\n", c.args[0]) })
}

func runAliasAdd(c *call) error {
	if err := exactly(c, 2); err != nil {
		return err
	}
	if err := c.do(api.OpAliasAdd, api.Alias{Alias: c.args[0], Target: c.args[1]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"alias": c.args[0], "mailbox": c.args[1]}, func() { c.printf("%s → %s\n", c.args[0], c.args[1]) })
}

// --- mail ---
