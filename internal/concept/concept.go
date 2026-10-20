// Package concept reads concept.md, the editorial brief of a newsletter: who
// it is for, what it promises, how each issue is built, how often it comes,
// how long it is, how it sounds, and when to worry. Sections are fixed, the
// words are the author's, and decisions cite research by ID (V012 a voice,
// P003 a peer newsletter, E101 evidence). A few lines are machine-readable
// ("Every: weekly", "Words: 400-900", "Use: …") so lfr can hold every issue to
// the brief.
package concept

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/finding"
)

// File is concept.md in the project root.
const File = "concept.md"

// Minimums the brief must meet.
const (
	MinVoiceNotes = 8
	MinAxes       = 3
	MinFormat     = 2
	MaxFormat     = 8
	MinPillars    = 2
	MaxPillars    = 6
	MinExamples   = 3
	MinUse        = 2
	MinAvoid      = 2
)

// Spec is one section and whether it may be omitted.
type Spec struct {
	Name     string
	Optional bool
	Help     string
}

// Sections are the sections of concept.md in order.
var Sections = []Spec{
	{"Diagnosis", true, "only when an existing newsletter is rebuilt: what works, what is generic, why readers stay or leave"},
	{"Reader", false, "who subscribes, where they come from, what they want from each issue"},
	{"Promise", false, "first line: one sentence of at most 160 characters for the signup page; then why it is credible"},
	{"Voices", false, "coded observations from the voices, each citing its IDs"},
	{"Axes", false, "table | Axis | Market | Readers | Chosen | where peers cluster and readers disagree"},
	{"Format", false, "numbered parts of a typical issue: 1. <part>: <job>"},
	{"Cadence", false, "Every: daily|weekly|biweekly|monthly|<n>d|<n>w, optional Day:, Time: HH:MM, Zone: <IANA>, and why it is sustainable"},
	{"Length", false, "Words: <min>-<max> for the body, and why"},
	{"Subject", false, "Max: <characters>, the style, and at least three Example: \"…\" lines"},
	{"Voice", false, "tone in prose, Use: \"…\" phrases from readers and Avoid: \"…\" words"},
	{"Pillars", false, "the recurring themes: - <name>: <what it covers>"},
	{"Signup", false, "Headline:, Pitch: and Frequency: for the website: what the form promises and how often mail comes"},
	{"Welcome", false, "what the welcome issue (welcome.md) says and why"},
	{"Growth", false, "how readers find the newsletter without paid ads"},
	{"Guardrails", false, "Unsubscribes: <n>% and Complaints: <n>% per issue that trigger a review, and what then"},
	{"Changelog", true, "dated changes and the research batch or learning behind them"},
}

// Line is one source line.
type Line struct {
	N    int
	Text string
}

// Section is the content under one "## " heading.
type Section struct {
	Name  string
	Line  int
	Lines []Line
}

// Text joins a section's lines.
func (s *Section) Text() string {
	if s == nil {
		return ""
	}
	var b []string
	for _, l := range s.Lines {
		b = append(b, l.Text)
	}
	return strings.Join(b, "\n")
}

// Field returns the values of "Key: value" lines in a section.
func (s *Section) Field(key string) []Line {
	if s == nil {
		return nil
	}
	var out []Line
	for _, l := range s.Lines {
		t := strings.TrimPrefix(strings.TrimSpace(l.Text), "- ")
		if v, ok := strings.CutPrefix(t, key+": "); ok {
			out = append(out, Line{l.N, strings.TrimSpace(v)})
		}
	}
	return out
}

// Cadence is how often issues go out.
type Cadence struct {
	Every        time.Duration
	Label        string // as written
	Day          time.Weekday
	HasDay       bool
	Hour, Minute int
	HasTime      bool
	Zone         *time.Location
}

// Concept is a parsed brief with its machine-readable fields.
type Concept struct {
	Title        string
	Sections     map[string]*Section
	Promise      string
	Cadence      Cadence
	MinWords     int
	MaxWords     int
	MaxSubject   int
	Examples     []string
	Use          []string
	Avoid        []string
	Pillars      []string
	Headline     string
	Pitch        string
	Frequency    string  // how often mail comes, in the readers' language, shown at signup
	MaxUnsub     float64 // percent
	MaxComplaint float64 // percent
	Open         []Line
}

// citation matches a research ID: V012 (a voice), P003 (a peer newsletter),
// E101 (evidence) from the project's research/, or the same with "lfr:" for
// lfr's built-in craft research (lfr evidence).
var citation = regexp.MustCompile(`\b(?:lfr:)?[VPE][0-9]{3,6}\b`)

// Citations returns the research IDs cited in text.
func Citations(text string) []string { return citation.FindAllString(text, -1) }

// Rule IDs.
const (
	RuleFormat   = "concept.format"
	RuleEvidence = "concept.evidence"
	RuleOpen     = "concept.open"
)

// Rules describes the concept checks.
func Rules() []finding.Rule {
	return []finding.Rule{
		{ID: RuleFormat, Summary: "concept.md has every section in order and readable fields", Help: "The brief has fixed sections (lfr skills concept) and machine-readable lines (Every:, Words:, Max:, Use:, Avoid:, Headline:, Unsubscribes: …). A missing section is a decision nobody made; an unreadable field cannot hold the issues to the brief."},
		{ID: RuleEvidence, Summary: "Decisions cite research that exists", Help: "Voices, axes and pillars cite V, P and E IDs from research/ (or lfr's built-in evidence, lfr evidence). A decision without evidence is taste, and taste drifts toward the typical newsletter."},
		{ID: RuleOpen, Summary: "Open questions for the owner are answered", Help: "Open: lines name what only the owner can decide (price, name, personal stories). They stay findings until answered in the brief."},
	}
}

func quote(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 50 {
		s = string(r[:50]) + "…"
	}
	return "\"" + s + "\""
}

// Parse reads concept.md and reports format defects. ids are the citable
// research IDs; nil skips citation checks.
func Parse(text string, ids map[string]bool) (Concept, []finding.Finding) {
	c := Concept{Sections: map[string]*Section{}}
	var fs []finding.Finding
	add := func(rule string, line int, sev, msg, fix string) {
		fs = append(fs, finding.Finding{Rule: rule, Severity: sev, File: File, Line: line, Message: msg, Fix: fix})
	}
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
	if t, ok := strings.CutPrefix(lines[0], "# Concept: "); ok && strings.TrimSpace(t) != "" {
		c.Title = strings.TrimSpace(t)
	} else {
		add(RuleFormat, 1, finding.Error, "The file must start with \"# Concept: <newsletter>\".", "Restore the title line.")
	}
	index := map[string]int{}
	for i, s := range Sections {
		index[s.Name] = i
	}
	var cur *Section
	last := -1
	for i := 1; i < len(lines); i++ {
		line, n := strings.TrimRight(lines[i], " \t"), i+1
		if name, ok := strings.CutPrefix(line, "## "); ok {
			name = strings.TrimSpace(name)
			k, known := index[name]
			switch {
			case !known:
				add(RuleFormat, n, finding.Error, "Unknown section "+quote(name)+".", "Use only the sections of the format (lfr skills concept), in order.")
				cur = nil
				continue
			case c.Sections[name] != nil:
				add(RuleFormat, n, finding.Error, "Section "+name+" appears twice.", "Merge both into one.")
				cur = nil
				continue
			case k < last:
				add(RuleFormat, n, finding.Error, "Section "+name+" is out of order.", "Keep the order of the format.")
			}
			last = k
			cur = &Section{Name: name, Line: n}
			c.Sections[name] = cur
			continue
		}
		if strings.TrimSpace(line) == "" || cur == nil {
			if cur == nil && strings.TrimSpace(line) != "" {
				add(RuleFormat, n, finding.Error, "Text before the first section.", "Move it into a section.")
			}
			continue
		}
		cur.Lines = append(cur.Lines, Line{n, line})
		if q, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")), "Open: "); ok {
			c.Open = append(c.Open, Line{n, q})
		}
	}
	for _, s := range Sections {
		sec := c.Sections[s.Name]
		switch {
		case sec == nil && !s.Optional:
			add(RuleFormat, 1, finding.Error, "Section "+s.Name+" is missing.", "Add \"## "+s.Name+"\": "+s.Help+".")
		case sec != nil && len(sec.Lines) == 0 && !s.Optional:
			add(RuleFormat, sec.Line, finding.Error, "Section "+s.Name+" is empty.", s.Help+".")
		}
	}
	c.fields(add)
	if ids != nil {
		for _, s := range c.Sections {
			for _, l := range s.Lines {
				for _, id := range Citations(l.Text) {
					if !ids[id] {
						add(RuleEvidence, l.N, finding.Error, id+" is not in the research.", "Cite an ID from research/ or lfr evidence, or remove it.")
					}
				}
			}
		}
		c.checkEvidence(add)
	}
	for _, o := range c.Open {
		add(RuleOpen, o.N, finding.Warning, "Open question: "+o.Text, "Ask the owner and write the answer into the brief.")
	}
	return c, fs
}

type adder func(rule string, line int, sev, msg, fix string)

var wordRange = regexp.MustCompile(`^(\d+)\s*[-–]\s*(\d+)$`)

func (c *Concept) fields(add adder) {
	sec := c.Sections
	if p := sec["Promise"]; p != nil && len(p.Lines) > 0 {
		c.Promise = strings.TrimSpace(Citation(p.Lines[0].Text))
		switch n := len([]rune(c.Promise)); {
		case n == 0:
			add(RuleFormat, p.Lines[0].N, finding.Error, "The promise is empty.", "Write one sentence: what readers get, how often.")
		case n > 160:
			add(RuleFormat, p.Lines[0].N, finding.Warning, fmt.Sprintf("The promise has %d characters; the signup page shows at most 160.", n), "Cut it to one plain sentence.")
		}
	}
	if s := sec["Cadence"]; s != nil {
		every := s.Field("Every")
		if len(every) == 0 {
			add(RuleFormat, s.Line, finding.Error, "Cadence has no Every: line.", "Write Every: weekly (or daily, biweekly, monthly, 3d, 2w).")
		} else if d, ok := ParseEvery(every[0].Text); ok {
			c.Cadence.Every, c.Cadence.Label = d, every[0].Text
		} else {
			add(RuleFormat, every[0].N, finding.Error, "Every: "+quote(every[0].Text)+" is not a cadence.", "Use daily, weekly, biweekly, monthly, or a count of days or weeks like 3d or 2w.")
		}
		if day := s.Field("Day"); len(day) > 0 {
			if wd, ok := weekday(day[0].Text); ok {
				c.Cadence.Day, c.Cadence.HasDay = wd, true
			} else {
				add(RuleFormat, day[0].N, finding.Error, "Day: "+quote(day[0].Text)+" is not a weekday.", "Write the day in English, e.g. Day: Tuesday.")
			}
		}
		if t := s.Field("Time"); len(t) > 0 {
			if hm, err := time.Parse("15:04", t[0].Text); err == nil {
				c.Cadence.Hour, c.Cadence.Minute, c.Cadence.HasTime = hm.Hour(), hm.Minute(), true
			} else {
				add(RuleFormat, t[0].N, finding.Error, "Time: "+quote(t[0].Text)+" is not HH:MM.", "Write a 24-hour time, e.g. Time: 07:30.")
			}
		}
		c.Cadence.Zone = time.UTC
		if z := s.Field("Zone"); len(z) > 0 {
			if loc, err := time.LoadLocation(z[0].Text); err == nil {
				c.Cadence.Zone = loc
			} else {
				add(RuleFormat, z[0].N, finding.Error, "Zone: "+quote(z[0].Text)+" is not a time zone.", "Use an IANA name such as Europe/Berlin.")
			}
		}
	}
	if s := sec["Length"]; s != nil {
		w := s.Field("Words")
		if len(w) == 0 {
			add(RuleFormat, s.Line, finding.Error, "Length has no Words: line.", "Write Words: 400-900 (the body's range).")
		} else if m := wordRange.FindStringSubmatch(w[0].Text); m != nil {
			c.MinWords, _ = strconv.Atoi(m[1])
			c.MaxWords, _ = strconv.Atoi(m[2])
			if c.MinWords < 30 || c.MaxWords > 6000 || c.MinWords >= c.MaxWords {
				add(RuleFormat, w[0].N, finding.Error, "Words: "+w[0].Text+" is not a sensible range.", "Use a range like 300-800, minimum below maximum, between 30 and 6000.")
			}
		} else {
			add(RuleFormat, w[0].N, finding.Error, "Words: "+quote(w[0].Text)+" is not a range.", "Write Words: <min>-<max>, e.g. Words: 400-900.")
		}
	}
	if s := sec["Subject"]; s != nil {
		if m := s.Field("Max"); len(m) == 0 {
			add(RuleFormat, s.Line, finding.Error, "Subject has no Max: line.", "Write Max: 50 (characters).")
		} else if n, err := strconv.Atoi(m[0].Text); err != nil || n < 20 || n > 100 {
			add(RuleFormat, m[0].N, finding.Error, "Max: "+quote(m[0].Text)+" is not a character count from 20 to 100.", "Phones show about 40 characters; most briefs pick 40 to 60.")
		} else {
			c.MaxSubject = n
		}
		for _, e := range s.Field("Example") {
			ex := strings.Trim(e.Text, "\"“”")
			c.Examples = append(c.Examples, ex)
			if c.MaxSubject > 0 && len([]rune(ex)) > c.MaxSubject {
				add(RuleFormat, e.N, finding.Warning, "Example "+quote(ex)+" is longer than Max.", "Shorten the example or raise Max.")
			}
		}
		if len(c.Examples) < MinExamples {
			add(RuleFormat, s.Line, finding.Error, fmt.Sprintf("Subject has %d Example lines; the brief needs %d.", len(c.Examples), MinExamples), "Add Example: \"…\" lines in the style the subjects should follow.")
		}
	}
	if s := sec["Voice"]; s != nil {
		for _, u := range s.Field("Use") {
			c.Use = append(c.Use, phrases(u.Text)...)
		}
		for _, a := range s.Field("Avoid") {
			c.Avoid = append(c.Avoid, phrases(a.Text)...)
		}
		if len(c.Use) < MinUse || len(c.Avoid) < MinAvoid {
			add(RuleFormat, s.Line, finding.Error, fmt.Sprintf("Voice has %d Use and %d Avoid phrases; the brief needs %d and %d.", len(c.Use), len(c.Avoid), MinUse, MinAvoid), "Add Use: \"…\" with readers' own words and Avoid: \"…\" with words the issues must not say.")
		}
	}
	if s := sec["Pillars"]; s != nil {
		for _, l := range s.Lines {
			if name, _, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(l.Text), "- "), ":"); ok && strings.HasPrefix(strings.TrimSpace(l.Text), "- ") {
				c.Pillars = append(c.Pillars, strings.ToLower(strings.TrimSpace(name)))
			}
		}
		if len(c.Pillars) < MinPillars || len(c.Pillars) > MaxPillars {
			add(RuleFormat, s.Line, finding.Error, fmt.Sprintf("%d pillars; a newsletter needs %d to %d.", len(c.Pillars), MinPillars, MaxPillars), "List them as - <name>: <what it covers>.")
		}
	}
	if s := sec["Format"]; s != nil {
		n := 0
		for _, l := range s.Lines {
			if numbered(l.Text) {
				n++
			}
		}
		if n < MinFormat || n > MaxFormat {
			add(RuleFormat, s.Line, finding.Error, fmt.Sprintf("Format lists %d parts; an issue has %d to %d.", n, MinFormat, MaxFormat), "Number the parts of a typical issue: 1. <part>: <job>.")
		}
	}
	if s := sec["Signup"]; s != nil {
		if h := s.Field("Headline"); len(h) > 0 {
			c.Headline = Citation(h[0].Text)
			if len([]rune(c.Headline)) > 80 {
				add(RuleFormat, h[0].N, finding.Warning, "The signup headline is longer than 80 characters.", "Shorten it; it is the first line of the website.")
			}
		} else {
			add(RuleFormat, s.Line, finding.Error, "Signup has no Headline: line.", "Write Headline: <the website's first line>.")
		}
		if p := s.Field("Pitch"); len(p) > 0 {
			c.Pitch = Citation(p[0].Text)
		} else {
			add(RuleFormat, s.Line, finding.Error, "Signup has no Pitch: line.", "Write Pitch: <one or two sentences under the headline>.")
		}
		if f := s.Field("Frequency"); len(f) > 0 {
			c.Frequency = Citation(f[0].Text)
			if len([]rune(c.Frequency)) > 80 {
				add(RuleFormat, f[0].N, finding.Warning, "Frequency: is longer than 80 characters.", "Say it short, like \"Every Tuesday, five minutes to read\".")
			}
		} else {
			add(RuleFormat, s.Line, finding.Error, "Signup has no Frequency: line.", "Write Frequency: <how often mail comes, as the signup page says it>; readers who know what to expect stay (lfr evidence frequency).")
		}
	}
	if s := sec["Guardrails"]; s != nil {
		c.MaxUnsub = percent(s, "Unsubscribes", add)
		c.MaxComplaint = percent(s, "Complaints", add)
	}
}

// Citation strips research IDs from text, for fields shown to readers.
func Citation(text string) string {
	return strings.Join(strings.Fields(citation.ReplaceAllString(text, "")), " ")
}

func percent(s *Section, key string, add adder) float64 {
	f := s.Field(key)
	if len(f) == 0 {
		add(RuleFormat, s.Line, finding.Error, "Guardrails has no "+key+": line.", "Write "+key+": <percent per issue>, e.g. "+key+": 0.5%.")
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.Fields(f[0].Text + " ")[0], "%")), 64)
	if err != nil || v <= 0 || v > 20 {
		add(RuleFormat, f[0].N, finding.Error, key+": "+quote(f[0].Text)+" is not a percentage.", "Write it like "+key+": 0.5%.")
		return 0
	}
	return v
}

var quoted = regexp.MustCompile(`"([^"]+)"|“([^”]+)”`)

func phrases(s string) []string {
	var out []string
	for _, m := range quoted.FindAllStringSubmatch(s, -1) {
		p := m[1]
		if p == "" {
			p = m[2]
		}
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func numbered(s string) bool {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && strings.HasPrefix(s[i:], ". ")
}

func weekday(s string) (time.Weekday, bool) {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if strings.EqualFold(s, d.String()) || strings.EqualFold(s, d.String()[:3]) {
			return d, true
		}
	}
	return 0, false
}

// ParseEvery reads a cadence such as weekly or 3d.
func ParseEvery(s string) (time.Duration, bool) {
	day := 24 * time.Hour
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "daily":
		return day, true
	case "weekdays":
		return day, true
	case "weekly":
		return 7 * day, true
	case "biweekly", "fortnightly":
		return 14 * day, true
	case "monthly":
		return 30 * day, true
	case "quarterly":
		return 91 * day, true
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 1 || n > 365 {
		return 0, false
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(n) * day, true
	case 'w':
		return time.Duration(n) * 7 * day, true
	}
	return 0, false
}

func (c *Concept) checkEvidence(add adder) {
	if s := c.Sections["Voices"]; s != nil {
		notes, cited := 0, 0
		for _, l := range s.Lines {
			if strings.HasPrefix(strings.TrimSpace(l.Text), "- ") {
				notes++
				if len(Citations(l.Text)) > 0 {
					cited++
				} else {
					add(RuleEvidence, l.N, finding.Warning, "Voice note without a citation.", "Cite the voices (V…) that say it.")
				}
			}
		}
		if notes < MinVoiceNotes {
			add(RuleEvidence, s.Line, finding.Error, fmt.Sprintf("%d voice notes; the brief needs %d.", notes, MinVoiceNotes), "Code more of what readers said: wants, dreads, habits, words.")
		}
	}
	if s := c.Sections["Axes"]; s != nil {
		rows := 0
		for _, l := range s.Lines {
			t := strings.TrimSpace(l.Text)
			if !strings.HasPrefix(t, "|") || strings.Contains(t, "---") || strings.HasPrefix(t, "| Axis") {
				continue
			}
			rows++
			if len(Citations(t)) == 0 {
				add(RuleEvidence, l.N, finding.Warning, "Axis without citations.", "Cite the peers (P…) holding the market position and the voices (V…) holding the readers' position.")
			}
		}
		if rows < MinAxes {
			add(RuleEvidence, s.Line, finding.Error, fmt.Sprintf("%d axes; the brief needs %d.", rows, MinAxes), "Add | Axis | Market | Readers | Chosen | rows where peers and readers separate.")
		}
	}
}

// Next returns the first send slot after last (the previous issue; zero for
// none) that is not before now: one cadence period later, moved to the
// brief's weekday and time in its zone.
func (c Cadence) Next(last, now time.Time) time.Time {
	zone := c.Zone
	if zone == nil {
		zone = time.UTC
	}
	t := now
	if !last.IsZero() && c.Every > 0 {
		// Leave slack so an issue sent a day or two late does not push the
		// whole schedule: weekly on Fridays after a Saturday send is still
		// the coming Friday.
		if due := last.Add(c.Every * 5 / 7); due.After(t) {
			t = due
		}
	}
	t = t.In(zone)
	slot := func(t time.Time) time.Time {
		if c.HasTime {
			return time.Date(t.Year(), t.Month(), t.Day(), c.Hour, c.Minute, 0, 0, zone)
		}
		return t
	}
	for i := 0; i < 400; i++ {
		cand := slot(t.AddDate(0, 0, i))
		if c.HasDay && cand.Weekday() != c.Day {
			continue
		}
		if !cand.Before(t) {
			return cand
		}
	}
	return t
}
