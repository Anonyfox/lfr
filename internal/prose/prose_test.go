package prose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rules(t *testing.T, text, lang string) map[string]int {
	t.Helper()
	got := map[string]int{}
	for _, f := range Check(Text{File: "x.md", Markdown: text, Language: lang}) {
		got[f.Rule]++
	}
	return got
}

func TestHabits(t *testing.T) {
	text := `# Why sourdough fails

Imagine a world where every loaf rises.

It isn't about the flour. It's about time. It's not the oven, it's the starter. Baking isn't magic. It's chemistry. This isn't a recipe. It's a rescue.

In this issue, we'll explore why starters fail and how to fix them, delve into the science, and unlock the power of fermentation.

Studies show that most home bakers give up within a month.

- **Hydration:** how wet the dough is.
- **Temperature:** how warm the room is.
- **Time:** how long it rests.

Moreover, the crumb matters. Furthermore, the crust matters. Additionally, the color matters.

TODO: add the recipe.

In short, baking is a journey, not a destination. Keep going, you've got this!
`
	got := rules(t, text, "en")
	for _, r := range []string{RuleCorrective, RuleSignpost, RuleTells, RuleSources, RuleLabels, RuleConnectors, RulePlaceholder, RuleEnding, RuleOpening} {
		if got[r] == 0 {
			t.Errorf("%s not found; got %v", r, got)
		}
	}
	// An issue is short; two hits are already a habit.
	short := "# The last hop\n\nDeliverability isn't just about servers. It's about trust. It's not the code, it's the reputation.\n\nMoreover, it matters. Furthermore, it lasts.\n"
	for _, r := range []string{RuleCorrective, RuleConnectors} {
		if rules(t, short, "en")[r] == 0 {
			t.Errorf("%s not found in a short issue", r)
		}
	}
	plain := `# Feed your starter twice a day

Ana's starter smelled of nail polish on Monday. She had fed it once in four days, at 18 °C in a cold kitchen.

We moved it next to the radiator and fed it 1:5:5 at 8 and 20 o'clock. By Wednesday it doubled in six hours, and the smell was gone.

If yours smells like that, it is hungry, not dead. Reply with a photo of your jar and I'll tell you what I see.
`
	if got := rules(t, plain, "en"); len(got) != 0 {
		t.Errorf("plain text flagged: %v", got)
	}
}

func TestDetectorOnFixtures(t *testing.T) {
	read := func(glob string) []string {
		files, _ := filepath.Glob(filepath.Join("..", "aitext", "testdata", glob))
		var out []string
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, string(b))
		}
		return out
	}
	flagged := 0
	gen := read("generated/opus-0*.md")
	for _, g := range gen {
		if rules(t, g, "en")[RuleMachine] > 0 {
			flagged++
		}
	}
	if flagged == 0 {
		t.Errorf("no generated fixture flagged as machine-written (%d tried)", len(gen))
	}
	for _, h := range read("human/en-gutenberg-*.txt") {
		if n := rules(t, h, "en")[RuleMachine]; n > 0 {
			t.Errorf("human fixture flagged: %.60s", h)
		}
	}
}

func TestRepeat(t *testing.T) {
	old := "Feed your starter twice a day at a ratio of one part starter to five parts flour and water, and keep it warm."
	text := Text{File: "new.md", Language: "en", Markdown: "# New\n\nAs always: feed your starter twice a day at a ratio of one part starter to five parts flour and water, and keep it warm.\n",
		Earlier: []Earlier{{File: "old.md", Markdown: "# Old\n\n" + old + "\n"}}}
	found := false
	for _, f := range Check(text) {
		if f.Rule == RuleRepeat && strings.Contains(f.Message, "old.md") {
			found = true
		}
	}
	if !found {
		t.Fatal("repetition of an earlier issue not found")
	}
}

func FuzzCheck(f *testing.F) {
	f.Add("# x\n\nIt isn't X. It's Y.", "en")
	f.Add("- **a:** b\n> quote\n\n| t | a |", "de")
	f.Fuzz(func(t *testing.T, md, lang string) {
		Check(Text{Markdown: md, Language: lang})
	})
}

func BenchmarkCheck(b *testing.B) {
	files, _ := filepath.Glob(filepath.Join("..", "aitext", "testdata", "generated", "opus-0*.md"))
	src, _ := os.ReadFile(files[0])
	text := Text{Markdown: string(src), Language: "en"}
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		Check(text)
	}
}

func TestGermanHabits(t *testing.T) {
	text := `# Die Reise zu einem gesunden Rücken

In der heutigen schnelllebigen Welt ist Rückenschmerz allgegenwärtig. Es geht nicht nur um Schmerzen, es geht um Lebensqualität. Rückengesundheit ist nicht einfach ein Trend. Es ist eine Haltung.

Lass uns gemeinsam eintauchen und herausfinden, welche Faktoren eine entscheidende Rolle spielen.

Darüber hinaus zeigen Studien, dass Bewegung hilft. Zudem ist es wichtig zu beachten, dass jeder Körper anders ist. Außerdem solltest du auf deinen Körper hören.

Fazit: Ein gesunder Rücken ist eine Reise, kein Ziel. Du schaffst das!
`
	got := rules(t, text, "de")
	for _, r := range []string{RuleCorrective, RuleSignpost, RuleTells, RuleSources, RuleConnectors, RuleEnding, RuleOpening} {
		if got[r] == 0 {
			t.Errorf("%s not found; got %v", r, got)
		}
	}
	plain := `# Die Wand-Übung

Herr Müller kam am Dienstag mit brennendem unterem Rücken in die Praxis: neun Stunden Schreibtisch, seit März jeden Abend.

Stell dich mit dem Rücken an eine Wand, die Fersen eine Handbreit entfernt. Drück den unteren Rücken zehn Sekunden sanft gegen die Wand und lös die Spannung wieder. Zehnmal, morgens vor dem Kaffee.

Wo spürst du deinen Rücken am Abend am meisten? Antworte einfach auf diese Mail.
`
	if got := rules(t, plain, "de"); len(got) != 0 {
		t.Errorf("plain German text flagged: %v", got)
	}
}
