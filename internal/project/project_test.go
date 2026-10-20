package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func write(t *testing.T, dir, rel, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func voices() string {
	var b strings.Builder
	b.WriteString("# Voices: Bakers Weekly\n\n## Batch 2026-10-19 · from zero · queries: \"sourdough newsletter\" · unreachable: none\n")
	platforms := []string{"Hacker News", "Lemmy", "Mastodon", "Discourse"}
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "\n### V%03d · %s (baking) · reader · posted 2025-%02d · fetched 2026-10-19 · via fetch\n> Quote number %d about starters and weekend bakes.\n<https://example.org/v/%d>\n",
			i, platforms[i%4], 1+i%12, i, i)
	}
	return b.String()
}

func peers() string {
	var b strings.Builder
	b.WriteString("# Peers: Bakers Weekly\n\n## Batch 2026-10-19 · from zero\n")
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&b, `
### P%03d · Bread Letter %d · https://bread%d.example/archive · fetched 2026-10-19 · via fetch
- Cadence: weekly
- Length: 600 / 800 / 700
- Structure: note, recipe, links
- Subject: "Crumb shots %d"
- Opening: "Hi bakers,"
- Voice: first person
- Visual: light HTML
- Links: 3 to 5
- Signup: promise and sample
- Takeaway: a recipe each week
`, i, i, i, i)
	}
	b.WriteString("\n### Grid after this batch\n\n| Column | Value | Peers |\n| --- | --- | --- |\n")
	for _, col := range []string{"Cadence", "Length", "Structure", "Subject", "Opening", "Voice", "Visual", "Links", "Signup", "Takeaway"} {
		fmt.Fprintf(&b, "| %s | the same | 8 |\n", col)
	}
	b.WriteString("\n### Baseline\n\nWeekly, short, personal.\n\n### Gaps\n\n- Nobody fixes failed loaves.\n")
	return b.String()
}

const brief = `# Concept: Bakers Weekly

## Reader
Home bakers who keep a starter and bake on weekends. V001

## Promise
One recipe and one fix for your sourdough, every Friday morning.
Credible because the writer bakes daily. V002

## Voices
- They want fixes, not theory. V001
- They hate long intros before the recipe. V002
- They bake on weekends. V003
- They share crumb photos. V004
- They distrust gear ads. V005
- They read on phones in the kitchen. V006
- They reply with their own failures. V007
- They forward to baking friends. V008

## Axes
| Axis | Market | Readers | Chosen |
| --- | --- | --- | --- |
| Length | long essays P001 | short fixes V001 | short |
| Visual | photo heavy P002 | phone in flour V006 | plain text |
| Tone | expert P003 | friend V007 | friend |

## Format
1. Opening: one line about this week's bake
2. Fix: one problem and its fix
3. Recipe: one recipe

## Cadence
Every: weekly
Day: Friday
Time: 07:00
Zone: Europe/Berlin
Weekly fits the writer's baking routine. lfr:E101

## Length
Words: 40-700

## Subject
Max: 50
Example: "Why your starter smells like nail polish"
Example: "The 2-day loaf"
Example: "Flat bread, fixed"

## Voice
Warm, short sentences, first person.
Use: "crumb", "starter"
Avoid: "game-changer", "hack"

## Pillars
- fixes: what went wrong and how to save it
- recipes: one tested recipe

## Signup
Headline: Better sourdough by Friday
Pitch: One fix and one recipe in your inbox each Friday.
Frequency: Every Friday morning, five minutes to read

## Welcome
Say hi, link the three best fixes.

## Growth
Archive pages rank for "sourdough fix" searches.

## Guardrails
Unsubscribes: 0.5%
Complaints: 0.1%
`

const welcome = `---
subject: Welcome to Bakers Weekly
preheader: What comes on Fridays, and the three fixes readers like most
---
Hi, I'm Ana. Every Friday you get one fix for your starter and one recipe that works on a weekend.

Three fixes readers liked most: [the acetone smell](https://bakers.example/w/acetone), [flat loaves](https://bakers.example/w/flat) and [a sluggish starter](https://bakers.example/w/slow).

Reply and tell me what your last bake looked like; I read every answer.
`

const issue = `---
subject: Why your starter smells like nail polish
preheader: It is hungry, not dead, and two feeds fix it
idea: I001
---
Your starter smells of acetone because it ran out of food. The yeast is fine; it is hungry.

Feed it twice a day for two days, at a ratio of one part starter to five parts flour and water. The smell goes, the rise comes back.

This weekend's recipe is a plain country loaf that forgives a weak starter: [the recipe](https://bakers.example/recipes/country-loaf).

Reply with a crumb shot of your next bake; I answer every one.
`

func backlog(status string) string {
	return `# Backlog: Bakers Weekly

### I001 · Acetone smell · fixes · added 2026-10-19 · ` + status + `
Starters that smell of nail polish are hungry, not dead.
Sources: V001, V007

### I002 · The two-day loaf · recipes · added 2026-10-19 · idea
A loaf for people who only bake on weekends.
Sources: V003

### I003 · Flat loaves · fixes · added 2026-10-19 · idea
Shaping and proofing, the two reasons loaves spread.
Sources: V004

### I004 · Rye for beginners · recipes · added 2026-10-19 · planned
Rye behaves differently; one recipe that works.
Sources: V006, owner

### I005 · Crumb shots from readers · fixes · added 2026-10-20 · idea
Readers' failed loaves, diagnosed.
Sources: V007
`
}

func check(t *testing.T, dir string, remote *Remote) *Report {
	t.Helper()
	r, err := Check(dir, Options{Now: now, Builtin: map[string]bool{"lfr:E101": true}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func expect(t *testing.T, r *Report, phase, next string) {
	t.Helper()
	if r.Phase != phase || !strings.Contains(r.Next, next) {
		var fs []string
		for _, f := range r.Findings {
			fs = append(fs, f.Severity+" "+f.Rule+" "+f.Where()+" "+f.Message)
		}
		t.Fatalf("phase %s (%s), want %s (%s)\n%s", r.Phase, r.Next, phase, next, strings.Join(fs, "\n"))
	}
	if r.Skill != Skills[phase] {
		t.Fatalf("skill %s for %s", r.Skill, phase)
	}
}

func TestLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bakers")
	written, err := Init(dir, Config{SchemaVersion: 1, List: "bakers", Language: "en", Owner: "ana@example.com"}, "Bakers Weekly")
	if err != nil || len(written) < 6 {
		t.Fatal(written, err)
	}
	if _, err := Init(dir, Config{SchemaVersion: 1, List: "bakers", Language: "en"}, "x"); err == nil {
		t.Fatal("init into a project")
	}
	if found, ok := Find(filepath.Join(dir, "issues")); !ok || filepath.Base(found) != "bakers" {
		t.Fatal("find", found)
	}
	expect(t, check(t, dir, nil), PhaseResearch, "collect reader voices")

	write(t, dir, "research/voices.md", voices())
	expect(t, check(t, dir, nil), PhaseResearch, "analyze more peer newsletters: 0 of 8")
	write(t, dir, "research/peers.md", peers())
	expect(t, check(t, dir, nil), PhaseConcept, "write concept.md")

	write(t, dir, "concept.md", strings.Replace(brief, "lfr:E101", "lfr:E999", 1))
	expect(t, check(t, dir, nil), PhaseConcept, "lfr:E999 is not in the research")
	write(t, dir, "concept.md", brief+"Open: Do we name the bakery?\n")
	expect(t, check(t, dir, nil), PhaseConcept, "ask the owner: Do we name the bakery?")
	write(t, dir, "concept.md", brief)
	expect(t, check(t, dir, nil), PhaseLaunch, "write welcome.md")

	write(t, dir, "welcome.md", welcome)
	expect(t, check(t, dir, nil), PhasePlan, "0 open")
	write(t, dir, "backlog.md", backlog("planned"))
	r := check(t, dir, nil)
	expect(t, r, PhaseDraft, "from I001 (Acetone smell), due Fri 2026-10-23 07:00")

	write(t, dir, "issues/acetone.md", issue)
	r = check(t, dir, nil)
	expect(t, r, PhaseTest, "on the server: lfr issue put issues/acetone.md")
	if !has(r, RuleBacklog, "I001 has a draft") {
		t.Fatal("drafted status not demanded")
	}
	write(t, dir, "backlog.md", backlog("drafted"))
	if r := check(t, dir, nil); has(r, RuleBacklog, "I001") {
		t.Fatal("status still flagged")
	}

	// Rules from the brief reach the draft.
	write(t, dir, "issues/acetone.md", strings.Replace(issue, "The yeast is fine", "The yeast is a real game-changer", 1))
	if r := check(t, dir, nil); !has(r, "copy.avoid", "game-changer") {
		t.Fatal("Avoid: not applied")
	}
	write(t, dir, "issues/acetone.md", strings.Replace(issue, "idea: I001", "idea: I042", 1))
	expect(t, check(t, dir, nil), PhaseFix, "Idea I042 is not in the backlog")
	write(t, dir, "issues/acetone.md", issue)

	// With the server.
	expect(t, check(t, dir, &Remote{Error: "no server is running"}), PhaseTest, "on the server")
	expect(t, check(t, dir, &Remote{}), PhaseLaunch, "lfr list add bakers")
	list := &RemoteList{Slug: "bakers", Description: "One recipe and one fix for your sourdough, every Friday morning.", Headline: "Better sourdough by Friday", Pitch: "One fix and one recipe in your inbox each Friday.",
		Cadence: "Every Friday morning, five minutes to read", Language: "en"}
	expect(t, check(t, dir, &Remote{List: list}), PhaseLaunch, "lfr push")
	wd := ParseDraft("welcome.md", welcome)
	wel := RemoteIssue{Slug: "welcome", Subject: wd.Subject, Preheader: wd.Preheader, Markdown: wd.Markdown, Status: "draft", Welcome: true}
	expect(t, check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel}}), PhaseTest, "lfr issue put issues/acetone.md, then lfr issue test acetone ana@example.com")

	d := ParseDraft("issues/acetone.md", issue)
	rw := RemoteIssue{Slug: "acetone", Subject: d.Subject, Preheader: d.Preheader, Markdown: d.Markdown, Status: "draft"}
	expect(t, check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}}), PhaseTest, "lfr issue test acetone")
	rw.Tested = now.Add(-time.Hour)
	expect(t, check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}}), PhaseApprove, "lfr issue send acetone --at 2026-10-23T07:00:00+02:00")
	write(t, dir, "lfr.json", `{"schemaVersion": 1, "list": "bakers", "language": "en", "owner": "ana@example.com", "autonomy": "send"}`)
	expect(t, check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}}), PhaseSend, "lfr issue send acetone --at")
	rw.Status, rw.Scheduled = "scheduled", time.Date(2026, 10, 23, 5, 0, 0, 0, time.UTC)
	expect(t, check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}}), PhaseWait, "acetone goes out Fri 2026-10-23 07:00 CEST")

	// Sent three days ago: learn, then guardrails and the next draft.
	rw.Status, rw.Started = "sent", now.Add(-72*time.Hour)
	rw.Stats = &Numbers{Sent: 1000, Clicked: 120, Unsubscribed: 9, Complained: 2}
	r = check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}})
	expect(t, r, PhaseLearn, "lfr issue show acetone --learn")
	if !has(r, RuleGuardrail, "acetone drew 0.20% spam complaints") || !has(r, RuleGuardrail, "lost 0.90% to unsubscribes") {
		t.Fatal("guardrails not checked")
	}
	write(t, dir, "learnings.md", "# Learnings: Bakers Weekly\n\n"+rw.Stats.Line("acetone", rw.Started)+"\n- Keep: one fix per issue\n- Change: shorter recipe\n")
	write(t, dir, "backlog.md", backlog("sent"))
	r = check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}})
	if f := blocking(r.Findings, ""); f != nil && f.Rule == RuleGuardrail {
		t.Fatal("a guardrail breach still blocks after its learning was recorded")
	}
	rw.Stats.Complained = 0
	r = check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}})
	expect(t, r, PhaseDraft, "from I004 (Rye for beginners), due Fri 2026-10-23 07:00 CEST")
	if !r.Slot.Equal(time.Date(2026, 10, 23, 7, 0, 0, 0, r.Slot.Location())) {
		t.Fatal("slot", r.Slot)
	}

	// Overdue cadence.
	rw.Started = now.Add(-20 * 24 * time.Hour)
	if r := check(t, dir, &Remote{List: list, Issues: []RemoteIssue{wel, rw}}); !has(r, RuleCadence, "20 days ago") {
		t.Fatal("overdue cadence not flagged")
	}
}

func has(r *Report, rule, text string) bool {
	for _, f := range r.Findings {
		if f.Rule == rule && strings.Contains(f.Message, text) {
			return true
		}
	}
	return false
}

func TestDraftParsing(t *testing.T) {
	d := ParseDraft("issues/x.md", "# Heading subject\n\nBody.")
	if d.Slug != "x" || d.Subject != "Heading subject" || d.Idea != "" {
		t.Fatalf("%+v", d)
	}
	d = ParseDraft("issues/x.md", "---\nslug: Other\nidea: i003\n---\nBody")
	if d.Slug != "other" || d.Idea != "I003" {
		t.Fatalf("%+v", d)
	}
}

func FuzzProjectFiles(f *testing.F) {
	f.Add(backlog("idea"), "# Learnings: x\n\n### a · sent 2026-10-20 · recipients 1\n- Keep: x\n", issue)
	f.Add("### I · · ·", "### ·", "---\n---")
	f.Fuzz(func(t *testing.T, b, l, d string) {
		ParseBacklog(b, []string{"fixes"})
		ParseLearnings(l)
		ParseDraft("issues/x.md", d)
	})
}
