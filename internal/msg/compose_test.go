package msg

import (
	"bytes"
	"strings"
	"testing"
)

// Long fields fold at whitespace; non-ASCII file names use RFC 2231.
func TestFoldAndFilenames(t *testing.T) {
	raw, err := Compose(Draft{From: Address{Email: "a@example.com"}, To: []Address{{Email: "b@example.org"}},
		Subject: strings.Repeat("Größere Brötchen backen über Nacht ", 6),
		Header:  [][2]string{{"List-Unsubscribe", "<https://news.example.com/u/" + strings.Repeat("x", 40) + ">, <mailto:unsub+" + strings.Repeat("y", 40) + "@example.com?subject=unsubscribe>"}},
		Files:   []File{{Name: "Prüfbericht März.pdf", Data: []byte("x")}}, Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	head := string(raw[:bytes.Index(raw, []byte("\r\n\r\n"))])
	for _, l := range strings.Split(head, "\r\n") {
		if len(l) > 90 {
			t.Errorf("unfolded %d: %s", len(l), l)
		}
	}
	p := Parse(raw)
	if p.Subject != strings.Repeat("Größere Brötchen backen über Nacht ", 6) || !strings.Contains(p.Header.Get("List-Unsubscribe"), "subject=unsubscribe") {
		t.Fatalf("folding changed values: %q", p.Subject)
	}
	if !strings.Contains(string(raw), "filename*=utf-8''Pr%C3%BCfbericht") || len(p.Attachments) != 1 || p.Attachments[0].Name != "Prüfbericht März.pdf" {
		t.Fatalf("filename: %+v", p.Attachments)
	}
}

// Folding never changes what a header says.
func FuzzFold(f *testing.F) {
	f.Add("Subject: " + strings.Repeat("word ", 40) + "\r\nTo: a@b.c\r\n\r\nbody\r\n")
	f.Add("X: " + strings.Repeat("x", 200) + "\r\n\r\n")
	f.Fuzz(func(t *testing.T, raw string) {
		if !strings.Contains(raw, "\r\n\r\n") || strings.ContainsAny(strings.ReplaceAll(raw, "\r\n", ""), "\r\n") {
			return
		}
		folded := Fold([]byte(raw))
		unfold := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n ", " ") }
		if unfold(folded) != unfold([]byte(raw)) {
			t.Fatalf("folding changed content:\n%q\n%q", raw, folded)
		}
	})
}
