package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/token"
)

func TestSignupGuardsDoNotCreateMembers(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		age        time.Duration
		status     int
		paused     bool
		signupErr  error
	}{
		{name: "malformed JSON", body: `{`, status: 400},
		{name: "honeypot", body: `{"email":"a@reader.org","website":"spam"}`, status: 200},
		{name: "invalid timestamp", body: `{"email":"a@reader.org","Ts":"forged"}`, status: 200},
		{name: "too quick", age: time.Second, status: 200},
		{name: "expired form", age: 8 * 24 * time.Hour, status: 200},
		{name: "paused", body: `{"email":"a@reader.org"}`, status: 503, paused: true},
		{name: "confirmation limit", body: `{"email":"a@reader.org"}`, status: 429, signupErr: ErrLimited},
		{name: "storage failure", body: `{"email":"a@reader.org"}`, status: 500, signupErr: errors.New("storage unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setup(t)
			ctx := context.Background()
			if tc.paused {
				if _, err := e.srv.Lists.Pause(ctx, e.l.Slug, "review"); err != nil {
					t.Fatal(err)
				}
			}
			if tc.signupErr != nil {
				e.srv.Signup = func(context.Context, *lists.List, string, lists.Signup) error { return tc.signupErr }
			}
			body := tc.body
			if tc.age != 0 {
				body = `{"email":"a@reader.org","Ts":"` + e.srv.Tokens.Make(token.Form, e.now.Add(-tc.age).Unix()) + `"}`
			}
			r := e.do("POST", "https://news.example.com/subscribe", body, "Content-Type", "application/json", "Accept", "application/json")
			if r.Code != tc.status {
				t.Fatalf("%d %s", r.Code, r.Body.String())
			}
			members, err := e.srv.Lists.Members(ctx, e.l.Slug, lists.Query{})
			if err != nil || len(members) != 0 {
				t.Fatalf("guard created members: %+v %v", members, err)
			}
		})
	}
}

func TestWebhookGuardsRejectBeforeApplyingFeedback(t *testing.T) {
	e := setup(t)
	path := "https://mail.example.com" + e.srv.HookPath("generic")
	for _, tc := range []struct {
		name, method, target, body string
		status                     int
	}{
		{"wrong method", "GET", path, `{}`, 404},
		{"forged token", "POST", "https://mail.example.com/hooks/forged/generic", `{}`, 404},
		{"unknown provider", "POST", strings.TrimSuffix(path, "generic") + "other", `{}`, 404},
		{"oversized", "POST", path, strings.Repeat("x", (1<<20)+1), http.StatusRequestEntityTooLarge},
		{"bad payload", "POST", path, `{`, 400},
		{"unsigned SES", "POST", strings.TrimSuffix(path, "generic") + "ses", `{}`, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := e.do(tc.method, tc.target, tc.body, "Content-Type", "application/json")
			if r.Code != tc.status {
				t.Fatalf("%d %s", r.Code, r.Body.String())
			}
			if len(e.feedback) != 0 {
				t.Fatal("invalid webhook applied feedback")
			}
		})
	}
}
