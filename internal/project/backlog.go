package project

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/finding"
)

// Statuses lists idea statuses in the order an idea moves through them.
var Statuses = []string{"focus", "idea", "planned", "drafted", "sent", "dropped"}

// Idea is one backlog entry.
type Idea struct {
	Line    int
	ID      int
	Title   string
	Pillar  string
	Added   time.Time
	Status  string
	Angle   string
	Sources []string
}

// Open reports whether the idea still waits for an issue.
func (i Idea) Open() bool { return i.Status == "focus" || i.Status == "idea" || i.Status == "planned" }

// Code is the idea's ID as written: I007.
func (i Idea) Code() string { return fmt.Sprintf("I%03d", i.ID) }

var sourceRE = regexp.MustCompile(`\b(?:lfr:)?[VPE][0-9]{3,6}\b|https?://\S+|\bowner\b`)

// ParseBacklog reads backlog.md:
//
//	### I001 · <title> · <pillar> · added YYYY-MM-DD · <status>
//	<the angle, one or more lines>
//	Sources: V012, E104, https://…, owner
func ParseBacklog(text string, pillars []string) ([]Idea, []finding.Finding) {
	var ideas []Idea
	var fs []finding.Finding
	add := func(n int, sev, msg, fix string) {
		fs = append(fs, finding.Finding{Rule: RuleBacklogFormat, Severity: sev, File: BacklogFile, Line: n, Message: msg, Fix: fix})
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if !strings.HasPrefix(lines[0], "# Backlog: ") {
		add(1, finding.Error, "The file must start with \"# Backlog: <newsletter>\".", "Restore the title line.")
	}
	format := "### I001 · <title> · <pillar> · added YYYY-MM-DD · " + strings.Join(Statuses, "|")
	var cur *Idea
	var angle []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Angle = strings.TrimSpace(strings.Join(angle, " "))
		if cur.Angle == "" {
			add(cur.Line, finding.Warning, cur.Code()+" has no angle.", "Write one or two sentences under the heading: what the issue says and why readers need it now.")
		}
		if len(cur.Sources) == 0 && cur.Status != "sent" && cur.Status != "dropped" {
			add(cur.Line, finding.Warning, cur.Code()+" has no sources.", "Add a Sources: line with research IDs, URLs you opened, or owner for the owner's own material.")
		}
		ideas = append(ideas, *cur)
		cur, angle = nil, nil
	}
	known := map[string]bool{}
	for _, p := range pillars {
		known[p] = true
	}
	for i := 1; i < len(lines); i++ {
		line, n := strings.TrimRight(lines[i], " \t"), i+1
		switch {
		case strings.HasPrefix(line, "### "):
			flush()
			f := strings.Split(strings.TrimPrefix(line, "### "), " · ")
			if len(f) != 5 {
				add(n, finding.Error, "Idea heading does not match the format.", "Write it as: "+format)
				continue
			}
			idea := Idea{Line: n, Title: strings.TrimSpace(f[1]), Pillar: strings.ToLower(strings.TrimSpace(f[2])), Status: strings.TrimSpace(f[4])}
			id, ok := ideaID(f[0])
			if !ok {
				add(n, finding.Error, "Idea ID "+f[0]+" is not I followed by three digits.", "Continue the sequence: I001, I002, …")
				continue
			}
			idea.ID = id
			if d, ok := strings.CutPrefix(f[3], "added "); !ok {
				add(n, finding.Error, "Idea heading lacks its added date.", "Write it as: "+format)
			} else if t, err := time.Parse(time.DateOnly, d); err != nil {
				add(n, finding.Error, "Added date "+d+" is not YYYY-MM-DD.", "Use the day the idea was written down.")
			} else {
				idea.Added = t
			}
			if !contains(Statuses, idea.Status) {
				add(n, finding.Error, "Status "+idea.Status+" is not one of "+strings.Join(Statuses, ", ")+".", "focus is an owner request and goes first; planned is next in line.")
			}
			if len(known) > 0 && !known[idea.Pillar] {
				add(n, finding.Warning, "Pillar "+idea.Pillar+" is not in the concept.", "Use one of: "+strings.Join(pillars, ", ")+"; or add the pillar to concept.md with a changelog entry.")
			}
			if idea.Title == "" {
				add(n, finding.Error, "Idea has no title.", "Name the idea in a few words.")
			}
			cur = &idea
		case cur != nil && strings.HasPrefix(line, "Sources: "):
			cur.Sources = sourceRE.FindAllString(line, -1)
		case cur != nil && strings.TrimSpace(line) != "":
			angle = append(angle, strings.TrimSpace(line))
		case cur == nil && strings.TrimSpace(line) != "":
			add(n, finding.Error, "Text outside an idea.", "Every line belongs under an idea heading.")
		}
	}
	flush()
	for k := 1; k < len(ideas); k++ {
		if ideas[k].ID <= ideas[k-1].ID {
			add(ideas[k].Line, finding.Error, fmt.Sprintf("%s comes after %s.", ideas[k].Code(), ideas[k-1].Code()), "Number ideas in order of writing; never reuse a number.")
		}
	}
	return ideas, fs
}

func ideaID(s string) (int, bool) {
	d, ok := strings.CutPrefix(strings.TrimSpace(s), "I")
	if !ok || len(d) < 3 {
		return 0, false
	}
	n := 0
	for _, c := range d {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, n > 0
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// Learning is one entry of learnings.md.
type Learning struct {
	Line  int
	Slug  string
	Sent  time.Time
	Notes []string
}

// ParseLearnings reads learnings.md:
//
//	### <slug> · sent YYYY-MM-DD · recipients N · clicks N · replies N · unsubscribes N · complaints N
//	- Keep: …
//	- Change: …
func ParseLearnings(text string) ([]Learning, []finding.Finding) {
	var out []Learning
	var fs []finding.Finding
	add := func(n int, sev, msg, fix string) {
		fs = append(fs, finding.Finding{Rule: RuleLearningsFormat, Severity: sev, File: LearningsFile, Line: n, Message: msg, Fix: fix})
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if !strings.HasPrefix(lines[0], "# Learnings: ") {
		add(1, finding.Error, "The file must start with \"# Learnings: <newsletter>\".", "Restore the title line.")
	}
	for i := 1; i < len(lines); i++ {
		line, n := strings.TrimRight(lines[i], " \t"), i+1
		switch {
		case strings.HasPrefix(line, "### "):
			f := strings.Split(strings.TrimPrefix(line, "### "), " · ")
			l := Learning{Line: n, Slug: strings.TrimSpace(f[0])}
			if len(f) < 2 {
				add(n, finding.Error, "Learning heading lacks its sent date and numbers.", "Paste the line lfr issue show <slug> --learn prints.")
			} else if d, ok := strings.CutPrefix(f[1], "sent "); !ok {
				add(n, finding.Error, "Learning heading lacks its sent date.", "Paste the line lfr issue show <slug> --learn prints.")
			} else if t, err := time.Parse(time.DateOnly, d); err == nil {
				l.Sent = t
			}
			out = append(out, l)
		case strings.HasPrefix(line, "- ") && len(out) > 0:
			out[len(out)-1].Notes = append(out[len(out)-1].Notes, strings.TrimPrefix(line, "- "))
		case strings.TrimSpace(line) != "" && len(out) > 0:
			out[len(out)-1].Notes = append(out[len(out)-1].Notes, strings.TrimSpace(line))
		case strings.TrimSpace(line) != "":
			add(n, finding.Error, "Text before the first learning.", "Every note belongs under an issue's heading.")
		}
	}
	for _, l := range out {
		keep, change := false, false
		for _, note := range l.Notes {
			keep = keep || strings.HasPrefix(note, "Keep: ")
			change = change || strings.HasPrefix(note, "Change: ")
		}
		if !keep || !change {
			add(l.Line, finding.Warning, "Learning for "+l.Slug+" lacks a Keep: or Change: line.", "Write what to keep doing and what to change in the next issue, citing the numbers or replies.")
		}
	}
	return out, fs
}
