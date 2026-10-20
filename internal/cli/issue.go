package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/project"
)

func issueFlags() []flag {
	return []flag{{name: "subject", value: true, usage: "subject"}, {name: "slug", value: true, usage: "URL name in the archive"},
		{name: "preheader", value: true, usage: "preview text after the subject"}, {name: "summary", value: true, usage: "teaser on the website"}}
}

// projectList is the list of the newsletter project around the working
// directory, or "".
func projectList() string {
	dir, ok := project.Find(".")
	if !ok {
		return ""
	}
	cfg, err := project.LoadConfig(dir)
	if err != nil {
		return ""
	}
	return cfg.List
}

// issueRef reads an issue argument: an ID, <list>/<slug>, or, inside a
// newsletter project, a slug or a file in issues/.
func issueRef(s string) (api.IssueRef, error) {
	if n, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64); err == nil && n > 0 {
		return api.IssueRef{ID: n}, nil
	}
	if !strings.HasSuffix(s, ".md") {
		if list, slug, ok := strings.Cut(s, "/"); ok && list != "" && slug != "" {
			return api.IssueRef{List: list, Slug: slug}, nil
		}
	}
	if list := projectList(); list != "" {
		return api.IssueRef{List: list, Slug: strings.TrimSuffix(filepath.Base(s), ".md")}, nil
	}
	return api.IssueRef{}, usage("%q is not an issue: give its ID or <list>/<slug> (or a slug inside a newsletter project)", s)
}

// refArg is how follow-up hints name an issue.
func refArg(r api.IssueRef, w issues.Issue) string {
	if r.ID == 0 && w.Slug != "" && projectList() == w.List {
		return w.Slug
	}
	if w.List != "" && w.Slug != "" {
		return w.List + "/" + w.Slug
	}
	return strconv.FormatInt(w.ID, 10)
}

func issueDraft(c *call, src string) (api.IssueDraft, error) {
	md, err := readArg(c, src)
	if err != nil {
		return api.IssueDraft{}, err
	}
	return api.IssueDraft{Markdown: md, Subject: c.str("subject"), Slug: c.str("slug"), Preheader: c.str("preheader"), Summary: c.str("summary")}, nil
}

func printIssue(c *call, w issues.Issue) {
	kind := ""
	if w.Welcome {
		kind = "welcome issue: every new member gets it when they confirm"
	}
	kv(c.env.Stdout, "issue", strconv.FormatInt(w.ID, 10), "list", w.List, "slug", w.Slug, "subject", w.Subject, "preheader", w.Preheader,
		"status", w.Status, "kind", kind, "scheduled", when(w.Scheduled), "started", when(w.Started), "finished", when(w.Finished),
		"tested", when(w.Tested), "summary", cut(w.Summary, 120))
}

// localMedia reads the images Markdown references by a path relative to dir.
func localMedia(dir, markdown string) (*[]issues.File, error) {
	_, body := issues.FrontMatter(markdown)
	files := []issues.File{}
	for _, p := range emailmd.LocalImages(body) {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("image %s: %w", p, err)
		}
		files = append(files, issues.File{Path: p, Data: data})
	}
	return &files, nil
}

func runIssuePut(c *call) error {
	var list, file string
	switch len(c.args) {
	case 1:
		if list = projectList(); list == "" {
			return usage("name the list: lfr issue put <list> <file.md> (inside a newsletter project lfr.json names it)")
		}
		file = c.args[0]
	case 2:
		list, file = c.args[0], c.args[1]
	default:
		return usage("lfr issue put [list] <file.md>")
	}
	d, err := issueDraft(c, file)
	if err != nil {
		return err
	}
	d.List, d.Welcome = list, c.has("welcome")
	dir := "."
	if file != "-" {
		dir = filepath.Dir(file)
	}
	if d.Media, err = localMedia(dir, d.Markdown); err != nil {
		return err
	}
	if d.Slug == "" && file != "-" {
		if meta, _ := issues.FrontMatter(d.Markdown); meta["slug"] == "" {
			d.Slug = strings.TrimSuffix(filepath.Base(file), ".md")
		}
	}
	var res struct {
		api.IssuePut
		Findings []issues.Finding `json:"findings"` // the server's checks of the rendered issue
	}
	if err := c.do(api.OpIssuePut, d, &res); err != nil {
		return err
	}
	// One call tells the whole story: what the server holds and what it found.
	if err := c.do(api.OpIssueLint, api.IssueRef{ID: res.Issue.ID}, &res.Findings); err != nil {
		return err
	}
	return c.result(res, func() {
		verb := "Updated"
		if res.Created {
			verb = "Created"
		}
		c.printf("%s issue %s/%s\n", verb, res.Issue.List, res.Issue.Slug)
		printIssue(c, res.Issue)
		if len(res.Findings) > 0 {
			c.printf("\n")
			printFindings(c, res.Findings)
		}
		r := refArg(api.IssueRef{}, res.Issue)
		to := ownerAddress()
		switch {
		case issues.Blocking(res.Findings):
			c.next("fix the errors above; list settings with lfr list set %s, the text in the file and lfr issue put again", res.Issue.List)
		case res.Issue.Welcome:
			c.next("lfr issue test %s %s; new readers get it when they confirm", r, to)
		case res.Issue.Status == "scheduled":
			c.next("it still goes out %s, with these changes: test them first, lfr issue test %s %s (or lfr issue cancel %s)", res.Issue.Scheduled.Local().Format("Mon 2006-01-02 15:04"), r, to, r)
		default:
			c.next("lfr issue test %s %s", r, to)
		}
	})
}

func runIssueList(c *call) error {
	list := projectList()
	switch {
	case len(c.args) == 1:
		list = c.args[0]
	case len(c.args) > 1:
		return usage("too many arguments")
	case list == "":
		return usage("name the list: lfr issue ls <list>")
	}
	var ws []issues.Issue
	if err := c.do(api.OpIssueList, api.IssueList{List: list}, &ws); err != nil {
		return err
	}
	return c.result(ws, func() {
		var rows [][]string
		for _, w := range ws {
			at := w.Started
			if at.IsZero() {
				at = w.Scheduled
			}
			status := w.Status
			if w.Welcome {
				status = "welcome"
			}
			rows = append(rows, []string{strconv.FormatInt(w.ID, 10), w.Slug, status, when(at), cut(w.Subject, 50)})
		}
		table(c.env.Stdout, []string{"id", "slug", "status", "sent", "subject"}, rows)
	})
}

// oneIssue runs an issue op on the single issue argument.
func oneIssue(c *call, op string, out any) (api.IssueRef, error) {
	if err := exactly(c, 1); err != nil {
		return api.IssueRef{}, err
	}
	ref, err := issueRef(c.args[0])
	if err != nil {
		return ref, err
	}
	return ref, c.do(op, ref, out)
}

func runIssueShow(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	ref, err := issueRef(c.args[0])
	if err != nil {
		return err
	}
	var w issues.Issue
	if err := c.do(api.OpIssueShow, ref, &w); err != nil {
		return err
	}
	if c.has("learn") {
		return issueLearn(c, ref, w)
	}
	if c.str("html") != "" || c.has("text") {
		return issuePreview(c, ref)
	}
	arg := c.args[0]
	view := struct {
		Issue    issues.Issue     `json:"issue"`
		Findings []issues.Finding `json:"findings,omitempty"`
		Stats    *issues.Stats    `json:"stats,omitempty"`
	}{Issue: w}
	draft := w.Status == "draft" || w.Status == "scheduled"
	if draft {
		if err := c.do(api.OpIssueLint, ref, &view.Findings); err != nil {
			return err
		}
	} else {
		view.Stats = &issues.Stats{}
		if err := c.do(api.OpIssueStats, ref, view.Stats); err != nil {
			return err
		}
	}
	err = c.result(view, func() {
		printIssue(c, w)
		if draft {
			c.printf("\n")
			printFindings(c, view.Findings)
			switch {
			case issues.Blocking(view.Findings):
				c.next("fix the errors: list settings with lfr list set %s, the text in the file and lfr issue put again; then lfr issue show %s", w.List, arg)
			case w.Tested.IsZero():
				c.next("lfr issue test %s %s", arg, ownerAddress())
			default:
				c.next("lfr issue send %s", arg)
			}
			return
		}
		printStats(c, *view.Stats)
		c.next("lfr issue show %s --learn two days after sending", arg)
	})
	if err == nil && draft && issues.Blocking(view.Findings) {
		return &silentErr{}
	}
	return err
}

func issuePreview(c *call, ref api.IssueRef) error {
	var p api.Preview
	if err := c.do(api.OpIssuePreview, ref, &p); err != nil {
		return err
	}
	if out := c.str("html"); out != "" {
		if err := os.WriteFile(out, []byte(p.HTML), 0o644); err != nil {
			return err
		}
	}
	return c.result(p, func() {
		kv(c.env.Stdout, "subject", p.Subject, "size", fmt.Sprintf("%d KB", p.Size/1024), "web", p.WebURL, "html file", c.str("html"))
		if c.has("text") {
			c.printf("\n%s\n", p.Text)
		}
	})
}

func printStats(c *call, s issues.Stats) {
	of := func(x, total int) string {
		if total == 0 {
			return strconv.Itoa(x)
		}
		return fmt.Sprintf("%d (%.1f%%)", x, 100*float64(x)/float64(total))
	}
	measured := func(x int) string {
		if !s.Tracking {
			return "not measured (tracking off)"
		}
		return of(x, s.Sent)
	}
	c.printf("\n")
	kv(c.env.Stdout, "recipients", strconv.Itoa(s.Recipients), "queued", strconv.Itoa(s.Queued), "delivered", strconv.Itoa(s.Sent),
		"bounced", of(s.Bounced, s.Sent+s.Bounced), "failed", strconv.Itoa(s.Failed), "canceled", strconv.Itoa(s.Canceled),
		"opened", measured(s.Opened)+map[bool]string{true: " (includes privacy proxies)"}[s.Tracking], "clicked", measured(s.Clicked), "replies", strconv.Itoa(s.Replies),
		"unsubscribed", of(s.Unsubscribed, s.Sent), "complaints", of(s.Complained, s.Sent))
	for _, u := range sortedKeys(s.Links) {
		c.printf("  %4d  %s\n", s.Links[u], u)
	}
}

func printFindings(c *call, fs []issues.Finding) {
	if len(fs) == 0 {
		c.printf("No findings.\n")
		return
	}
	for _, f := range fs {
		where := f.Where()
		if where != "" {
			where = "  " + where
		}
		c.printf("%-7s %-24s %s%s\n        fix: %s\n", paint(c.env.Stdout, f.Severity), f.Rule, f.Message, where, f.Fix)
	}
}

func runIssueTest(c *call) error {
	if err := need(c, 2); err != nil {
		return err
	}
	ref, err := issueRef(c.args[0])
	if err != nil {
		return err
	}
	if err := c.do(api.OpIssueTest, api.IssueTest{IssueRef: ref, To: c.args[1:]}, nil); err != nil {
		return err
	}
	var fs []issues.Finding
	c.do(api.OpIssueLint, ref, &fs)
	return c.result(map[string]any{"issue": c.args[0], "to": c.args[1:]}, func() {
		c.printf("Test copies queued to %s\n", strings.Join(c.args[1:], ", "))
		if issues.Blocking(fs) {
			c.next("check them in a real inbox; before it can go out, fix the errors lfr issue show %s lists", c.args[0])
			return
		}
		c.next("check them in a real inbox (Gmail and Outlook, phone and dark mode), then lfr issue send %s", c.args[0])
	})
}

func runIssueSend(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	ref, err := issueRef(c.args[0])
	if err != nil {
		return err
	}
	var w issues.Issue
	err = c.do(api.OpIssueSend, api.IssueSend{IssueRef: ref, At: c.str("at"), Force: c.has("force")}, &w)
	if err != nil {
		var fs []issues.Finding
		if c.do(api.OpIssueLint, ref, &fs) == nil && len(fs) > 0 && !c.json {
			printFindings(c, fs)
		}
		return err
	}
	return c.result(w, func() {
		printIssue(c, w)
		c.next("lfr issue show %s (lfr events --follow shows progress live)", c.args[0])
	})
}

func runIssueCancel(c *call) error {
	var w issues.Issue
	if _, err := oneIssue(c, api.OpIssueCancel, &w); err != nil {
		return err
	}
	return c.result(w, func() { printIssue(c, w) })
}

func issueLearn(c *call, ref api.IssueRef, w issues.Issue) error {
	if w.Started.IsZero() {
		return usage("issue %s has not gone out yet", c.args[0])
	}
	var s issues.Stats
	if err := c.do(api.OpIssueStats, ref, &s); err != nil {
		return err
	}
	n := numbers(s)
	line := n.Line(w.Slug, w.Started)
	return c.result(map[string]any{"line": line, "numbers": n}, func() {
		c.printf("%s\n- Keep: \n- Change: \n", line)
		c.printf("\nClicks %.1f%%, replies %d, unsubscribes %.2f%%, complaints %.2f%% of %d delivered.\n",
			n.Rate(n.Clicked), n.Replies, n.Rate(n.Unsubscribed), n.Rate(n.Complained), n.Sent)
		if len(s.ReplyList) > 0 {
			c.printf("\nReplies (lfr mail read <id> for the whole text; they are research: lfr-research):\n")
			for _, r := range s.ReplyList {
				c.printf("  %-6d %-28s %s\n", r.ID, r.From, r.Snippet)
			}
		}
		for _, u := range sortedKeys(s.Links) {
			c.printf("  %4d  %s\n", s.Links[u], u)
		}
		c.next("append the entry to learnings.md with what the next issue keeps and changes (skill lfr-learn)")
	})
}

func runIssueRemove(c *call) error {
	if _, err := oneIssue(c, api.OpIssueRemove, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"removed": c.args[0]}, func() { c.printf("Removed issue %s\n", c.args[0]) })
}

// --- sites, relay, queue, spam, reports, backup ---

// ownerAddress is the owner of the newsletter project around the current
// directory, for test copies; a placeholder outside one.
func ownerAddress() string {
	if dir, ok := project.Find("."); ok {
		if cfg, err := project.LoadConfig(dir); err == nil && cfg.Owner != "" {
			return cfg.Owner
		}
	}
	return "you@example.com"
}
