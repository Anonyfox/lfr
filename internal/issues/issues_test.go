package issues

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/store"
	"github.com/Anonyfox/lfr/internal/token"
)

type sink struct {
	mu   sync.Mutex
	msgs map[string][]byte
}

func (s *sink) Send(_ context.Context, from, rcpt string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs[rcpt] = raw
	return nil
}
func (s *sink) Close()       {}
func (s *sink) Name() string { return "sink" }

type env struct {
	s    *Service
	r    *Renderer
	l    *lists.List
	sink *sink
	now  time.Time
}

func ptr[T any](v T) *T { return &v }

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), sink: &sink{msgs: map[string][]byte{}}}
	now := func() time.Time { return e.now }
	ds := &domains.Service{DB: db, Now: now}
	ds.Add(ctx, "example.com")
	bus := &events.Bus{DB: db, Now: now}
	ls := &lists.Service{DB: db, Events: bus, Now: now}
	e.l, err = ls.Create(ctx, "bakers", lists.Input{Name: ptr("Bakers Weekly"), From: ptr("news@example.com"), Host: ptr("news.example.com"), Tracking: ptr(true), Cadence: ptr("Every Friday morning")})
	if err != nil {
		t.Fatal(err)
	}
	q := &outbound.Queue{DB: db, Domains: ds, Events: bus, Now: now, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Transport: func(context.Context) (outbound.Transport, string, error) { return e.sink, "sink", nil },
		Rate:      func(context.Context) (int, int) { return 100000, 4 },
		Bounce:    func(r outbound.Row, d string) string { return "bounce@" + d }}
	e.s = &Service{DB: db, Lists: ls, Events: bus, Queue: q, Now: now}
	e.r = &Renderer{Tokens: token.New([]byte("secret")), Now: now,
		ListURL: func(l *lists.List) string { return "https://" + l.Host },
		BaseURL: func(l *lists.List) string { return "https://" + l.Host }}
	q.Builders = map[string]outbound.Builder{outbound.KindIssue: e.s.Builder(e.r)}
	return e
}

const sample = `---
subject: Starter rescue
preheader: Hungry, not dead
---
# Your starter is hungry

It smells of acetone because it ran out of food. [Feed it](https://example.com/feed) twice a day.

[Read the full guide →](https://example.com/guide)
`

func TestCreateSlugsAndFrontMatter(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	w, err := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if err != nil {
		t.Fatal(err)
	}
	if w.Subject != "Starter rescue" || w.Preheader != "Hungry, not dead" || w.Slug != "starter-rescue" || !strings.HasPrefix(w.Summary, "It smells of acetone") || strings.Contains(w.Markdown, "subject:") {
		t.Fatalf("%+v", w)
	}
	w2, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if w2.Slug != "starter-rescue-2" {
		t.Fatal(w2.Slug)
	}
	w3, err := e.s.Create(ctx, "bakers", Draft{Markdown: "# Größe & Übermut"})
	if err != nil || w3.Slug != "groesse-uebermut" || w3.Subject != "Größe & Übermut" {
		t.Fatalf("%+v %v", w3, err)
	}
	if _, err := e.s.Create(ctx, "bakers", Draft{Markdown: "no heading at all"}); err == nil {
		t.Fatal("issue without subject accepted")
	}
	w, err = e.s.Update(ctx, w.ID, Draft{Markdown: sample, Subject: "New subject"})
	if err != nil || w.Subject != "New subject" {
		t.Fatalf("%+v %v", w, err)
	}
}

func TestSendFlow(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	ls := e.s.Lists
	a, _, _ := ls.Add(ctx, e.l, "ann@x.org", "Ann", true, lists.Signup{})
	ls.Add(ctx, e.l, "bo@y.org", "", true, lists.Signup{Consent: e.l.ConsentText()})
	ls.Subscribe(ctx, e.l, "pending@z.org", lists.Signup{})
	ls.Add(ctx, e.l, "gone@z.org", "", true, lists.Signup{})
	ls.Suppress(ctx, "gone@z.org", "hard bounce")

	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if _, err := e.s.Schedule(ctx, w.ID, e.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.s.Tick(ctx)
	if w, _ = e.s.Get(ctx, w.ID); w.Status != "scheduled" {
		t.Fatal("started early")
	}
	e.now = e.now.Add(2 * time.Hour)
	e.s.Tick(ctx)
	st, _ := e.s.Stats(ctx, w.ID)
	if st.Recipients != 2 || st.Queued != 2 {
		t.Fatalf("stats %+v", st)
	}
	// Ann unsubscribes before her copy goes out.
	ls.Unsubscribe(ctx, a.ID, 0, "test")
	for i := 0; i < 3; i++ {
		e.s.Queue.Wake()
		runOnce(t, e)
	}
	e.s.Tick(ctx)
	w, _ = e.s.Get(ctx, w.ID)
	st, _ = e.s.Stats(ctx, w.ID)
	if w.Status != "sent" || st.Sent != 1 || st.Canceled != 1 {
		t.Fatalf("%s %+v", w.Status, st)
	}
	raw := e.sink.msgs["bo@y.org"]
	if raw == nil {
		t.Fatal("bo got nothing")
	}
	p := msg.Parse(raw)
	h := p.Header
	if !strings.HasPrefix(h.Get("List-Unsubscribe"), "<https://news.example.com/u/") || h.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" ||
		!strings.Contains(h.Get("List-Unsubscribe"), "mailto:unsub+") || !strings.Contains(h.Get("List-Id"), "<bakers.example.com>") || h.Get("Feedback-ID") == "" {
		t.Fatalf("headers:\n%s", raw[:1500])
	}
	if !strings.Contains(p.HTML, "https://news.example.com/c/") || !strings.Contains(p.HTML, "https://news.example.com/o/") || strings.Contains(p.HTML, "LFR__") || !strings.Contains(p.HTML, "lfr-button") {
		t.Fatalf("tracking links missing or placeholders left:\n%s", p.HTML)
	}
	if !strings.Contains(p.Text, "Feed it (https://example.com/feed)") || !strings.Contains(p.Text, "https://news.example.com/u/") {
		t.Fatalf("text:\n%s", p.Text)
	}
	if strings.Count(string(raw), "DKIM-Signature") != 2 {
		t.Fatal("not signed")
	}
}

func runOnce(t *testing.T, e *env) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	e.s.Queue.Run(ctx)
}

func TestNoTrackingKeepsLinks(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.s.Lists.Update(ctx, "bakers", lists.Input{Tracking: ptr(false)})
	l, _ := e.s.Lists.Get(ctx, "bakers")
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	raw, err := e.r.Message(l, w, Recipient{OutboundID: 5, MemberID: 1, Email: "x@y.org"}, false)
	if err != nil {
		t.Fatal(err)
	}
	p := msg.Parse(raw)
	if !strings.Contains(p.HTML, `href="https://example.com/feed"`) || strings.Contains(p.HTML, "/o/") || strings.Contains(p.HTML, "/c/") {
		t.Fatalf("%s", p.HTML)
	}
}

func TestTestSendAndCancel(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if err := e.s.Test(ctx, w.ID, []string{"me@example.org"}); err != nil {
		t.Fatal(err)
	}
	runOnce(t, e)
	if p := msg.Parse(e.sink.msgs["me@example.org"]); !strings.HasPrefix(p.Subject, "[Test] ") {
		t.Fatalf("test subject %q", p.Subject)
	}
	if st, _ := e.s.Stats(ctx, w.ID); st.Recipients != 0 {
		t.Fatal("test send counted as recipient")
	}
	e.s.Schedule(ctx, w.ID, time.Time{})
	w, _ = e.s.Cancel(ctx, w.ID)
	if w.Status != "draft" {
		t.Fatal(w.Status)
	}
}

func TestLint(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: "# FREE STUFF FOR EVERYONE NOW\n\nAct now.\n\n![](/img.png)\n\n[here](/relative) [x](https://bit.ly/x) click here"})
	f, err := e.s.Lint(ctx, e.r, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, x := range f {
		got[x.Rule] = x.Severity
	}
	for rule, sev := range map[string]string{"subject.caps": "warning", "images.alt": "warning", "images.url": "error", "links.relative": "error", "links.shortener": "warning", "links.text": "warning", "copy.spammy": "warning", "preheader.missing": "info", "list.address": "error"} {
		if got[rule] != sev {
			t.Errorf("%s = %q, want %s (all: %v)", rule, got[rule], sev, got)
		}
	}
	if !Blocking(f) {
		t.Fatal("errors do not block")
	}
	clean, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	e.s.Lists.Update(ctx, "bakers", lists.Input{Address: ptr("Bakers, Street 1, Berlin"), PrivacyURL: ptr("https://example.com/privacy")})
	f, _ = e.s.Lint(ctx, e.r, clean.ID)
	if len(f) != 0 {
		t.Fatalf("clean issue has findings: %+v", f)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{"Hello, World!": "hello-world", "  Ça va?  ": "ca-va", "Äpfel & Öl": "aepfel-oel", "!!!": ""} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArchiveKeepsStats(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.s.Lists.Add(ctx, e.l, "a@x.org", "", true, lists.Signup{})
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	e.s.Schedule(ctx, w.ID, time.Time{})
	e.s.Tick(ctx)
	runOnce(t, e)
	e.s.Tick(ctx)
	before, _ := e.s.Stats(ctx, w.ID)
	e.now = e.now.Add(500 * 24 * time.Hour)
	n, err := e.s.Archive(ctx, e.now.Add(-400*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("archive %d %v", n, err)
	}
	var rows int
	e.s.DB.R.QueryRow("SELECT count(*) FROM outbound WHERE issue_id = ?", w.ID).Scan(&rows)
	after, _ := e.s.Stats(ctx, w.ID)
	if rows != 0 || after.Sent != 1 || after.Sent != before.Sent {
		t.Fatalf("rows %d, before %+v after %+v", rows, before, after)
	}
}

func FuzzFrontMatterAndSlug(f *testing.F) {
	f.Add("---\nsubject: x\n---\n# Hi")
	f.Add("---\n---")
	f.Fuzz(func(t *testing.T, s string) {
		FrontMatter(s)
		if slug := Slugify(s); slug != "" && !slugRE.MatchString(slug) {
			t.Fatalf("Slugify(%q) = %q is not a valid slug", s, slug)
		}
	})
}

func TestPutTestedWelcomeAndGap(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	w, created, err := e.s.Put(ctx, "bakers", Draft{Markdown: sample, Slug: "starter-rescue"}, false)
	if err != nil || !created || w.Slug != "starter-rescue" {
		t.Fatal(w, created, err)
	}
	// Same slug updates in place; a test copy is remembered until the content changes.
	if err := e.s.Test(ctx, w.ID, []string{"me@example.org"}); err != nil {
		t.Fatal(err)
	}
	if w, _ = e.s.Get(ctx, w.ID); w.Tested.IsZero() {
		t.Fatal("test not recorded")
	}
	again, created, err := e.s.Put(ctx, "bakers", Draft{Markdown: sample, Slug: "starter-rescue"}, false)
	if err != nil || created || again.ID != w.ID || again.Tested.IsZero() {
		t.Fatal("unchanged put lost the test", again, created, err)
	}
	changed, _, err := e.s.Put(ctx, "bakers", Draft{Markdown: sample + "\nOne more line.\n", Slug: "starter-rescue"}, false)
	if err != nil || !changed.Tested.IsZero() {
		t.Fatal("changed content kept the old test", changed, err)
	}

	// The minimum gap: a second issue right after the first is crowding it.
	if _, err := e.s.Schedule(ctx, w.ID, e.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	next, _, _ := e.s.Put(ctx, "bakers", Draft{Markdown: "# Second\n\nMore bread.", Slug: "second"}, false)
	if other, err := e.s.Crowding(ctx, next.ID, e.now.Add(3*time.Hour)); err != nil || other == nil || other.ID != w.ID {
		t.Fatal("gap not enforced", other, err)
	}
	if other, _ := e.s.Crowding(ctx, next.ID, e.now.Add(30*time.Hour)); other != nil {
		t.Fatal("a day later is fine", other)
	}
	e.now = e.now.Add(2 * time.Hour)
	e.s.Tick(ctx)
	if _, _, err := e.s.Put(ctx, "bakers", Draft{Markdown: sample, Slug: "starter-rescue"}, false); err == nil {
		t.Fatal("a scheduled-then-sent issue changed")
	}

	// The welcome issue goes to each confirmed member once and is never broadcast.
	wel, _, err := e.s.Put(ctx, "bakers", Draft{Markdown: "# Welcome\n\nHere is what comes and when.", Slug: "welcome"}, true)
	if err != nil || !wel.Welcome {
		t.Fatal(wel, err)
	}
	if _, err := e.s.Schedule(ctx, wel.ID, time.Time{}); err == nil {
		t.Fatal("welcome issue scheduled")
	}
	m, _, err := e.s.Lists.Add(ctx, e.l, "new@example.org", "", true, lists.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	e.s.Welcome(ctx, e.l, m)
	e.s.Welcome(ctx, e.l, m)
	// Without a postal address the welcome waits instead of going out unlawful.
	runOnce(t, e)
	if e.sink.msgs["new@example.org"] != nil {
		t.Fatal("welcome issue sent without the legal footer")
	}
	e.s.Lists.Update(ctx, "bakers", lists.Input{Address: ptr("Bakers, Ofenweg 3, 10115 Berlin")})
	e.s.DB.W.Exec("UPDATE outbound SET next_at = 0 WHERE issue_id = ?", wel.ID)
	runOnce(t, e)
	if p := msg.Parse(e.sink.msgs["new@example.org"]); p.Subject != "Welcome" || !strings.Contains(p.Text, "Ofenweg 3") || strings.Contains(p.Text, "/issues/welcome") {
		t.Fatalf("welcome subject %q", p.Subject)
	}
	if st, _ := e.s.Stats(ctx, wel.ID); st.Recipients != 1 {
		t.Fatalf("welcome sent %d times", st.Recipients)
	}
	// Only one welcome issue per list.
	other, _, _ := e.s.Put(ctx, "bakers", Draft{Markdown: "# Hello\n\nNew welcome.", Slug: "hello"}, true)
	if wel, _ = e.s.Get(ctx, wel.ID); wel.Welcome || !other.Welcome {
		t.Fatal("two welcome issues")
	}
}

func TestFooterSaysWhy(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	m, _, err := e.s.Lists.Add(ctx, e.l, "ann@example.org", "", true, lists.Signup{Source: "web: https://news.example.com/starter"})
	if err != nil {
		t.Fatal(err)
	}
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	e.s.Schedule(ctx, w.ID, time.Time{})
	e.s.Tick(ctx)
	runOnce(t, e)
	p := msg.Parse(e.sink.msgs[m.Email])
	for _, want := range []string{"subscribed to Bakers Weekly on October 19, 2026 at news.example.com", "Forwarded to you? Subscribe at", "Hungry, not dead"} {
		if !strings.Contains(p.Text, want) || !strings.Contains(p.HTML, want) {
			t.Errorf("missing %q\n%s", want, p.Text)
		}
	}
	if strings.Contains(p.HTML, "display:none") {
		t.Error("hidden content in the email")
	}
}

// A postal address added after the issue was rendered reaches the send: the
// render cache follows the list's settings.
func TestFooterFollowsListChanges(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	m, _, _ := e.s.Lists.Add(ctx, e.l, "ann@example.org", "", true, lists.Signup{Source: "import"})
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if _, err := e.s.Lint(ctx, e.r, w.ID); err != nil {
		t.Fatal(err)
	}
	e.s.Lists.Update(ctx, "bakers", lists.Input{Address: ptr("Bakers, Ofenweg 3, 10115 Berlin")})
	e.s.Schedule(ctx, w.ID, time.Time{})
	e.s.Tick(ctx)
	runOnce(t, e)
	if p := msg.Parse(e.sink.msgs[m.Email]); !strings.Contains(p.Text, "Ofenweg 3") {
		t.Fatalf("stale footer:\n%s", p.Text)
	}
}

// Only readers whose consent named open and click statistics are measured.
func TestTrackingFollowsConsent(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.s.Lists.Add(ctx, e.l, "old@x.org", "", true, lists.Signup{Consent: "I want to receive Bakers Weekly by email."})
	e.s.Lists.Add(ctx, e.l, "new@x.org", "", true, lists.Signup{Consent: e.l.ConsentText()})
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	e.s.Schedule(ctx, w.ID, time.Time{})
	e.s.Tick(ctx)
	runOnce(t, e)
	old, fresh := msg.Parse(e.sink.msgs["old@x.org"]), msg.Parse(e.sink.msgs["new@x.org"])
	if strings.Contains(old.HTML, "/o/") || strings.Contains(old.HTML, "/c/") || !strings.Contains(old.HTML, "https://example.com/feed") {
		t.Fatalf("reader without tracking consent is measured:\n%s", old.HTML)
	}
	if !strings.Contains(fresh.HTML, "/o/") || !strings.Contains(fresh.HTML, "/c/") {
		t.Fatal("reader with tracking consent is not measured")
	}
}

// An issue whose slot passed long ago while the server was down waits for the
// owner instead of going out days late.
func TestMissedSlotWaits(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	e.s.Lists.Update(ctx, "bakers", lists.Input{Address: ptr("Bakers, Ofenweg 3, 10115 Berlin")})
	e.s.Lists.Add(ctx, e.l, "ann@example.org", "", true, lists.Signup{Consent: "form"})
	w, _ := e.s.Create(ctx, "bakers", Draft{Markdown: sample})
	if _, err := e.s.Schedule(ctx, w.ID, e.now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(MaxLate + 2*time.Hour)
	e.s.Tick(ctx)
	if w, _ = e.s.Get(ctx, w.ID); w.Status != "scheduled" {
		t.Fatalf("missed issue started: %s", w.Status)
	}
	if missed, _ := e.s.Missed(ctx); len(missed) != 1 {
		t.Fatalf("missed %d", len(missed))
	}
	if _, err := e.s.Schedule(ctx, w.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	e.s.Tick(ctx)
	if w, _ = e.s.Get(ctx, w.ID); w.Status != "sending" {
		t.Fatalf("send now after a missed slot: %s", w.Status)
	}
}
