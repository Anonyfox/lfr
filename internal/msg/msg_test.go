package msg

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestComposeParseRoundTrip(t *testing.T) {
	d := Draft{
		From:       Address{Name: "Jö Sender", Email: "jo@example.com"},
		To:         []Address{{Email: "ann@example.org"}},
		Cc:         []Address{{Name: "Bo", Email: "bo@example.org"}},
		Subject:    "Grüße & news",
		Date:       time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC),
		InReplyTo:  "abc@x",
		References: []string{"root@x", "abc@x"},
		Header:     [][2]string{{"List-Unsubscribe", "<https://example.com/u/t>"}},
		Text:       "Hello\nworld",
		HTML:       "<p>Hello <b>world</b></p>",
		Files:      []File{{Name: "a.txt", Data: []byte("attached")}},
	}
	raw, err := Compose(d)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ReplaceAll(raw, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("bare LF in composed message")
	}
	p := Parse(raw)
	if p.From.Email != "jo@example.com" || p.From.Name != "Jö Sender" {
		t.Errorf("from %+v", p.From)
	}
	if p.Subject != "Grüße & news" || len(p.To) != 1 || len(p.Cc) != 1 {
		t.Errorf("header %+v", p)
	}
	if p.InReplyTo != "abc@x" || len(p.References) != 2 || p.MessageID == "" {
		t.Errorf("ids %q %v %q", p.InReplyTo, p.References, p.MessageID)
	}
	if strings.TrimSpace(p.Text) != "Hello\nworld" || !strings.Contains(p.HTML, "<b>world</b>") {
		t.Errorf("bodies %q %q", p.Text, p.HTML)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Name != "a.txt" {
		t.Fatalf("attachments %+v", p.Attachments)
	}
	_, data, ok := AttachmentData(raw, 0)
	if !ok || string(data) != "attached" {
		t.Fatalf("attachment data %q", data)
	}
	if p.Header.Get("List-Unsubscribe") != "<https://example.com/u/t>" {
		t.Errorf("extra header %q", p.Header.Get("List-Unsubscribe"))
	}
}

func TestParseMalformed(t *testing.T) {
	cases := []string{
		"",
		"just some text without header",
		"From: broken <<<\r\nSubject: =?bogus?Q?x?=\r\n\r\nbody",
		"Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain; charset=iso-8859-1\r\n\r\nGr\xfc\xdfe\r\n--x--",
		"Content-Type: multipart/mixed\r\n\r\nno boundary",
	}
	for _, c := range cases {
		p := Parse([]byte(c))
		if p == nil {
			t.Fatalf("nil for %q", c)
		}
	}
	p := Parse([]byte(cases[3]))
	if !strings.Contains(p.Text, "Grüße") {
		t.Errorf("latin1 text = %q", p.Text)
	}
	p = Parse([]byte(cases[2]))
	if p.From.Email != "" && !strings.Contains(p.From.Email, "@") {
		t.Errorf("from %q", p.From.Email)
	}
}

func TestHTMLToText(t *testing.T) {
	in := `<html><head><style>p{}</style></head><body>
<div style="display:none">preheader</div>
<h1>Title</h1><p>Read <a href="https://x.example/a">the post</a> now.</p>
<ul><li>one</li><li>two</li></ul><p>Line<br>break</p><script>x()</script></body></html>`
	got := HTMLToText(in)
	want := "Title\n\nRead the post (https://x.example/a) now.\n\n- one\n- two\n\nLine\nbreak"
	if got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestRewrite(t *testing.T) {
	raw := []byte("From: a@b.c\r\nDKIM-Signature: v=1;\r\n b=xyz\r\nSubject: hi\r\n\r\nbody\r\n")
	out, err := Rewrite(raw, [][2]string{{"From", "x@y.z"}}, []string{"DKIM-Signature"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "DKIM") || strings.Contains(s, "a@b.c") || !strings.Contains(s, "From: x@y.z\r\n") || !strings.HasSuffix(s, "\r\n\r\nbody\r\n") || !strings.Contains(s, "Subject: hi") {
		t.Errorf("rewrite:\n%s", s)
	}
}

func TestSnippet(t *testing.T) {
	if Snippet("a  b\n c", 10) != "a b c" || Snippet("äöüäöü", 3) != "äöü…" {
		t.Fatal(Snippet("äöüäöü", 3))
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("From: a@b.c\r\nSubject: x\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n--b\r\nContent-Type: text/html\r\n\r\n<p>x</p>\r\n--b--\r\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		p := Parse(raw)
		_ = Snippet(p.Text, 100)
		for i := range p.Attachments {
			AttachmentData(raw, i)
		}
	})
}

func FuzzHTMLToText(f *testing.F) {
	f.Add("<p>a<a href=x>b</a></p><ul><li>c")
	f.Fuzz(func(t *testing.T, s string) { HTMLToText(s) })
}
