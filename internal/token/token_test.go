package token

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	s := New([]byte("0123456789abcdef0123456789abcdef"))
	tok := s.Make(Click, 123456, 3)
	ids, err := s.Parse(Click, tok, 2)
	if err != nil || ids[0] != 123456 || ids[1] != 3 {
		t.Fatalf("ids %v, %v", ids, err)
	}
	if strings.ContainsAny(tok, "+/=") || strings.ToLower(tok) != tok {
		t.Fatalf("token %q is not URL and local-part safe", tok)
	}
	if _, err := s.Parse(Click, strings.ToUpper(tok), 2); err != nil {
		t.Fatal("upper-cased token rejected:", err)
	}
	if _, err := s.Parse(Open, tok, 2); err == nil {
		t.Fatal("accepted wrong kind")
	}
	if _, err := s.Parse(Click, tok, 1); err == nil {
		t.Fatal("accepted wrong arity")
	}
	other := New([]byte("another secret, another secret!!"))
	if _, err := other.Parse(Click, tok, 2); err == nil {
		t.Fatal("accepted foreign signature")
	}
	b := []byte(tok)
	b[2] ^= 1
	if _, err := s.Parse(Click, string(b), 2); err == nil {
		t.Fatal("accepted tampered token")
	}
}

func FuzzParse(f *testing.F) {
	s := New([]byte("k"))
	f.Add(s.Make(Unsub, 1, 2))
	f.Add("")
	f.Add("dQ")
	f.Fuzz(func(t *testing.T, tok string) {
		ids, err := s.Parse(Unsub, tok, 2)
		if err == nil && s.Make(Unsub, ids...) != strings.ToLower(tok) {
			// Only canonical encodings may verify.
			t.Fatalf("non-canonical token %q verified as %v", tok, ids)
		}
	})
}
