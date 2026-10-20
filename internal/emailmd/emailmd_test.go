package emailmd

import (
	"html/template"
	"strings"
	"testing"
)

const sample = `# Starter rescue

Your starter smells of **acetone**? It is hungry, not dead. Read [the guide](https://example.com/guide).

![A jar of starter](https://example.com/jar.jpg)

[Get the course →](https://example.com/course)

- feed twice a day
- keep it warm

> Patience beats flour.

| Day | Feeds |
|-----|-------|
| 1   | 2     |

` + "```\nflour: 50g\n```\n"

func TestRender(t *testing.T) {
	d, err := Render(sample)
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Starter rescue" || !strings.HasPrefix(d.Summary, "Your starter smells of acetone?") {
		t.Fatalf("title %q summary %q", d.Title, d.Summary)
	}
	if len(d.Links) != 2 || d.Links[1] != "https://example.com/course" {
		t.Fatalf("links %v", d.Links)
	}
}

func TestInline(t *testing.T) {
	d, _ := Render(sample)
	out := Inline(d.HTML, DefaultTheme)
	for _, want := range []string{
		`<h1 style="margin:0 0 16px;font-size:28px`,
		`<a href="https://example.com/guide" style="color:#0f766e`,
		`class="lfr-button"`, `Get the course →</a>`,
		`<blockquote style=`, `<td style="border:1px solid`,
		`<img src="https://example.com/jar.jpg" alt="A jar of starter" style="display:block`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if strings.Count(out, "lfr-button") != 1 {
		t.Error("only the lone arrow link is a button")
	}
}

func TestRewriteLinks(t *testing.T) {
	d, _ := Render(sample)
	out := RewriteLinks(d.HTML, func(i int, h string) string { return "https://t.example/c/" + string(rune('0'+i)) })
	if !strings.Contains(out, `href="https://t.example/c/0"`) || !strings.Contains(out, `href="https://t.example/c/1"`) || strings.Contains(out, "example.com/guide") {
		t.Fatal(out)
	}
}

func TestText(t *testing.T) {
	txt := Text(sample)
	for _, want := range []string{"Starter rescue", "acetone?", "the guide (https://example.com/guide)", "[A jar of starter]", "Get the course (https://example.com/course)", "flour: 50g"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "**") || strings.Contains(txt, "# ") {
		t.Errorf("markdown left in\n%s", txt)
	}
}

func TestEmailLayout(t *testing.T) {
	out, err := Email(Layout{Title: "Hi <there>", Preheader: "Short preview", Brand: "Bakers", Body: template.HTML("<p>x</p>"), FooterHTML: "<a href=\"https://u\">Unsubscribe</a>", Pixel: "https://t/o/x.gif"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title>Hi &lt;there&gt;</title>", "Short preview", "<p>x</p>", "Unsubscribe</a>", `src="https://t/o/x.gif"`, "prefers-color-scheme:dark", "font-family:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(out, "ZgotmplZ") {
		t.Fatal("template refused a value:\n" + out)
	}
}

func FuzzRender(f *testing.F) {
	f.Add(sample)
	f.Fuzz(func(t *testing.T, s string) {
		d, err := Render(s)
		if err != nil {
			return
		}
		Inline(d.HTML, DefaultTheme)
		RewriteLinks(d.HTML, func(int, string) string { return "x" })
		Text(s)
	})
}

func TestMarkButtons(t *testing.T) {
	d, _ := Render(sample)
	out := MarkButtons(d.HTML)
	if strings.Count(out, `class="button"`) != 1 || !strings.Contains(out, `<p class="cta">`) {
		t.Fatal(out)
	}
}
