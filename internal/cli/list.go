package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/i18n"
	"github.com/Anonyfox/lfr/internal/lists"
)

func listFlags() []flag {
	return []flag{
		{name: "name", value: true, usage: "display name"}, {name: "from", value: true, usage: "sender address on a served domain"},
		{name: "from-name", value: true, usage: "sender name (default: the list name)"}, {name: "description", value: true, usage: "one sentence shown on the website"},
		{name: "host", value: true, usage: "website host, e.g. news.example.com"}, {name: "language", value: true, usage: "language of pages and mails (en/de built in; English fallback, list text overrides)"},
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
	warned := warnListLanguage(c, l)
	return c.result(l, func() {
		printList(c, l)
		if warned {
			return
		}
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
	if c.has("language") {
		warnListLanguage(c, l)
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

func warnListLanguage(c *call, l lists.List) bool {
	if i18n.Supported(l.Language) || len(l.Texts) > 0 {
		return false
	}
	fmt.Fprintf(c.env.Stderr, "Warning: %s has no built-in texts; readers see English until overrides are imported.\n", l.Language)
	fmt.Fprintf(c.env.Stderr, "Next: lfr list text %s --import nl.json\n", l.Slug)
	return true
}

func runListText(c *call) error {
	if len(c.args) < 1 {
		return usage("name a list; use lfr list text <slug>")
	}
	values := map[string]string{}
	op := api.OpListText
	var args any = api.Name{Name: c.args[0]}
	if c.has("import") {
		if len(c.args) != 1 || c.has("reset") {
			return usage("use lfr list text <slug> --import <file|->")
		}
		r := c.env.Stdin
		if c.str("import") != "-" {
			f, err := os.Open(c.str("import"))
			if err != nil {
				return err
			}
			defer f.Close()
			r = f
		}
		b, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(b) > 1<<20 {
			return usage("text import is too large; keep it below 1 MiB")
		}
		// Raw values reject null as well as non-string JSON values.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil || raw == nil {
			return usage("invalid text import; provide a flat JSON object of strings")
		}
		for k, v := range raw {
			var str string
			if string(v) == "null" || json.Unmarshal(v, &str) != nil {
				return usage("invalid text %s; provide a string value", k)
			}
			values[k] = str
		}
		op = api.OpListTextSet
	} else if c.has("reset") {
		if len(c.args) != 2 {
			return usage("use lfr list text <slug> <key> --reset")
		}
		values[c.args[1]] = ""
		op = api.OpListTextSet
	} else if len(c.args) == 3 {
		values[c.args[1]] = c.args[2]
		op = api.OpListTextSet
	} else if len(c.args) != 1 {
		return usage("use lfr list text <slug> [<key> <value>]")
	}
	if op == api.OpListTextSet {
		args = api.ListTexts{Slug: c.args[0], Values: values}
	}
	var out map[string]lists.TextValue
	if err := c.do(op, args, &out); err != nil {
		return err
	}
	return c.result(out, func() {
		keys := make([]string, 0, len(out))
		for k := range out {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			source := "built-in"
			if out[k].Overridden {
				source = "overridden"
			}
			c.printf("%s (%s): %s\n", k, source, out[k].Value)
		}
		c.next("lfr list text %s", c.args[0])
	})
}
