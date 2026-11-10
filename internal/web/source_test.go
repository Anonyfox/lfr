package web

import (
	"strings"
	"testing"
)

func FuzzSignupSource(f *testing.F) {
	for _, s := range []string{"", "campaign-1_a", "<script>Ü", strings.Repeat("a", 40)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got := signupSource(raw)
		if len(got) > 32 || signupSource(got) != got {
			t.Fatalf("invalid source %q", got)
		}
		for _, c := range got {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				t.Fatalf("invalid character %q", c)
			}
		}
	})
}
