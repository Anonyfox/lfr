package concept

import (
	"strings"
	"testing"
	"time"
)

const good = `# Concept: Bakers Weekly

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
Weekly fits the writer's baking routine. E101

## Length
Words: 300-700

## Subject
Max: 50
Example: "Why your starter smells like nail polish"
Example: "The 2-day loaf"
Example: "Flat bread, fixed"

## Voice
Warm, short sentences, first person.
Use: "crumb shot", "starter"
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
Open: Do we mention the bakery by name?
`

func ids() map[string]bool {
	m := map[string]bool{"E101": true}
	for _, id := range []string{"V001", "V002", "V003", "V004", "V005", "V006", "V007", "V008", "P001", "P002", "P003"} {
		m[id] = true
	}
	return m
}

func TestParseGood(t *testing.T) {
	c, fs := Parse(good, ids())
	for _, f := range fs {
		if f.Rule != RuleOpen {
			t.Errorf("%d %s: %s", f.Line, f.Rule, f.Message)
		}
	}
	if c.Cadence.Every != 7*24*time.Hour || c.Cadence.Day != time.Friday || c.Cadence.Hour != 7 || c.Cadence.Zone.String() != "Europe/Berlin" {
		t.Errorf("cadence %+v", c.Cadence)
	}
	if c.MinWords != 300 || c.MaxWords != 700 || c.MaxSubject != 50 || len(c.Examples) != 3 || len(c.Use) != 2 || len(c.Avoid) != 2 {
		t.Errorf("%+v", c)
	}
	if strings.Join(c.Pillars, ",") != "fixes,recipes" || c.Headline != "Better sourdough by Friday" || c.MaxUnsub != 0.5 || len(c.Open) != 1 {
		t.Errorf("%+v", c)
	}
	if c.Promise != "One recipe and one fix for your sourdough, every Friday morning." {
		t.Errorf("promise %q", c.Promise)
	}
}

func TestParseBad(t *testing.T) {
	bad := strings.NewReplacer("Every: weekly", "Every: sometimes", "Words: 300-700", "Words: lots", "V008", "V999", "## Growth\n", "## Pillars\n").Replace(good)
	_, fs := Parse(bad, ids())
	var msgs []string
	for _, f := range fs {
		msgs = append(msgs, f.Message)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"is not a cadence", "is not a range", "V999 is not in the research", "appears twice"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
}

func TestParseEvery(t *testing.T) {
	for in, want := range map[string]time.Duration{"daily": 24 * time.Hour, "2w": 14 * 24 * time.Hour, "3d": 72 * time.Hour, "monthly": 30 * 24 * time.Hour} {
		if got, ok := ParseEvery(in); !ok || got != want {
			t.Errorf("%s = %v", in, got)
		}
	}
	if _, ok := ParseEvery("0d"); ok {
		t.Error("0d accepted")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(good)
	f.Fuzz(func(t *testing.T, s string) { Parse(s, ids()) })
}

func TestCadenceNext(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	c := Cadence{Every: 7 * 24 * time.Hour, Day: time.Tuesday, HasDay: true, Hour: 7, Minute: 30, HasTime: true, Zone: berlin}
	mon := time.Date(2026, 10, 19, 12, 0, 0, 0, berlin) // a Monday
	if got := c.Next(time.Time{}, mon); !got.Equal(time.Date(2026, 10, 20, 7, 30, 0, 0, berlin)) {
		t.Fatal("first slot", got)
	}
	// Sent Tuesday 07:30, asked on Wednesday: next Tuesday.
	sent := time.Date(2026, 10, 20, 7, 30, 0, 0, berlin)
	if got := c.Next(sent, sent.Add(26*time.Hour)); !got.Equal(time.Date(2026, 10, 27, 7, 30, 0, 0, berlin)) {
		t.Fatal("next week", got)
	}
	// Sent late on Wednesday: the schedule stays on Tuesdays.
	late := time.Date(2026, 10, 21, 18, 0, 0, 0, berlin)
	if got := c.Next(late, late.Add(time.Hour)); got.Weekday() != time.Tuesday || !got.After(late) {
		t.Fatal("late issue moved the day", got)
	}
	daily := Cadence{Every: 24 * time.Hour, Hour: 6, HasTime: true}
	now := time.Date(2026, 10, 19, 7, 0, 0, 0, time.UTC)
	if got := daily.Next(now.Add(-time.Hour), now); !got.Equal(time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)) {
		t.Fatal("daily", got)
	}
}
