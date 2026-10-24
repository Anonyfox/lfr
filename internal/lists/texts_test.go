package lists

import (
	"context"
	"testing"
)

func TestTextOverrides(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	if l.Text("form.submit") != "Abonnieren" {
		t.Fatalf("built-in German: %q", l.Text("form.submit"))
	}
	if _, err := s.SetTexts(ctx, "bakers", map[string]string{"form.submit": "Jetzt abonnieren", "confirm.subject": "no placeholder"}); err == nil {
		t.Fatal("a value without the placeholder was accepted")
	}
	if l, _ = s.Get(ctx, "bakers"); l.Text("form.submit") != "Abonnieren" {
		t.Fatal("a rejected batch changed a text")
	}
	l, err := s.SetTexts(ctx, "bakers", map[string]string{"form.submit": "Jetzt abonnieren", "confirm.subject": "Bitte bestätigen Sie %s"})
	if err != nil {
		t.Fatal(err)
	}
	v := l.TextValues()
	if !v["form.submit"].Overridden || v["form.submit"].Value != "Jetzt abonnieren" || v["form.email"].Overridden {
		t.Fatalf("values %+v %+v", v["form.submit"], v["form.email"])
	}
	if l, err = s.SetTexts(ctx, "bakers", map[string]string{"form.submit": ""}); err != nil || l.Text("form.submit") != "Abonnieren" {
		t.Fatalf("reset: %v %q", err, l.Text("form.submit"))
	}
	if _, err := s.SetTexts(ctx, "nope", map[string]string{"form.submit": "x"}); err == nil {
		t.Fatal("unknown list")
	}
}
