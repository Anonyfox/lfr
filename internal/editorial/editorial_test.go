package editorial

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/prose"
)

func rules(fs []finding.Finding) map[string]string {
	m := map[string]string{}
	for _, f := range fs {
		m[f.Rule] = f.Severity
	}
	return m
}

const clean = `# The 2-day loaf

My starter smelled of nail polish last week. It was hungry, not dead: I fed it twice a day for two days and it doubled again by Sunday.

Here is the schedule I used, with the flour ratios and the water temperature. Keep the jar somewhere warm, around 24 degrees, and discard half before each feeding so the starter does not drown in its own acids.

![A jar of bubbly starter on a windowsill](https://example.com/jar.jpg)

The [full feeding schedule](https://example.com/schedule) has a printable table.

What does your starter smell like this week? Hit reply and tell me.
`

func TestCleanIssue(t *testing.T) {
	fs := Check(Issue{Subject: "The 2-day loaf", Preheader: "Hungry, not dead: how I revived my starter", Markdown: clean}, Brief{MaxSubject: 50, MinWords: 40, MaxWords: 300, Use: []string{"starter"}, Avoid: []string{"game-changer"}})
	if len(fs) != 0 {
		t.Fatalf("clean issue has findings: %+v", fs)
	}
}

func TestDirtyIssue(t *testing.T) {
	md := "Hi {first_name},\n\nIn today's fast-paced world this is a game-changer. Let's dive in. [Click here](https://x.example) and [read more](https://x.example/a). ACT NOW, 100% free!\n\n![](https://x.example/i.png)\n![](https://x.example/j.png)"
	got := rules(Check(Issue{Subject: "RE: 🚀 HUGE NEWS FOR EVERYONE!!!", Markdown: md}, Brief{Avoid: []string{"game-changer"}, Use: []string{"crumb"}}))
	want := map[string]string{
		SubjectDeceptive: finding.Error, SubjectCaps: finding.Warning, SubjectEmoji: finding.Warning, SubjectPunctuation: finding.Warning,
		PreheaderMissing: finding.Info, Personalization: finding.Error, prose.RuleTells: finding.Warning, LinksText: finding.Warning,
		ImagesAlt: finding.Warning, ImagesOnly: finding.Warning, CopyAvoid: finding.Warning, CopyUse: finding.Info, CopySpammy: finding.Warning, ContentTeaser: finding.Warning,
	}
	for r, sev := range want {
		if got[r] != sev {
			t.Errorf("%s = %q, want %q (all %v)", r, got[r], sev, got)
		}
	}
}

func TestLengthAndPreheader(t *testing.T) {
	long := strings.Repeat("word ", 1000)
	got := rules(Check(Issue{Subject: "Long one", Preheader: "Long one", Markdown: long + "\n\nThoughts? Reply."}, Brief{MinWords: 200, MaxWords: 500}))
	if got[LengthWords] != finding.Warning || got[PreheaderRepeat] != finding.Warning {
		t.Fatal(got)
	}
	got = rules(Check(Issue{Subject: "Short", Preheader: "x", Markdown: "Too short."}, Brief{MinWords: 200, MaxWords: 500}))
	if got[LengthWords] != finding.Warning {
		t.Fatal(got)
	}
}

func TestGerman(t *testing.T) {
	got := rules(Check(Issue{Subject: "Brot", Preheader: "Teig", Language: "de", Markdown: "In der heutigen schnelllebigen Welt tauchen wir ein. [Hier](https://x.example) klicken."}, Brief{}))
	if got[prose.RuleTells] == "" || got[LinksText] == "" {
		t.Fatal(got)
	}
}

func FuzzCheck(f *testing.F) {
	f.Add("subject", clean)
	f.Fuzz(func(t *testing.T, s, md string) {
		Check(Issue{Subject: s, Preheader: s, Markdown: md}, Brief{MaxSubject: 40, MinWords: 10, MaxWords: 100, Use: []string{"a"}, Avoid: []string{"b"}})
	})
}

func TestSubjectTells(t *testing.T) {
	got := Check(Issue{File: "x.md", Subject: "Unlock the Secrets of Perfect Rye Bread", Preheader: "Everything you need to know", Markdown: "# Rye\n\nBake it 15 minutes longer.\n"}, Brief{})
	n := 0
	for _, f := range got {
		if f.Rule == prose.RuleTells && strings.Contains(f.Message, "The subject") {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("subject tell not found: %+v", got)
	}
}
