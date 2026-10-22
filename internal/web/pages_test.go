package web

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"html/template"

	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/i18n"
)

// sampleIssue is an issue body with every element Markdown issues produce.
const sampleIssue = `# Your starter is hungry, not dead

Ana's starter smelled of nail polish on Monday. She had fed it once in four days, at 18 °C in a cold
kitchen. The smell is acetone: the yeast ran out of food and the bacteria took over.

![Ana's starter after two feeds, doubled in its jar](https://news.example.com/media/starter.png)

## Two feeds a day

We moved the jar next to the radiator and fed it 1:5:5 at 8 and 20 o'clock:

1. Discard all but 20 g.
2. Add 100 g flour and 100 g water at 26 °C.
3. Mark the level with a rubber band.

> By Wednesday it doubled in six hours, and the smell was gone.

| Day | Rise | Smell |
| --- | --- | --- |
| Monday | none | acetone |
| Wednesday | ×2 in 6 h | yoghurt |

Hydration is ` + "`water / flour`" + `, so 100 g into 100 g is 100 %.

[Get the feeding table →](https://news.example.com/issues/feeding-table)

If yours smells like that, reply with a photo of your jar and I'll tell you what I see.
`

// samplePages renders every state of every built-in template, in English and
// German, by file name.
func samplePages(t *testing.T) map[string][]byte {
	t.Helper()
	doc, err := emailmd.Render(sampleIssue)
	if err != nil {
		t.Fatal(err)
	}
	sites := &Sites{Dir: t.TempDir()}
	out := map[string][]byte{}
	render := func(name, tmpl string, p page) {
		set, err := sites.Page("bakers", tmpl)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := set.ExecuteTemplate(&b, "layout", p); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name+".html"] = b.Bytes()
	}
	for _, lang := range []string{"en", "de"} {
		name := "Bakers Weekly"
		dates, cadence := []string{"October 16, 2026", "October 9, 2026", "October 2, 2026"}, "Every Friday at 7, five minutes to read."
		if lang == "de" {
			dates, cadence = []string{"16.10.2026", "9.10.2026", "2.10.2026"}, "Jeden Freitag um 7, fünf Minuten zum Lesen."
		}
		base := func() page {
			return page{Lang: lang, ListName: name, HomeURL: "/", FeedURL: "/feed.xml", ArchiveURL: "/archive",
				ImprintURL: "https://example.com/imprint", PrivacyURL: "https://example.com/privacy",
				SignupURL: "/subscribe", FormToken: "1760000000.x", Description: "Sourdough that rises, every Friday.",
				Consent:  fmt.Sprintf(i18n.T(lang, "form.consent"), name),
				Cadence:  cadence,
				Headline: "Sourdough that rises, every Friday",
				Pitch:    "One problem from a reader's kitchen each week, solved with numbers you can repeat: flour, water, temperature, time.",
			}
		}
		issuesList := []issueItem{
			{URL: "/issues/acetone", Subject: "Why your starter smells like nail polish", Summary: "It is hungry, not dead, and two feeds fix it.", Date: dates[0]},
			{URL: "/issues/cold-kitchen", Subject: "Baking in an 18 °C kitchen", Summary: "Warm water, a smaller jar and an oven with the light on.", Date: dates[1]},
			{URL: "/issues/flat-loaf", Subject: "The loaf that spread like a pancake", Date: dates[2]},
		}

		p := base()
		p.Title, p.Home, p.Issues, p.More = name, true, issuesList, true
		render(lang+"-home", "home", p)

		p = base()
		p.Title, p.Home, p.Cadence = name, true, ""
		p.Consent = fmt.Sprintf(i18n.T(lang, "form.consent.tracked"), name)
		render(lang+"-home-empty", "home", p)

		p = base()
		p.Title, p.Issues = i18n.T(lang, "home.archive")+" · "+name, issuesList
		render(lang+"-archive", "archive", p)

		p = base()
		item := issuesList[0]
		item.Body = templateHTML(emailmd.MarkButtons(doc.HTML))
		p.Title, p.Issue = item.Subject+" · "+name, &item
		render(lang+"-issue", "issue", p)

		for _, m := range []struct{ key, action string }{
			{"check", ""}, {"confirm", "/confirm/x"}, {"confirmed", ""}, {"unsub", "/u/x"},
			{"unsubbed", ""}, {"invalid", ""}, {"error", ""}, {"limit", ""},
		} {
			p = base()
			p.Robots = "noindex"
			p.Heading = i18n.T(lang, "page."+m.key+".title")
			p.Body = i18n.T(lang, "page."+m.key+".body")
			if strings.Contains(p.Body, "%s") {
				p.Body = fmt.Sprintf(p.Body, name)
			}
			if m.action != "" {
				p.Action = m.action
				p.Button = i18n.T(lang, "confirm.action")
				if m.key == "unsub" {
					p.Button = i18n.T(lang, "page.unsub.action")
				}
			}
			p.Title = p.Heading + " · " + name
			render(lang+"-"+m.key, "message", p)
		}
	}
	return out
}

// TestWritePages writes every rendered state to $LFR_PAGES, for looking at
// them in a browser and checking them with lfm.
func TestWritePages(t *testing.T) {
	dir := os.Getenv("LFR_PAGES")
	if dir == "" {
		t.Skip("set LFR_PAGES to a directory")
	}
	for name, b := range samplePages(t) {
		b = bytes.ReplaceAll(b, []byte("https://news.example.com/media/"), []byte("media/"))
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func templateHTML(s string) template.HTML { return template.HTML(s) }
