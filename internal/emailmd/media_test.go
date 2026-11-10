package emailmd

import (
	"strings"
	"testing"
)

func TestLocalImageValidationAndRewriting(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 30))
	for _, tc := range []struct {
		name, path string
		data       []byte
		ok         bool
	}{
		{"relative", "images/garden.png", png, true},
		{"parent escape", "../garden.png", png, false},
		{"unclean", "images/../garden.png", png, false},
		{"absolute", "/garden.png", png, false},
		{"windows path", `images\garden.png`, png, false},
		{"remote", "https://example.org/garden.png", png, false},
		{"data URL", "data:image/png;base64,AAAA", png, false},
		{"oversized", "garden.png", make([]byte, MaxImage+1), false},
		{"svg", "garden.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ, err := CheckImage(tc.path, tc.data)
			if (err == nil) != tc.ok || tc.ok && typ != "image/png" {
				t.Fatalf("%s %v", typ, err)
			}
		})
	}
	source := "![A](images/a.png)\n\n![Remote](https://example.org/r.jpg)\n\n![Again](images/a.png)\n\n![B](b.gif)"
	local := LocalImages(source)
	if len(local) != 2 || local[0] != "images/a.png" || local[1] != "b.gif" {
		t.Fatalf("local images: %v", local)
	}
	doc, err := Render(source)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := RewriteImages(doc.HTML, func(src string) string {
		if Local(src) {
			return "https://media.example/" + src
		}
		return src
	})
	for _, want := range []string{`src="https://media.example/images/a.png"`, `src="https://example.org/r.jpg"`, `src="https://media.example/b.gif"`} {
		if !strings.Contains(rewritten, want) {
			t.Errorf("missing %s in %s", want, rewritten)
		}
	}
}

func TestFrontMatterAndPlainTextEdges(t *testing.T) {
	for _, tc := range []struct{ name, source, subject, body string }{
		{"BOM and CRLF", "\ufeff---\r\nSubject: 'A: title'\r\nignored\r\n---\r\nBody", "A: title", "Body"},
		{"unclosed header", "---\nsubject: Title", "", "---\nsubject: Title"},
		{"ordinary body", "Body", "", "Body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta, body := FrontMatter(tc.source)
			if meta["subject"] != tc.subject || body != tc.body {
				t.Fatalf("%v %q", meta, body)
			}
		})
	}
	text := Text("# hello\n\n[https://example.org](https://example.org)\n\n\n\n```\n**literal**\n```\n\n<b>Plain</b>")
	if text != "Hello\n\nhttps://example.org\n\n**literal**\n\nPlain" {
		t.Fatalf("plain text: %q", text)
	}
}
