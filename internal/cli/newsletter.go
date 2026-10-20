package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/concept"
	"github.com/Anonyfox/lfr/internal/craft"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/editorial"
	"github.com/Anonyfox/lfr/internal/engine"
	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/project"
	"github.com/Anonyfox/lfr/internal/research"
	"github.com/Anonyfox/lfr/internal/skills"
)

// --- newsletter projects ---

func runInit(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	dir := c.args[0]
	cfg := project.Config{SchemaVersion: 1, List: c.str("list"), Language: c.str("language"), Owner: c.str("owner"), Autonomy: c.str("autonomy")}
	if cfg.List == "" {
		cfg.List = strings.ToLower(filepath.Base(filepath.Clean(dir)))
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	name := c.str("name")
	if name == "" {
		name = cfg.List
	}
	written, err := project.Init(dir, cfg, name)
	if err != nil {
		return usage("%v", err)
	}
	data := skillData(c)
	var installed []string
	for _, f := range skills.Choose(dir, data, c.env.getenv) {
		changes, err := skills.Install(filepath.Join(dir, f.Rel), data, false, skills.Newsletter...)
		if err != nil {
			return err
		}
		for _, ch := range changes {
			rel, _ := filepath.Rel(dir, ch.Path)
			installed = append(installed, rel)
		}
	}
	return c.result(map[string]any{"dir": dir, "written": written, "skills": installed}, func() {
		c.printf("Newsletter project %s for list %s\n", dir, cfg.List)
		for _, w := range append(written, installed...) {
			c.printf("  wrote %s\n", w)
		}
		if cfg.Owner == "" {
			c.next("cd %s && lfr check (it names each step and the skill for it); add \"owner\": \"you@example.com\" to lfr.json for test copies", dir)
			return
		}
		c.next("cd %s && lfr check (it names each step and the skill for it)", dir)
	})
}

func projectDir(c *call) (string, error) {
	start := "."
	if len(c.args) == 1 {
		start = c.args[0]
	} else if len(c.args) > 1 {
		return "", usage("one project directory at most")
	}
	dir, ok := project.Find(start)
	if !ok {
		return "", usage("no lfr.json in %s or above; start a newsletter project with lfr init <dir> --list <slug>", start)
	}
	return dir, nil
}

// builtin returns the citable IDs of lfr's own research.
func builtin() map[string]bool { return craft.IDs() }

func runEvidence(c *call) error {
	if len(c.args) > 1 {
		return usage("one dimension or ID at a time")
	}
	arg := ""
	if len(c.args) == 1 {
		arg = strings.ToLower(c.args[0])
	}
	if arg == "" {
		counts := map[string]int{}
		for _, e := range craft.Evidence("") {
			for _, d := range e.Applies {
				counts[d]++
			}
		}
		sum := craft.Report().Summary
		return c.result(map[string]any{"summary": sum, "dimensions": counts}, func() {
			c.printf("lfr's research on newsletter craft: %d voices of readers and writers, %d peer newsletters, %d pieces of evidence.\n\n", sum.Voices, sum.Peers, sum.Evidence)
			for _, d := range research.Dimensions {
				c.printf("  %-16s %3d\n", d, counts[d])
			}
			c.next("lfr evidence <dimension> prints the evidence verbatim; cite it as lfr:E012 in concept.md and backlog.md")
		})
	}
	if slices.Contains(research.Dimensions, arg) {
		es := craft.Evidence(arg)
		return c.result(es, func() {
			for _, e := range es {
				c.printf("%s\n", craft.FormatEvidence(e))
			}
		})
	}
	text, ok := craft.Lookup(arg)
	if !ok {
		return usage("%q is neither a dimension (%s) nor a built-in ID like lfr:E012", c.args[0], strings.Join(research.Dimensions, ", "))
	}
	return c.result(map[string]string{"text": text}, func() { c.printf("%s", text) })
}

// remote asks the server what it knows about the project's list.
func remote(c *call, list string) *project.Remote {
	var l lists.List
	err := c.do(api.OpListShow, api.Name{Name: list}, &l)
	var ce *ctl.Error
	switch {
	case errors.Is(err, ctl.ErrNotRunning) && c.remote() != "":
		return &project.Remote{Error: "no lfr server runs on " + c.remote()}
	case errors.Is(err, ctl.ErrNotRunning):
		return &project.Remote{Error: "no lfr server runs here (" + c.data + "); use --remote user@host or LFR_REMOTE to reach one over ssh"}
	case errors.As(err, &ce) && ce.Code == ctl.CodeNotFound:
		return &project.Remote{}
	case err != nil:
		return &project.Remote{Error: err.Error()}
	}
	r := &project.Remote{List: &project.RemoteList{Slug: l.Slug, Name: l.Name, Description: l.Description, Cadence: l.Cadence, Headline: l.Headline, Pitch: l.Pitch,
		Language: l.Language, Confirmed: l.Confirmed, Paused: l.Paused, MinGap: l.MinGap.Duration()}}
	var ws []issues.Issue
	if err := c.do(api.OpIssueList, api.IssueList{List: list}, &ws); err != nil {
		return &project.Remote{Error: err.Error()}
	}
	for _, w := range ws {
		rw := project.RemoteIssue{ID: w.ID, Slug: w.Slug, Subject: w.Subject, Preheader: w.Preheader, Status: w.Status, Welcome: w.Welcome,
			Scheduled: w.Scheduled, Started: w.Started, Tested: w.Tested}
		switch w.Status {
		case "draft", "scheduled", "welcome":
			var full issues.Issue
			if c.do(api.OpIssueShow, api.IssueRef{ID: w.ID}, &full) == nil {
				rw.Markdown = full.Markdown
			}
			var fs []finding.Finding
			if c.do(api.OpIssueLint, api.IssueRef{ID: w.ID}, &fs) == nil {
				rw.Findings = fs
			}
		case "sending", "sent":
			var st issues.Stats
			if c.do(api.OpIssueStats, api.IssueRef{ID: w.ID}, &st) == nil {
				rw.Stats = numbers(st)
			}
		}
		r.Issues = append(r.Issues, rw)
	}
	return r
}

func numbers(st issues.Stats) *project.Numbers {
	return &project.Numbers{Recipients: st.Recipients, Sent: st.Sent, Bounced: st.Bounced, Clicked: st.Clicked,
		Replies: st.Replies, Unsubscribed: st.Unsubscribed, Complained: st.Complained}
}

func runCheck(c *call) error {
	dir, err := projectDir(c)
	if err != nil {
		return err
	}
	cfg, cerr := project.LoadConfig(dir)
	var rm *project.Remote
	if cerr == nil && !c.has("offline") {
		rm = remote(c, cfg.List)
	}
	r, err := project.Check(dir, project.Options{Now: time.Now(), Builtin: builtin(), Remote: rm})
	if err != nil {
		return err
	}
	// Skills from an older lfr teach commands and rules as they were.
	var old []string
	version := ""
	for _, cp := range skills.Find(dir, skillData(c)) {
		if cp.State == skills.Outdated {
			old, version = append(old, cp.Name), cp.Version
		}
	}
	if len(old) > 0 {
		r.Findings = append(r.Findings, finding.Finding{Rule: project.RuleSkills, Severity: finding.Info,
			Message: fmt.Sprintf("%s from lfr %s, this is lfr %s: %s.", count(len(old), "skill is", "skills are"), version, c.env.Build.Version, strings.Join(old, ", ")),
			Fix:     "lfr skills install updates every copy you did not edit; lfr skills <topic> prints the current one"})
	}
	return c.result(r, func() {
		e, w, i := finding.Count(r.Findings)
		kv(c.env.Stdout, "project", r.Dir, "list", r.List, "autonomy", r.Autonomy, "server", r.Server,
			"research", fmt.Sprintf("%s from %s, %s, %d evidence", count(r.Research.Voices, "voice", "voices"), count(r.Research.Platforms, "platform", "platforms"), count(r.Research.Peers, "peer", "peers"), r.Research.Evidence),
			"backlog", fmt.Sprintf("%d open ideas", r.Ideas), "next slot", slot(r.Slot),
			"findings", fmt.Sprintf("%s, %s, %s", count(e, "error", "errors"), count(w, "warning", "warnings"), count(i, "note", "notes")))
		if len(r.Findings) > 0 {
			c.printf("\n")
			printFindings(c, r.Findings)
		}
		skill := ""
		if r.Skill != "" {
			skill = " (" + r.Skill + ")"
		}
		c.printf("\nNext (%s): %s%s\n", r.Phase, r.Next, skill)
	})
}

func slot(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Mon 2006-01-02 15:04 MST")
}

func runPush(c *call) error {
	dir, err := projectDir(c)
	if err != nil {
		return err
	}
	cfg, err := project.LoadConfig(dir)
	if err != nil {
		return usage("%v", err)
	}
	text, err := os.ReadFile(filepath.Join(dir, concept.File))
	if err != nil {
		return usage("no %s yet: lfr check names the steps before a push", concept.File)
	}
	brief, fs := concept.Parse(string(text), nil)
	if finding.Blocking(fs) {
		printFindings(c, fs)
		return usage("%s has errors; fix them before pushing", concept.File)
	}
	// Two issues closer than half the cadence feel like a mistake; never
	// more than the default, so a weekly list can still send a correction.
	gap := lists.DefaultMinGap
	if half := brief.Cadence.Every / 2; half > 0 && half < gap {
		gap = half
	}
	gapText := gap.String()
	fields := api.ListFields{Description: &brief.Promise, Cadence: &brief.Frequency, Headline: &brief.Headline, Pitch: &brief.Pitch, Language: &cfg.Language, MinGap: &gapText}
	var l lists.List
	if err := c.do(api.OpListSet, api.ListSet{Slug: cfg.List, ListFields: fields}, &l); err != nil {
		var ce *ctl.Error
		if errors.As(err, &ce) && ce.Code == ctl.CodeNotFound {
			return usage("list %s does not exist on this server: lfr list add %s --from <address> --name %q first", cfg.List, cfg.List, brief.Title)
		}
		return err
	}
	pushed := []string{"description", "headline", "pitch", "cadence", "language", "min gap " + gapText}
	if src, err := os.ReadFile(filepath.Join(dir, project.WelcomeFile)); err == nil {
		d := project.ParseDraft(project.WelcomeFile, string(src))
		var res api.IssuePut
		media, err := localMedia(dir, string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", project.WelcomeFile, err)
		}
		if err := c.do(api.OpIssuePut, api.IssueDraft{List: cfg.List, Markdown: string(src), Slug: d.Slug, Welcome: true, Media: media}, &res); err != nil {
			return fmt.Errorf("%s: %w", project.WelcomeFile, err)
		}
		pushed = append(pushed, "welcome issue "+res.Issue.Slug)
	}
	// The website's templates, when the project keeps its own.
	if info, err := os.Stat(filepath.Join(dir, "site")); err == nil && info.IsDir() {
		files, err := siteFiles(filepath.Join(dir, "site"))
		if err != nil {
			return err
		}
		if len(files) > 0 {
			if err := c.do(api.OpSitePut, api.SitePut{List: cfg.List, Files: files}, nil); err != nil {
				return fmt.Errorf("site/: %w", err)
			}
			pushed = append(pushed, "website ("+count(len(files), "file", "files")+")")
		}
	}
	return c.result(map[string]any{"list": l.Slug, "pushed": pushed}, func() {
		c.printf("Pushed to list %s: %s\n", l.Slug, strings.Join(pushed, ", "))
		c.next("lfr check")
	})
}

type ruleGroup struct {
	name  string
	rules []finding.Rule
}

func ruleGroups() []ruleGroup {
	return []ruleGroup{
		{"issues (lfr check and lfr issue show)", editorial.Rules()},
		{"issues (server only: rendered message and list settings)", issues.Rules()},
		{"concept.md", concept.Rules()},
		{"research/", research.Rules()},
		{"project", project.Rules()},
	}
}

func runRules(c *call) error {
	groups := ruleGroups()
	if len(c.args) > 1 {
		return usage("one rule at a time")
	}
	if len(c.args) == 1 {
		for _, g := range groups {
			for _, r := range g.rules {
				if r.ID == c.args[0] {
					return c.result(r, func() { c.printf("%s: %s\n\n%s\n", r.ID, r.Summary, r.Help) })
				}
			}
		}
		return usage("no rule %q; lfr rules lists them", c.args[0])
	}
	var all []finding.Rule
	for _, g := range groups {
		all = append(all, g.rules...)
	}
	return c.result(all, func() {
		for _, g := range groups {
			c.printf("%s\n", g.name)
			for _, r := range g.rules {
				c.printf("  %-26s %s\n", r.ID, r.Summary)
			}
			c.printf("\n")
		}
		c.printf("lfr rules <id> explains one.\n")
	})
}

func runProxy(c *call) error {
	path := c.env.getenv(engine.SocketEnv)
	if path == "" {
		path = engine.SocketPath(c.data)
	}
	return ctl.Proxy(c.ctx, path, c.env.Stdin, c.env.Stdout)
}
