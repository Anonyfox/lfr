package lists

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInvalidListEditsPreserveIdentityAndConsent(t *testing.T) {
	s, l, _ := setup(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		in   Input
	}{
		{"empty name", Input{Name: ptr(" ")}},
		{"invalid sender", Input{From: ptr("bad")}},
		{"another sender domain", Input{From: ptr("a@other.org")}},
		{"invalid reply address", Input{ReplyTo: ptr("bad")}},
		{"invalid host", Input{Host: ptr("bad host")}},
		{"short language", Input{Language: ptr("e")}},
		{"long language", Input{Language: ptr("very-long-language")}},
		{"imprint URL", Input{ImprintURL: ptr("javascript:alert(1)")}},
		{"privacy URL", Input{PrivacyURL: ptr("/privacy")}},
		{"long cadence", Input{Cadence: ptr(strings.Repeat("x", 81))}},
		{"negative gap", Input{MinGap: ptr(-time.Second)}},
		{"excessive gap", Input{MinGap: ptr(61 * 24 * time.Hour)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Update(ctx, l.Slug, tc.in); err == nil {
				t.Fatal("invalid edit accepted")
			}
			got, err := s.Get(ctx, l.Slug)
			if err != nil || got.From != l.From || got.Name != l.Name || got.ConsentText() != l.ConsentText() {
				t.Fatalf("invalid edit changed list: %+v %v", got, err)
			}
		})
	}
	got, err := s.Update(ctx, l.Slug, Input{From: ptr("edition@example.com"), FromName: ptr(" Editor "), ReplyTo: ptr("REPLIES@example.com"), Language: ptr(" EN "), PrivacyURL: ptr("https://example.com/privacy"), ImprintURL: ptr("https://example.com/imprint"), Address: ptr("Weg 1, Berlin"), Consent: ptr("Agreed on the paper form"), Description: ptr("Weekly recipes"), Cadence: ptr("Every Friday"), Headline: ptr("Bake better"), Pitch: ptr("One recipe per week"), MinGap: ptr(time.Hour)})
	if err != nil || got.From != "edition@example.com" || got.ReplyTo != "replies@example.com" || got.FromName != "Editor" || got.Language != "en" || got.ConsentText() != "Agreed on the paper form" || got.MinGap.Duration() != time.Hour {
		t.Fatalf("valid edit: %+v %v", got, err)
	}
	if _, err := s.Update(ctx, l.Slug, Input{ReplyTo: ptr(""), PrivacyURL: ptr(""), ImprintURL: ptr("")}); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationBudgetAndUnblockingDoNotRestoreConsent(t *testing.T) {
	s, l, c := setup(t)
	ctx := context.Background()
	m, send, err := s.Subscribe(ctx, l, "a@reader.org", Signup{Source: "form"})
	if err != nil || !send {
		t.Fatalf("signup: %+v %v %v", m, send, err)
	}
	if n := s.ConfirmsLastHour(ctx, l.ID); n != 1 {
		t.Fatalf("signup did not reserve confirmation budget: %d", n)
	}
	s.MarkConfirmSent(ctx, m.ID)
	if n := s.ConfirmsLastHour(ctx, l.ID); n != 1 {
		t.Fatalf("sent confirmation missing from budget: %d", n)
	}
	c.Add(time.Hour + time.Second)
	if n := s.ConfirmsLastHour(ctx, l.ID); n != 0 {
		t.Fatalf("expired confirmation consumes budget: %d", n)
	}
	if _, err := s.Confirm(ctx, m.ID, "198.51.100.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unsubscribe(ctx, m.ID, 0, "reader request"); err != nil {
		t.Fatal(err)
	}
	if err := s.Suppress(ctx, m.Email, "operator block"); err != nil {
		t.Fatal(err)
	}
	if err := s.Unsuppress(ctx, m.Email); err != nil {
		t.Fatal(err)
	}
	got, err := s.Member(ctx, m.ID)
	if err != nil || got.Suppressed != "" || got.Status != "unsubscribed" {
		t.Fatalf("unblocking restored consent: %+v %v", got, err)
	}
	if err := s.Unsuppress(ctx, "invalid"); err == nil {
		t.Fatal("invalid address accepted")
	}
}

func TestRemovingOneListKeepsOtherMembershipsAndGlobalSuppression(t *testing.T) {
	s, l, _ := setup(t)
	ctx := context.Background()
	other, err := s.Create(ctx, "other", Input{Name: ptr("Other"), From: ptr("other@example.com")})
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range []*List{l, other} {
		if _, _, err := s.Add(ctx, list, "a@reader.org", "", true, Signup{Consent: "Paper form"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Suppress(ctx, "a@reader.org", "complaint"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, l.Slug); err != nil {
		t.Fatal(err)
	}
	all, err := s.All(ctx)
	if err != nil || len(all) != 1 || all[0].ID != other.ID {
		t.Fatalf("list removal affected other lists: %+v %v", all, err)
	}
	m, err := s.Find(ctx, other.Slug, "a@reader.org")
	if err != nil || m.Suppressed != "complaint" || m.Status != "confirmed" {
		t.Fatalf("global suppression or other membership lost: %+v %v", m, err)
	}
	if err := s.Remove(ctx, l.Slug); err == nil {
		t.Fatal("missing list removed twice")
	}
}
