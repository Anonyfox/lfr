package addr

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]string{
		"Jo@Example.COM":               "jo@example.com",
		"Jo Doe <jo+x@example.com>":    "jo+x@example.com",
		"\"weird name\" <a@b.example>": "a@b.example",
		"x@bücher.de":                  "x@xn--bcher-kva.de",
	}
	for in, want := range cases {
		got, err := Email(in)
		if err != nil || got != want {
			t.Errorf("Email(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "nope", "a@", "@b.com", "a@localhost", "a@b..com", "a@b.com\r\nBcc: x@y.z", "a@-b.com"} {
		if got, err := Email(bad); err == nil {
			t.Errorf("Email(%q) = %q, want error", bad, got)
		}
	}
}

func TestLocalAndBase(t *testing.T) {
	if l, err := Local("News.Letter"); err != nil || l != "news.letter" {
		t.Fatal(l, err)
	}
	for _, bad := range []string{"", ".a", "a.", "a..b", "a b", "ä"} {
		if _, err := Local(bad); err == nil {
			t.Errorf("Local(%q) accepted", bad)
		}
	}
	if Base("jo+news") != "jo" || Base("+x") != "+x" || Base("jo") != "jo" {
		t.Fatal("Base")
	}
}

func FuzzParse(f *testing.F) {
	f.Add("Jo <jo@example.com>")
	f.Fuzz(func(t *testing.T, s string) {
		a, err := Parse(s)
		if err != nil {
			return
		}
		again, err := Parse(a.String())
		if err != nil || again != a {
			t.Fatalf("%q → %q does not reparse: %v %v", s, a, again, err)
		}
	})
}
