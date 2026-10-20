package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/concept"
	"github.com/Anonyfox/lfr/internal/editorial"
	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/prose"
	"github.com/Anonyfox/lfr/internal/research"
)

// Phases, in the order a newsletter moves through them. lfr check names
// exactly one, with the next step and the skill that covers it.
const (
	PhaseSetup    = "setup"    // lfr.json is broken
	PhaseResearch = "research" // collect voices and peers until the minimums hold
	PhaseConcept  = "concept"  // write or repair the editorial brief
	PhaseLaunch   = "launch"   // welcome issue, list settings and website match the brief
	PhaseLearn    = "learn"    // a sent issue has numbers nobody wrote down
	PhasePlan     = "plan"     // the backlog is too thin to pick a good next issue
	PhaseDraft    = "draft"    // write the next issue from the backlog
	PhaseFix      = "fix"      // a draft breaks a rule
	PhaseTest     = "test"     // push the draft and send a test copy
	PhaseApprove  = "approve"  // the owner approves the tested draft
	PhaseSend     = "send"     // schedule the tested draft for its slot
	PhaseWait     = "wait"     // the next issue is scheduled; nothing to do
)

// Skills names the skill for each phase.
var Skills = map[string]string{
	PhaseSetup: "lfr-setup", PhaseResearch: "lfr-research", PhaseConcept: "lfr-concept", PhaseLaunch: "lfr-launch",
	PhaseLearn: "lfr-learn", PhasePlan: "lfr-issue", PhaseDraft: "lfr-issue", PhaseFix: "lfr-issue", PhaseTest: "lfr-issue",
	PhaseApprove: "lfr-issue", PhaseSend: "lfr-issue", PhaseWait: "",
}

// Thresholds of the plan and learn phases.
const (
	MinOpenIdeas   = 4              // fewer open ideas and the next pick is forced, not chosen
	SettleTime     = 48 * time.Hour // numbers of an issue settle after two days
	MaxDraftsAhead = 3              // more unsent drafts than this and the backlog rots in issues/
)

// Rule IDs of the project checks.
const (
	RuleConfig    = "project.config"
	RuleDraft     = "project.draft"
	RuleBacklog   = "project.backlog"
	RuleLearnings = "project.learnings"
	RuleSync      = "project.sync"
	RuleCadence   = "project.cadence"
	RuleGuardrail = "project.guardrail"

	RuleSkills          = "project.skills"
	RuleBacklogFormat   = "backlog.format"
	RuleLearningsFormat = "learnings.format"
	RuleServer          = "project.server"
	RuleImages          = "project.images"
)

// Rules describes the project checks.
func Rules() []finding.Rule {
	return []finding.Rule{
		{ID: RuleConfig, Summary: "lfr.json names the list, language and autonomy", Help: "lfr.json ties the folder to a list on the server. Without it nothing can be pushed or compared."},
		{ID: RuleDraft, Summary: "Every issue file names its idea and fits the brief", Help: "A draft cites its backlog idea in front matter (idea: I007) so the backlog knows what was written, and passes the editorial rules with the brief's length, subject and voice."},
		{ID: RuleSkills, Summary: "The project's skills match this lfr", Help: "Skills installed by an older lfr describe commands and rules as they were. lfr skills install updates every copy you did not edit; lfr skills <topic> prints the current one."},
		{ID: RuleBacklogFormat, Summary: "backlog.md follows its format", Help: "Every idea is a heading \"### I007 · title · pillar · added YYYY-MM-DD · status\", one or two sentences of angle, and a Sources: line (lfr skills issue). IDs are never reused; statuses are focus, planned, idea, drafted, sent or dropped."},
		{ID: RuleLearningsFormat, Summary: "learnings.md follows its format", Help: "Every entry is the heading lfr issue show <issue> --learn prints, then Keep: and Change: lines that say what the next issue keeps and changes (lfr skills learn)."},
		{ID: RuleBacklog, Summary: "The backlog holds enough open ideas across the pillars", Help: fmt.Sprintf("At least %d open ideas, every pillar represented, and statuses that match the issues: drafted when a file exists, sent when it went out. A thin backlog forces whatever is left into the next issue.", MinOpenIdeas)},
		{ID: RuleLearnings, Summary: "Every sent issue has a learning", Help: "Two days after an issue goes out its numbers settle. learnings.md records them with what to keep and what to change; the next issue starts from there."},
		{ID: RuleSync, Summary: "The server shows what the brief says", Help: "The list's description, cadence text and welcome issue come from concept.md and welcome.md. lfr push copies them; a drift means the signup page promises something else than the brief."},
		{ID: RuleCadence, Summary: "Issues come as often as the brief promises", Help: "Readers who signed up for weekly and get nothing for a month forget they subscribed and mark the next issue as spam. Readers who get two in a day unsubscribe."},
		{ID: RuleGuardrail, Summary: "Unsubscribes and complaints stay under the brief's guardrails", Help: "An issue above the brief's Unsubscribes: or Complaints: percentage needs a learning that says what changes. Gmail and Yahoo start filtering senders above 0.3% complaints."},
		{ID: RuleImages, Summary: "Images next to the drafts are ready to upload", Help: "A draft can reference images by a path next to it (![alt](crumb.jpg)); lfr issue put uploads them and the list's website serves them. They must exist and be JPEG, PNG or GIF under 1 MB; above 300 KB they slow the issue down on phones (lfr:E122)."},
		{ID: RuleServer, Summary: "The server can be asked", Help: "lfr check compares the files with the list on the server through the control socket. Run it on the server (or over ssh) to see the whole state; without it only the files are checked."},
	}
}

// Remote is what the server knows about the project's list. The CLI fills
// it; a nil Remote means the files are checked alone.
type Remote struct {
	Error  string        `json:"error,omitempty"` // why the server could not be asked
	List   *RemoteList   `json:"list,omitempty"`  // nil: the list does not exist
	Issues []RemoteIssue `json:"issues,omitempty"`
}

// RemoteList is the list as the server has it.
type RemoteList struct {
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Cadence     string        `json:"cadence"`
	Headline    string        `json:"headline"`
	Pitch       string        `json:"pitch"`
	Language    string        `json:"language"`
	Confirmed   int           `json:"confirmed"`
	Paused      string        `json:"paused,omitempty"`
	MinGap      time.Duration `json:"min_gap"`
}

// RemoteIssue is an issue as the server has it. Markdown is filled for drafts
// and scheduled issues only.
type RemoteIssue struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	Subject   string    `json:"subject"`
	Preheader string    `json:"preheader,omitempty"`
	Markdown  string    `json:"-"`
	Status    string    `json:"status"`
	Welcome   bool      `json:"welcome,omitempty"`
	Scheduled time.Time `json:"scheduled,omitzero"`
	Started   time.Time `json:"started,omitzero"`
	Tested    time.Time `json:"tested,omitzero"`
	Stats     *Numbers  `json:"stats,omitempty"`
	// Findings are the server's checks of the rendered issue: the legal
	// footer, size, images. Only for issues not sent yet.
	Findings []finding.Finding `json:"findings,omitempty"`
}

// Numbers are the counts of a sent issue the learn phase records.
type Numbers struct {
	Recipients   int `json:"recipients"`
	Sent         int `json:"sent"`
	Bounced      int `json:"bounced"`
	Clicked      int `json:"clicked"`
	Replies      int `json:"replies"`
	Unsubscribed int `json:"unsubscribed"`
	Complained   int `json:"complained"`
}

// Rate returns n as a percentage of delivered mail.
func (n Numbers) Rate(x int) float64 {
	if n.Sent == 0 {
		return 0
	}
	return 100 * float64(x) / float64(n.Sent)
}

// Line is the heading of a learnings.md entry for an issue.
func (n Numbers) Line(slug string, sent time.Time) string {
	return fmt.Sprintf("### %s · sent %s · recipients %d · clicks %d · replies %d · unsubscribes %d · complaints %d",
		slug, sent.Format(time.DateOnly), n.Recipients, n.Clicked, n.Replies, n.Unsubscribed, n.Complained)
}

// sent reports whether the issue went (or is going) out.
func (w RemoteIssue) sent() bool { return w.Status == "sending" || w.Status == "sent" }

// Options steer a check.
type Options struct {
	Now     time.Time
	Builtin map[string]bool // citable IDs of lfr's built-in research ("lfr:E012")
	Remote  *Remote
}

// Report is the result of lfr check.
type Report struct {
	Dir      string            `json:"dir"`
	List     string            `json:"list"`
	Autonomy string            `json:"autonomy"`
	Phase    string            `json:"phase"`
	Next     string            `json:"next"`
	Skill    string            `json:"skill,omitempty"`
	Slot     time.Time         `json:"slot,omitzero"` // when the next issue is due
	Server   string            `json:"server"`        // connected, or why not
	Research research.Summary  `json:"research"`
	Ideas    int               `json:"open_ideas"`
	Drafts   []Draft           `json:"drafts"`
	Findings []finding.Finding `json:"findings"`

	Concept *concept.Concept `json:"-"`
	Backlog []Idea           `json:"-"`
	Config  Config           `json:"-"`
}

func (r *Report) add(f ...finding.Finding) { r.Findings = append(r.Findings, f...) }

func (r *Report) note(rule, sev, file string, line int, msg, fix string) {
	r.add(finding.Finding{Rule: rule, Severity: sev, File: file, Line: line, Message: msg, Fix: fix})
}

func (r *Report) set(phase, next string) {
	r.Phase, r.Next, r.Skill = phase, next, Skills[phase]
}

func blocking(fs []finding.Finding, file string) *finding.Finding {
	for i := range fs {
		if fs[i].Severity == finding.Error && (file == "" || strings.HasPrefix(fs[i].File, file)) {
			return &fs[i]
		}
	}
	return nil
}

func read(dir, rel string) (string, bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	return string(b), err == nil, err
}

// Check reads the project in dir, holds every file to its format and the
// issues to the brief, compares them with the server when opt.Remote is set,
// and names the one next step.
func Check(dir string, opt Options) (*Report, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	r := &Report{Dir: dir, Server: "not asked", Findings: []finding.Finding{}, Drafts: []Draft{}}
	cfg, err := LoadConfig(dir)
	if err != nil {
		r.note(RuleConfig, finding.Error, ConfigFile, 0, err.Error(), "Repair lfr.json: {\"schemaVersion\": 1, \"list\": \"<slug>\", \"language\": \"en\", \"owner\": \"you@example.com\", \"autonomy\": \"draft\"}.")
		r.set(PhaseSetup, "repair "+ConfigFile)
		return r, nil
	}
	r.Config, r.List, r.Autonomy = cfg, cfg.List, cfg.Mode()
	if cfg.Owner == "" {
		// A note until there is something to test; a warning from the first draft on.
		sev := finding.Info
		if ds, _ := Drafts(dir); len(ds) > 0 {
			sev = finding.Warning
		}
		r.note(RuleConfig, sev, ConfigFile, 0, "No owner address: test copies have nowhere to go.", "Add \"owner\": \"you@example.com\" to lfr.json.")
	}
	if rm := opt.Remote; rm != nil {
		if rm.Error != "" {
			r.Server = rm.Error
			r.note(RuleServer, finding.Info, "", 0, "The server was not asked: "+rm.Error, "Run lfr check on the server (or over ssh) to compare the files with the list.")
			opt.Remote = nil
		} else {
			r.Server = "connected"
		}
	}

	// Research.
	files := map[string][]byte{}
	for _, f := range []string{research.VoicesFile, research.PeersFile, research.EvidenceFile} {
		if text, ok, err := read(dir, "research/"+f); err != nil {
			return nil, err
		} else if ok {
			files[f] = []byte(text)
		}
	}
	rr := research.Check(files, "research", opt.Now)
	r.Research = rr.Summary
	r.add(rr.Findings...)
	ids := rr.IDs()
	for id := range opt.Builtin {
		ids[id] = true
	}

	// Concept.
	ctext, hasConcept, err := read(dir, concept.File)
	if err != nil {
		return nil, err
	}
	var brief editorial.Brief
	if hasConcept {
		c, fs := concept.Parse(ctext, ids)
		r.Concept = &c
		r.add(fs...)
		brief = editorial.Brief{MaxSubject: c.MaxSubject, MinWords: c.MinWords, MaxWords: c.MaxWords, Use: c.Use, Avoid: c.Avoid}
	}

	// Backlog and learnings.
	var pillars []string
	if r.Concept != nil {
		pillars = r.Concept.Pillars
	}
	btext, _, err := read(dir, BacklogFile)
	if err != nil {
		return nil, err
	}
	ideas, fs := ParseBacklog(btext, pillars)
	r.Backlog = ideas
	r.add(fs...)
	ltext, _, err := read(dir, LearningsFile)
	if err != nil {
		return nil, err
	}
	learnings, fs := ParseLearnings(ltext)
	r.add(fs...)
	learned := map[string]bool{}
	for _, l := range learnings {
		learned[l.Slug] = true
		keep, change := false, false
		for _, n := range l.Notes {
			keep = keep || strings.HasPrefix(n, "Keep:")
			change = change || strings.HasPrefix(n, "Change:")
		}
		if !keep || !change {
			r.note(RuleLearnings, finding.Warning, LearningsFile, l.Line, "The learning for "+l.Slug+" lacks a Keep: or a Change: line.", "Write what the next issue keeps and what it changes, citing the numbers or replies behind it.")
		}
	}
	byIdea := map[string]*Idea{}
	for i := range ideas {
		byIdea[ideas[i].Code()] = &ideas[i]
	}

	// Drafts.
	drafts, err := Drafts(dir)
	if err != nil {
		return nil, err
	}
	welcomeText, hasWelcome, err := read(dir, WelcomeFile)
	if err != nil {
		return nil, err
	}
	var welcome *Draft
	if hasWelcome {
		d := ParseDraft(WelcomeFile, welcomeText)
		d.Welcome = true
		welcome = &d
		r.add(d.inFile(editorial.Check(editorial.Issue{File: WelcomeFile, Subject: d.Subject, Preheader: d.Preheader, Markdown: d.Markdown, Language: cfg.Language},
			editorial.Brief{MaxSubject: brief.MaxSubject, Use: brief.Use, Avoid: brief.Avoid}))...)
	}
	// serverFindings adds what the server found in its copy of d and the
	// local checks cannot see, such as a list without a postal address.
	serverFindings := func(d Draft, w *RemoteIssue) {
		if w == nil || !d.Same(w) {
			return
		}
		seen := map[string]bool{}
		for _, f := range r.Findings {
			if f.File == d.File {
				seen[f.Rule+f.Message] = true
			}
		}
		for _, f := range w.Findings {
			if !seen[f.Rule+f.Message] && f.Severity != finding.Info {
				f.File = d.File
				r.add(f)
			}
		}
	}
	remote := map[string]*RemoteIssue{}
	var lastSent time.Time
	if opt.Remote != nil {
		for i := range opt.Remote.Issues {
			w := &opt.Remote.Issues[i]
			remote[w.Slug] = w
			if w.sent() && !w.Welcome && w.Started.After(lastSent) {
				lastSent = w.Started
			}
		}
	}
	slugs := map[string]string{}
	var pending []Draft // not sent yet
	// Issues already out: the server says so, or the backlog marks their idea sent.
	var earlier []prose.Earlier
	for _, d := range drafts {
		w := remote[d.Slug]
		if w != nil && w.sent() || byIdea[d.Idea] != nil && byIdea[d.Idea].Status == "sent" {
			earlier = append(earlier, prose.Earlier{File: d.File, Markdown: d.Markdown})
		}
	}
	for _, d := range drafts {
		if other, dup := slugs[d.Slug]; dup {
			r.note(RuleDraft, finding.Error, d.File, 0, "Slug "+d.Slug+" is also used by "+other+".", "Give each issue its own slug (file name or slug: in front matter).")
		}
		slugs[d.Slug] = d.File
		if w := remote[d.Slug]; w != nil && w.sent() {
			if idea := byIdea[d.Idea]; idea != nil && idea.Status != "sent" {
				r.note(RuleBacklog, finding.Warning, BacklogFile, idea.Line, idea.Code()+" went out as "+d.Slug+" but is marked "+idea.Status+".", "Set its status to sent.")
			}
			continue
		}
		r.add(d.inFile(editorial.Check(editorial.Issue{File: d.File, Subject: d.Subject, Preheader: d.Preheader, Markdown: d.Markdown, Language: cfg.Language, Earlier: earlier}, brief))...)
		serverFindings(d, remote[d.Slug])
		switch idea := byIdea[d.Idea]; {
		case d.Idea == "":
			r.note(RuleDraft, finding.Warning, d.File, 1, "The draft names no backlog idea.", "Add idea: I0xx to the front matter; every issue comes from an idea with sources.")
		case idea == nil:
			r.note(RuleDraft, finding.Error, d.File, 1, "Idea "+d.Idea+" is not in the backlog.", "Use the ID of an idea in backlog.md, or add the idea there first.")
		case idea.Status != "drafted":
			r.note(RuleBacklog, finding.Warning, BacklogFile, idea.Line, idea.Code()+" has a draft ("+d.File+") but is marked "+idea.Status+".", "Set its status to drafted.")
		}
		pending = append(pending, d)
	}
	r.Drafts = append(r.Drafts, drafts...)
	for _, d := range append(pending, welcomeDraft(welcome)...) {
		r.add(checkImages(dir, d)...)
	}
	if len(pending) > MaxDraftsAhead {
		r.note(RuleDraft, finding.Info, IssuesDir, 0, fmt.Sprintf("%d drafts wait unsent; ideas age in drafts too.", len(pending)), "Send or drop drafts before writing more.")
	}

	open := 0
	covered := map[string]bool{}
	for _, i := range ideas {
		if i.Open() {
			open++
			covered[i.Pillar] = true
		}
	}
	r.Ideas = open
	if r.Concept != nil && len(ideas) > 0 {
		for _, p := range pillars {
			if !covered[p] {
				r.note(RuleBacklog, finding.Info, BacklogFile, 0, "No open idea for the pillar "+p+".", "Add one, or the pillar fades from the newsletter.")
			}
		}
	}

	// Server.
	var list *RemoteList
	if opt.Remote != nil {
		list = opt.Remote.List
		if list != nil && r.Concept != nil {
			if list.Description != r.Concept.Promise {
				r.note(RuleSync, finding.Warning, concept.File, 0, "The list's description differs from the brief's promise.", "lfr push")
			}
			if list.Headline != r.Concept.Headline || list.Pitch != r.Concept.Pitch {
				r.note(RuleSync, finding.Warning, concept.File, 0, "The website's headline or pitch differs from the brief's Signup.", "lfr push")
			}
			if list.Cadence != r.Concept.Frequency {
				r.note(RuleSync, finding.Warning, concept.File, 0, "The list's cadence text differs from the brief's Frequency:.", "lfr push")
			}
			if list.Language != cfg.Language {
				r.note(RuleSync, finding.Warning, ConfigFile, 0, "The list's language is "+list.Language+", the project's "+cfg.Language+".", "lfr push")
			}
		}
		if list != nil && list.Paused != "" {
			r.note(RuleGuardrail, finding.Error, "", 0, "The list is paused and sends nothing: "+list.Paused,
				"Find the cause in the replies, lfr reports and the learnings; then lfr list set "+list.Slug+" --resume")
		}
		if welcome != nil && list != nil {
			if w := remote[welcome.Slug]; w == nil || !w.Welcome || !welcome.Same(w) {
				r.note(RuleSync, finding.Warning, WelcomeFile, 0, "The server's welcome issue is not welcome.md.", "lfr push")
			} else {
				serverFindings(*welcome, w)
			}
		}
		if list != nil && r.Concept != nil && r.Concept.Cadence.Every > 0 && !lastSent.IsZero() {
			every := r.Concept.Cadence.Every
			if late := opt.Now.Sub(lastSent); late > every+every/2 && !anyScheduled(opt.Remote.Issues) {
				r.note(RuleCadence, finding.Warning, concept.File, 0, fmt.Sprintf("The last issue went out %s ago; the brief promises %s.", days(late), r.Concept.Cadence.Label),
					"Send the next issue soon, or change Every: in the brief and say so in the next issue.")
			}
		}
		for _, w := range opt.Remote.Issues {
			if !w.sent() || w.Welcome || w.Stats == nil || r.Concept == nil {
				continue
			}
			// Once learnings.md says what changes, the breach is handled.
			unsubSev, complaintSev := finding.Warning, finding.Error
			if learned[w.Slug] {
				unsubSev, complaintSev = finding.Info, finding.Info
			}
			if u := w.Stats.Rate(w.Stats.Unsubscribed); r.Concept.MaxUnsub > 0 && u > r.Concept.MaxUnsub && w.Stats.Sent >= 50 {
				r.note(RuleGuardrail, unsubSev, LearningsFile, 0, fmt.Sprintf("%s lost %.2f%% to unsubscribes, above the brief's %.2f%%.", w.Slug, u, r.Concept.MaxUnsub), "The learning names what drove it away and what changes.")
			}
			if c := w.Stats.Rate(w.Stats.Complained); r.Concept.MaxComplaint > 0 && c > r.Concept.MaxComplaint && w.Stats.Sent >= 50 {
				r.note(RuleGuardrail, complaintSev, LearningsFile, 0, fmt.Sprintf("%s drew %.2f%% spam complaints, above the brief's %.2f%%.", w.Slug, c, r.Concept.MaxComplaint),
					"Record in "+LearningsFile+" what caused it and what changes (lfr issue show "+w.Slug+" --learn); review consent, cadence and the subject before the next issue.")
			}
		}
	}
	if r.Concept != nil {
		r.Slot = r.Concept.Cadence.Next(lastSent, opt.Now)
	}
	finding.Sort(r.Findings)

	// The one next step.
	switch {
	case !rr.Complete:
		r.set(PhaseResearch, researchNext(rr))
		return r, nil
	case r.Concept == nil:
		r.set(PhaseConcept, "write "+concept.File+" from the research")
		return r, nil
	case blocking(r.Findings, concept.File) != nil:
		f := blocking(r.Findings, concept.File)
		r.set(PhaseConcept, fixText(*f))
		return r, nil
	case len(r.Concept.Open) > 0:
		r.set(PhaseConcept, "ask the owner: "+r.Concept.Open[0].Text)
		return r, nil
	case welcome == nil:
		r.set(PhaseLaunch, "write "+WelcomeFile+" (the welcome issue)")
		return r, nil
	case blocking(r.Findings, WelcomeFile) != nil:
		f := blocking(r.Findings, WelcomeFile)
		r.set(PhaseLaunch, fixText(*f))
		return r, nil
	}
	if opt.Remote != nil {
		if list == nil {
			r.set(PhaseLaunch, fmt.Sprintf("create the list: lfr list add %s --from <address> --name %q --language %s, then lfr push", cfg.List, r.Concept.Title, cfg.Language))
			return r, nil
		}
		for _, f := range r.Findings {
			if f.Rule == RuleSync {
				r.set(PhaseLaunch, "lfr push (copies the promise, cadence and welcome issue to the server)")
				return r, nil
			}
		}
		for _, w := range opt.Remote.Issues {
			if w.Status == "sent" && !w.Welcome && !learned[w.Slug] && opt.Now.Sub(w.Started) >= SettleTime {
				r.set(PhaseLearn, fmt.Sprintf("lfr issue show %s --learn, then add the entry to %s", w.Slug, LearningsFile))
				return r, nil
			}
		}
	}
	// A crossed guardrail or a paused list is the learn step's work, at once:
	// find why readers objected before anything else goes out.
	for _, f := range r.Findings {
		if f.Rule == RuleGuardrail && f.Severity == finding.Error {
			r.set(PhaseLearn, strings.TrimSuffix(fixText(f), ".")+"; read the replies, then record what changes in "+LearningsFile)
			return r, nil
		}
	}
	if f := blocking(r.Findings, ""); f != nil {
		r.set(PhaseFix, fixText(*f))
		return r, nil
	}
	if len(pending) == 0 {
		if open < MinOpenIdeas {
			r.set(PhasePlan, fmt.Sprintf("add ideas to %s: %d open, at least %d keep the choice real", BacklogFile, open, MinOpenIdeas))
			return r, nil
		}
		next := pick(ideas)
		r.set(PhaseDraft, fmt.Sprintf("write %s/<slug>.md from %s (%s), due %s", IssuesDir, next.Code(), next.Title, slotText(r.Slot)))
		return r, nil
	}
	d := pending[0]
	if opt.Remote == nil {
		r.set(PhaseTest, fmt.Sprintf("on the server: lfr issue put %s, then lfr issue test %s %s", d.File, d.Slug, owner(cfg)))
		return r, nil
	}
	w := remote[d.Slug]
	warnings := 0
	for _, f := range r.Findings {
		if f.File == d.File && f.Severity == finding.Warning {
			warnings++
		}
	}
	switch {
	case (w == nil || !d.Same(w)) && warnings > 0:
		// Warnings do not block, but each is a decision: rewrite, or keep on purpose.
		r.set(PhaseFix, fmt.Sprintf("%d %s in %s: rewrite each passage from your material, keep one only on purpose; then lfr issue put %s, then lfr issue test %s %s",
			warnings, plural(warnings, "warning", "warnings"), d.File, d.File, d.Slug, owner(cfg)))
	case w == nil || !d.Same(w):
		r.set(PhaseTest, fmt.Sprintf("lfr issue put %s, then lfr issue test %s %s", d.File, d.Slug, owner(cfg)))
	case w.Tested.IsZero():
		r.set(PhaseTest, fmt.Sprintf("lfr issue test %s %s", d.Slug, owner(cfg)))
	case w.Status == "scheduled":
		r.Slot = w.Scheduled.In(r.Slot.Location())
		if open < MinOpenIdeas {
			r.set(PhasePlan, fmt.Sprintf("%s goes out %s; meanwhile add ideas to %s (%d open)", d.Slug, slotText(r.Slot), BacklogFile, open))
		} else {
			r.set(PhaseWait, fmt.Sprintf("%s goes out %s; run lfr check after it was sent", d.Slug, slotText(r.Slot)))
		}
	case cfg.Mode() == AutonomyDraft:
		r.set(PhaseApprove, fmt.Sprintf("ask the owner to approve %s (test copy went to %s); on approval: lfr issue send %s --at %s", d.Slug, owner(cfg), d.Slug, r.Slot.Format(time.RFC3339)))
	default:
		r.set(PhaseSend, fmt.Sprintf("lfr issue send %s --at %s", d.Slug, r.Slot.Format(time.RFC3339)))
	}
	return r, nil
}

func researchNext(rr research.Report) string {
	s := rr.Summary
	if s.Voices == 0 && s.Peers == 0 {
		return "collect reader voices into research/voices.md and analyze peer newsletters in research/peers.md"
	}
	// Broken lines first: they may hide entries from the counts.
	for _, f := range rr.Findings {
		if f.Rule == research.RuleFormat {
			return "fix " + f.Where() + ": " + f.Message
		}
	}
	voices, peers := rr.Short()
	switch {
	case voices:
		return fmt.Sprintf("collect more voices: %d of %d, from %d of %d platforms, or name the shortfall in the batch heading", s.Voices, research.MinVoices, s.Platforms, research.MinPlatforms)
	case peers:
		return fmt.Sprintf("analyze more peer newsletters: %d of %d, or name the shortfall in the batch heading", s.Peers, research.MinPeers)
	}
	return "complete the research"
}

// pick chooses the next idea: owner requests first, then planned, then the
// oldest idea.
func pick(ideas []Idea) Idea {
	for _, status := range []string{"focus", "planned", "idea"} {
		for _, i := range ideas {
			if i.Status == status {
				return i
			}
		}
	}
	return Idea{}
}

func anyScheduled(ws []RemoteIssue) bool {
	for _, w := range ws {
		if w.Status == "scheduled" && !w.Welcome {
			return true
		}
	}
	return false
}

func owner(c Config) string {
	if c.Owner == "" {
		return "<owner address>"
	}
	return c.Owner
}

func days(d time.Duration) string {
	if d < 48*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	return fmt.Sprintf("%d days", int(d.Hours()/24))
}

func slotText(t time.Time) string {
	if t.IsZero() {
		return "now"
	}
	return t.Format("Mon 2006-01-02 15:04 MST")
}

func welcomeDraft(w *Draft) []Draft {
	if w == nil {
		return nil
	}
	return []Draft{*w}
}

// checkImages holds the images a draft references by path to what lfr
// issue put will accept.
func checkImages(dir string, d Draft) []finding.Finding {
	var out []finding.Finding
	base := filepath.Join(dir, filepath.Dir(filepath.FromSlash(d.File)))
	for _, p := range emailmd.LocalImages(d.Markdown) {
		data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(p)))
		if err != nil {
			out = append(out, finding.Finding{Rule: RuleImages, Severity: finding.Error, File: d.File, Message: "Image " + p + " is not there.", Fix: "Put the file next to the draft, or fix the path."})
			continue
		}
		if _, err := emailmd.CheckImage(p, data); err != nil {
			out = append(out, finding.Finding{Rule: RuleImages, Severity: finding.Error, File: d.File, Message: err.Error() + ".", Fix: "Save it as JPEG, PNG or GIF under 1 MB."})
		} else if len(data) > emailmd.HeavyImage {
			out = append(out, finding.Finding{Rule: RuleImages, Severity: finding.Warning, File: d.File, Message: fmt.Sprintf("Image %s is %d KB.", p, len(data)>>10), Fix: "Resize it to about 1200 pixels wide and compress it; most readers are on phones."})
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// fixText is the next step for a blocking finding: where and what, and the
// command when the fix is one.
func fixText(f finding.Finding) string {
	t := f.Message
	if w := f.Where(); w != "" {
		t = "fix " + w + ": " + t
	}
	if strings.HasPrefix(f.Fix, "lfr ") {
		t = strings.TrimSuffix(t, ".") + "; run: " + f.Fix
	}
	return t
}
