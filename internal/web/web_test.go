package web

import (
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/feedback"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/store"
	"github.com/Anonyfox/lfr/internal/token"
)

type env struct {
	srv      *Server
	l        *lists.List
	now      time.Time
	signups  []string
	feedback []feedback.Event
	dir      string
}

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), dir: filepath.Join(dir, "sites")}
	now := func() time.Time { return e.now }
	ds := &domains.Service{DB: db, Now: now}
	ds.Add(ctx, "example.com")
	bus := &events.Bus{DB: db, Now: now}
	ls := &lists.Service{DB: db, Events: bus, Now: now}
	e.l, _ = ls.Create(ctx, "bakers", lists.Input{Name: ptr("Bakers Weekly"), From: ptr("news@example.com"), Host: ptr("news.example.com"), Tracking: ptr(true), Description: ptr("Bread, weekly.")})
	ls.Create(ctx, "plain", lists.Input{Name: ptr("Plain"), From: ptr("plain@example.com")})
	q := &outbound.Queue{DB: db, Domains: ds, Events: bus, Now: now, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ws := &issues.Service{DB: db, Lists: ls, Events: bus, Queue: q, Now: now}
	tokens := token.New([]byte("k"))
	r := &issues.Renderer{Tokens: tokens, Now: now,
		ListURL: func(l *lists.List) string {
			if l.Host != "" {
				return "https://" + l.Host
			}
			return "https://mail.example.com/lists/" + l.Slug
		},
		BaseURL: func(l *lists.List) string { return "https://mail.example.com" }}
	e.srv = &Server{DB: db, Lists: ls, Issues: ws, Renderer: r, Tokens: tokens, Events: bus, Config: config.Config{DB: db},
		Sites: &Sites{Dir: e.dir}, Now: now, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		MailHost: func(context.Context) string { return "mail.example.com" },
		MTASTS: func(_ context.Context, d string) (string, bool) {
			return domains.MTASTSPolicy(domains.Instance{Hostname: "mail.example.com"}), d == "example.com"
		},
		Signup: func(ctx context.Context, l *lists.List, email string, s lists.Signup) error {
			e.signups = append(e.signups, email)
			_, _, err := ls.Subscribe(ctx, l, email, s)
			return err
		},
		Feedback: func(_ context.Context, evs []feedback.Event) { e.feedback = append(e.feedback, evs...) },
	}
	return e
}

func (e *env) do(method, target string, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	req.RemoteAddr = "198.51.100.7:4242"
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func TestSitePages(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	w, _ := e.srv.Issues.Create(ctx, "bakers", issues.Draft{Markdown: "# First bake\n\nCrust matters. [More](https://example.com)."})
	draft, _ := e.srv.Issues.Create(ctx, "bakers", issues.Draft{Markdown: "# Secret draft\n\nNot yet."})
	e.srv.DB.W.Exec("UPDATE issues SET status = 'sent', started = ? WHERE id = ?", e.now.Unix(), w.ID)

	home := e.do("GET", "https://news.example.com/", "")
	body := home.Body.String()
	if home.Code != 200 || !strings.Contains(body, "Bakers Weekly") || !strings.Contains(body, `action="/subscribe"`) ||
		!strings.Contains(body, "First bake") || strings.Contains(body, "Secret draft") || !strings.Contains(body, "Bread, weekly.") {
		t.Fatalf("home %d\n%s", home.Code, body)
	}
	if h := home.Header().Get("Content-Security-Policy"); !strings.Contains(h, "frame-ancestors 'none'") {
		t.Fatal("no CSP")
	}
	if r := e.do("GET", "https://news.example.com/issues/first-bake", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "Crust matters") {
		t.Fatalf("issue %d", r.Code)
	}
	if r := e.do("GET", "https://news.example.com/issues/"+draft.Slug, ""); r.Code != 404 {
		t.Fatal("draft visible")
	}
	feed := e.do("GET", "https://news.example.com/feed.xml", "")
	var f atomFeed
	if err := xml.Unmarshal(feed.Body.Bytes(), &f); err != nil || len(f.Entries) != 1 || f.Entries[0].Title != "First bake" {
		t.Fatalf("feed %v %+v", err, f)
	}
	if r := e.do("GET", "https://mail.example.com/lists/plain/", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "Plain") {
		t.Fatalf("mail host list %d", r.Code)
	}
	if r := e.do("GET", "https://mail.example.com/lists/bakers/", ""); r.Code != 404 {
		t.Fatal("list with own host served on mail host too")
	}
	if r := e.do("GET", "https://unknown.example.org/", ""); r.Code != 404 {
		t.Fatal("unknown host served")
	}
	if r := e.do("GET", "https://mta-sts.example.com/.well-known/mta-sts.txt", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "mx: mail.example.com") {
		t.Fatalf("mta-sts %d %s", r.Code, r.Body.String())
	}
	if r := e.do("GET", "https://mta-sts.other.org/.well-known/mta-sts.txt", ""); r.Code != 404 {
		t.Fatal("mta-sts for a foreign domain")
	}
}

func TestSignupProtections(t *testing.T) {
	e := setup(t)
	form := func(v url.Values) string { return v.Encode() }
	r := e.do("POST", "https://news.example.com/subscribe", form(url.Values{"email": {"bot@x.org"}, "website": {"spam"}}))
	if r.Code != 200 || len(e.signups) != 0 {
		t.Fatal("honeypot")
	}
	fresh := e.srv.Tokens.Make(token.Form, e.now.Unix())
	r = e.do("POST", "https://news.example.com/subscribe", form(url.Values{"email": {"fast@x.org"}, "ts": {fresh}}))
	if r.Code != 200 || len(e.signups) != 0 {
		t.Fatal("too-fast form accepted")
	}
	old := e.srv.Tokens.Make(token.Form, e.now.Add(-time.Minute).Unix())
	r = e.do("POST", "https://news.example.com/subscribe", form(url.Values{"email": {"reader@x.org"}, "ts": {old}}))
	if r.Code != 200 || len(e.signups) != 1 || !strings.Contains(r.Body.String(), "Check your inbox") {
		t.Fatalf("signup %d %v", r.Code, e.signups)
	}
	r = e.do("POST", "https://news.example.com/subscribe", form(url.Values{"email": {"not an address"}}))
	if r.Code != 400 {
		t.Fatalf("invalid address %d", r.Code)
	}
	r = e.do("POST", "https://news.example.com/subscribe", `{"email":"js@x.org"}`, "Content-Type", "application/json", "Accept", "application/json")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"ok":true`) || r.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("json %d %s", r.Code, r.Body.String())
	}
	for i := 0; i < 12; i++ {
		r = e.do("POST", "https://news.example.com/subscribe", form(url.Values{"email": {"flood@x.org"}}))
	}
	if r.Code != 429 {
		t.Fatalf("no rate limit: %d", r.Code)
	}
}

func TestConfirmAndUnsubscribeNeedAClick(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	m, _, _ := e.srv.Lists.Subscribe(ctx, e.l, "reader@x.org", lists.Signup{})
	link := "https://mail.example.com/confirm/" + e.srv.Tokens.Make(token.Confirm, m.ID, m.SignupAt.Unix())
	r := e.do("GET", link, "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `<form method="post"`) {
		t.Fatalf("confirm page %d", r.Code)
	}
	if m, _ = e.srv.Lists.Member(ctx, m.ID); m.Status != "pending" {
		t.Fatal("GET confirmed")
	}
	if r = e.do("POST", link, ""); r.Code != 200 {
		t.Fatal(r.Code)
	}
	if m, _ = e.srv.Lists.Member(ctx, m.ID); m.Status != "confirmed" || m.ConfirmIP != "198.51.100.7" {
		t.Fatalf("%+v", m)
	}
	unsub := "https://mail.example.com/u/" + e.srv.Tokens.Make(token.Unsub, m.ID, 0)
	e.do("GET", unsub, "")
	if m, _ = e.srv.Lists.Member(ctx, m.ID); m.Status != "confirmed" {
		t.Fatal("GET unsubscribed")
	}
	e.do("POST", unsub, "List-Unsubscribe=One-Click")
	if m, _ = e.srv.Lists.Member(ctx, m.ID); m.Status != "unsubscribed" || m.UnsubHow != "one-click" {
		t.Fatalf("%+v", m)
	}
	if r := e.do("GET", "https://mail.example.com/u/forged", ""); r.Code != 404 {
		t.Fatal("forged token")
	}
	// RFC 8058 posts multipart/form-data.
	m2, _, _ := e.srv.Lists.Add(ctx, e.l, "other@x.org", "", true, lists.Signup{Consent: "form"})
	body := "--B\r\nContent-Disposition: form-data; name=\"List-Unsubscribe\"\r\n\r\nOne-Click\r\n--B--\r\n"
	e.do("POST", "https://mail.example.com/u/"+e.srv.Tokens.Make(token.Unsub, m2.ID, 0), body, "Content-Type", "multipart/form-data; boundary=B")
	if m2, _ = e.srv.Lists.Member(ctx, m2.ID); m2.Status != "unsubscribed" || m2.UnsubHow != "one-click" {
		t.Fatalf("multipart one-click: %+v", m2)
	}
	// An erased reader's one-click succeeds quietly.
	e.srv.Lists.Forget(ctx, "other@x.org")
	if r := e.do("POST", "https://mail.example.com/u/"+e.srv.Tokens.Make(token.Unsub, m2.ID, 0), "List-Unsubscribe=One-Click"); r.Code != 200 {
		t.Fatalf("one-click after erasure: %d", r.Code)
	}
}

func TestTrackingAndHooks(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	m, _, _ := e.srv.Lists.Add(ctx, e.l, "reader@x.org", "", true, lists.Signup{})
	w, _ := e.srv.Issues.Create(ctx, "bakers", issues.Draft{Markdown: "# Hi\n\n[Go](https://example.com/target)"})
	res, _ := e.srv.DB.W.Exec("INSERT INTO outbound(kind, domain_id, issue_id, contact_id, member_id, env_from, rcpt, status, next_at, created) VALUES('issue', ?, ?, ?, ?, '', ?, 'sent', 0, 0)",
		e.l.DomainID, w.ID, m.ContactID, m.ID, m.Email)
	oid, _ := res.LastInsertId()
	pix := "https://mail.example.com/o/" + e.srv.Tokens.Make(token.Open, oid) + ".gif"
	for i := 0; i < 2; i++ {
		if r := e.do("GET", pix, "", "User-Agent", "Mozilla/5.0 (iPhone)"); r.Header().Get("Content-Type") != "image/gif" {
			t.Fatal("pixel")
		}
	}
	click := "https://mail.example.com/c/" + e.srv.Tokens.Make(token.Click, oid, 0)
	r := e.do("GET", click, "", "User-Agent", "Mozilla/5.0 (Macintosh)")
	if r.Code != 302 || r.Header().Get("Location") != "https://example.com/target" {
		t.Fatalf("click %d %s", r.Code, r.Header().Get("Location"))
	}
	st, _ := e.srv.Issues.Stats(ctx, w.ID)
	if st.Opened != 1 || st.Clicked != 1 || st.Links["https://example.com/target"] != 1 {
		t.Fatalf("stats %+v", st)
	}
	if r := e.do("GET", "https://mail.example.com/c/"+e.srv.Tokens.Make(token.Click, oid, 9), ""); r.Code != 404 {
		t.Fatal("unknown link index redirected")
	}
	hook := "https://mail.example.com" + e.srv.HookPath("smtp2go")
	if r := e.do("POST", hook, `{"event":"bounce","rcpt":"reader@x.org","bounce":"hard"}`, "Content-Type", "application/json"); r.Code != 204 || len(e.feedback) != 1 {
		t.Fatalf("hook %d %v", r.Code, e.feedback)
	}
	if r := e.do("POST", "https://mail.example.com/hooks/wrong/smtp2go", "{}", "Content-Type", "application/json"); r.Code != 404 {
		t.Fatal("hook without secret accepted")
	}
}

func TestTemplateOverrideAndAssets(t *testing.T) {
	e := setup(t)
	os.MkdirAll(filepath.Join(e.dir, "bakers", "assets"), 0o755)
	os.WriteFile(filepath.Join(e.dir, "bakers", "home.html"), []byte(`{{define "content"}}<h1 class="custom">{{.ListName}} custom</h1>{{template "signup" .}}{{end}}`), 0o644)
	os.WriteFile(filepath.Join(e.dir, "bakers", "assets", "logo.svg"), []byte("<svg/>"), 0o644)
	if r := e.do("GET", "https://news.example.com/", ""); !strings.Contains(r.Body.String(), `class="custom"`) {
		t.Fatal("override ignored")
	}
	if r := e.do("GET", "https://news.example.com/assets/logo.svg", ""); r.Code != 200 {
		t.Fatal("asset", r.Code)
	}
	if r := e.do("GET", "https://news.example.com/assets/../home.html", ""); r.Code == 200 {
		t.Fatal("path traversal")
	}
	os.WriteFile(filepath.Join(e.dir, "bakers", "broken.html"), nil, 0o644)
	if err := e.srv.Sites.Check("bakers"); err != nil {
		t.Fatal(err)
	}
	if len(Templates()) != 6 {
		t.Fatalf("templates %d", len(Templates()))
	}
	// An upload goes live only when it parses, and never escapes its folder.
	home := []byte(`{{define "content"}}<h1>Custom {{.Headline}}</h1>{{template "signup" .}}{{end}}`)
	if err := e.srv.Sites.Put("plain", map[string][]byte{"home.html": home, "assets/site.css": []byte("body{}")}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string][]byte{
		{"home.html": []byte(`{{define "content"}}{{.Broken`)},
		{"../evil.html": []byte("x")},
		{"assets/run.sh": []byte("x")},
		{"notes.txt": []byte("x")},
	} {
		if err := e.srv.Sites.Put("plain", bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(e.dir, "plain", "home.html")); string(b) != string(home) {
		t.Fatal("a rejected upload replaced the live site")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "104.16.1.1:443"
	r.Header.Set("CF-Connecting-IP", "203.0.113.9")
	if clientIP(r) != "203.0.113.9" {
		t.Fatal("cloudflare header ignored")
	}
	r.RemoteAddr = "198.51.100.1:443"
	if clientIP(r) != "198.51.100.1" {
		t.Fatal("spoofed header trusted")
	}
	_ = http.StatusOK
}
