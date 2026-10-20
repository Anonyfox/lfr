package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/lists"
)

func listFlags() []flag {
	return []flag{
		{name: "name", value: true, usage: "display name"}, {name: "from", value: true, usage: "sender address on a served domain"},
		{name: "from-name", value: true, usage: "sender name (default: the list name)"}, {name: "description", value: true, usage: "one sentence shown on the website"},
		{name: "host", value: true, usage: "website host, e.g. news.example.com"}, {name: "language", value: true, usage: "en or de (texts of pages and mails)"},
		{name: "tracking", usage: "count opens and clicks (--tracking=false turns it off)"}, {name: "reply-to", value: true, usage: "where replies go"},
		{name: "imprint-url", value: true, usage: "imprint (Impressum) page"}, {name: "privacy-url", value: true, usage: "privacy policy page"},
		{name: "address", value: true, usage: "sender identity line in every footer (name and postal address)"},
		{name: "consent", value: true, usage: "consent statement shown under the signup form"},
		{name: "cadence", value: true, usage: "how often mail comes, shown at signup (\"Every Tuesday morning\")"},
		{name: "headline", value: true, usage: "the website's first line (default: the list's name)"},
		{name: "pitch", value: true, usage: "one or two sentences under the headline (default: the description)"},
		{name: "min-gap", value: true, usage: "time that must pass between two issues (default 20h; 0 turns it off)"},
	}
}

func listFields(c *call) api.ListFields {
	return api.ListFields{Name: c.opt("name"), Description: c.opt("description"), From: c.opt("from"), FromName: c.opt("from-name"),
		ReplyTo: c.opt("reply-to"), Host: c.opt("host"), Language: c.opt("language"), Tracking: c.boolFlag("tracking"),
		ImprintURL: c.opt("imprint-url"), PrivacyURL: c.opt("privacy-url"), Address: c.opt("address"), Consent: c.opt("consent"),
		Cadence: c.opt("cadence"), Headline: c.opt("headline"), Pitch: c.opt("pitch"), MinGap: c.opt("min-gap")}
}

func printList(c *call, l lists.List) {
	site := "https://" + l.Host + "/"
	kv(c.env.Stdout, "list", l.Slug, "name", l.Name, "from", fmt.Sprintf("%s <%s>", l.FromName, l.From), "reply-to", l.ReplyTo,
		"website", map[bool]string{true: site, false: "(on the mail host, /lists/" + l.Slug + "/)"}[l.Host != ""],
		"language", l.Language, "tracking", yes(l.Tracking), "readers", fmt.Sprintf("%d confirmed, %d pending, %d unsubscribed", l.Confirmed, l.Pending, l.Unsubscribed),
		"headline", l.Headline, "pitch", l.Pitch, "cadence", l.Cadence, "min gap", l.MinGap.Duration().String(),
		"address", orMissing(l.Address, "missing: nothing goes out without it (--address \"Name, Street 1, 12345 City\")"),
		"imprint", orMissing(l.ImprintURL, map[bool]string{true: "missing: German lists need one (--imprint-url)"}[strings.HasPrefix(l.Language, "de")]),
		"privacy", l.PrivacyURL, "consent", l.ConsentText(), "paused", l.Paused)
}

// orMissing shows v, or why its absence matters (nothing when it does not).
func orMissing(v, why string) string {
	if v != "" || why == "" {
		return v
	}
	return "(" + why + ")"
}

func runListAdd(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var l lists.List
	if err := c.do(api.OpListAdd, api.ListSet{Slug: c.args[0], ListFields: listFields(c)}, &l); err != nil {
		return err
	}
	return c.result(l, func() {
		printList(c, l)
		if l.Host != "" {
			c.next("point %s at this server (lfr domain dns %s lists the record), then lfr init <dir> --list %s starts the newsletter project", l.Host, l.Domain, l.Slug)
		} else {
			c.next("lfr init <dir> --list %s starts the newsletter project (research, concept, issues)", l.Slug)
		}
	})
}

func runListList(c *call) error {
	var ls []lists.List
	if err := c.do(api.OpListList, nil, &ls); err != nil {
		return err
	}
	return c.result(ls, func() {
		var rows [][]string
		for _, l := range ls {
			state := "active"
			if l.Paused != "" {
				state = "PAUSED"
			}
			rows = append(rows, []string{l.Slug, l.Name, l.From, l.Host, strconv.Itoa(l.Confirmed), strconv.Itoa(l.Pending), state})
		}
		table(c.env.Stdout, []string{"list", "name", "from", "website", "confirmed", "pending", "state"}, rows)
	})
}

func runListShow(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var l lists.List
	if err := c.do(api.OpListShow, api.Name{Name: c.args[0]}, &l); err != nil {
		return err
	}
	return c.result(l, func() { printList(c, l) })
}

func runListSet(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if c.has("pause") {
		return runListPause(c)
	}
	if c.has("resume") {
		return runListResume(c)
	}
	var l lists.List
	if err := c.do(api.OpListSet, api.ListSet{Slug: c.args[0], ListFields: listFields(c)}, &l); err != nil {
		return err
	}
	return c.result(l, func() { printList(c, l) })
}

func runListPause(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	reason := c.str("pause")
	if reason == "" {
		reason = "paused by operator"
	}
	var l lists.List
	if err := c.do(api.OpListPause, api.ListPause{Slug: c.args[0], Reason: reason}, &l); err != nil {
		return err
	}
	return c.result(l, func() { c.printf("Paused %s: nothing is sent until lfr list set %s --resume\n", l.Slug, l.Slug) })
}

func runListResume(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	var l lists.List
	if err := c.do(api.OpListPause, api.ListPause{Slug: c.args[0]}, &l); err != nil {
		return err
	}
	return c.result(l, func() { c.printf("Resumed %s\n", l.Slug) })
}

func runListRemove(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if !c.has("yes") {
		return usage("this deletes list %s with its members and issues; repeat with --yes", c.args[0])
	}
	if err := c.do(api.OpListRemove, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"removed": c.args[0]}, func() { c.printf("Removed %s\n", c.args[0]) })
}
