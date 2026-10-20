package lists

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }
func ptr[T any](v T) *T              { return &v }

func setup(t *testing.T) (*Service, *List, *clock) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	(&domains.Service{DB: db, Now: c.Now}).Add(ctx, "example.com")
	s := &Service{DB: db, Events: &events.Bus{DB: db, Now: c.Now}, Now: c.Now}
	l, err := s.Create(ctx, "bakers", Input{Name: ptr("Bakers Weekly"), From: ptr("News@example.com"), Host: ptr("news.example.com"), Language: ptr("de")})
	if err != nil {
		t.Fatal(err)
	}
	return s, l, c
}

func TestCreateUpdate(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	if l.From != "news@example.com" || l.FromName != "Bakers Weekly" || l.Host != "news.example.com" || l.Language != "de" {
		t.Fatalf("list %+v", l)
	}
	if !strings.Contains(l.ConsentText(), "Bakers Weekly per E-Mail") {
		t.Fatal(l.ConsentText())
	}
	if _, err := s.Create(ctx, "bakers", Input{Name: ptr("x"), From: ptr("a@example.com")}); err == nil {
		t.Fatal("duplicate slug")
	}
	if _, err := s.Create(ctx, "Bad Slug", Input{Name: ptr("x"), From: ptr("a@example.com")}); err == nil {
		t.Fatal("bad slug")
	}
	if _, err := s.Create(ctx, "other", Input{Name: ptr("x"), From: ptr("a@unknown.org")}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unknown domain")
	}
	if _, err := s.Create(ctx, "other", Input{Name: ptr("x"), From: ptr("a@example.com"), Host: ptr("news.example.com")}); err == nil {
		t.Fatal("host taken twice")
	}
	l, err := s.Update(ctx, "bakers", Input{Tracking: ptr(true), ImprintURL: ptr("https://example.com/impressum"), From: ptr("hello@example.com")})
	if err != nil || !l.Tracking || l.From != "hello@example.com" {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := s.Update(ctx, "bakers", Input{From: ptr("x@other.org")}); err == nil {
		t.Fatal("sender moved to another domain")
	}
	if _, err := s.Update(ctx, "bakers", Input{PrivacyURL: ptr("javascript:alert(1)")}); err == nil {
		t.Fatal("non-http privacy url")
	}
	byHost, err := s.ByHost(ctx, "NEWS.example.com")
	if err != nil || byHost.ID != l.ID {
		t.Fatal("by host")
	}
}

func TestDoubleOptIn(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	m, send, err := s.Subscribe(ctx, l, "Reader@Mail.org", Signup{IP: "198.51.100.1", Source: "web"})
	if err != nil || !send || m.Status != "pending" || m.Email != "reader@mail.org" || m.Consent == "" || m.SignupIP != "198.51.100.1" {
		t.Fatalf("%+v %v %v", m, send, err)
	}
	if _, send, _ := s.Subscribe(ctx, l, "reader@mail.org", Signup{}); send {
		t.Fatal("resent within cooldown")
	}
	c.Add(2 * time.Hour)
	if _, send, _ := s.Subscribe(ctx, l, "reader@mail.org", Signup{}); !send {
		t.Fatal("no resend after cooldown")
	}
	m, err = s.Confirm(ctx, m.ID, "198.51.100.2")
	if err != nil || m.Status != "confirmed" || m.ConfirmIP != "198.51.100.2" || m.ConfirmAt.IsZero() {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := s.Confirm(ctx, m.ID, ""); err != nil {
		t.Fatal("second confirm", err)
	}
	if _, send, _ := s.Subscribe(ctx, l, "reader@mail.org", Signup{}); send {
		t.Fatal("confirmed member asked again")
	}
	m, err = s.Unsubscribe(ctx, m.ID, 0, "link")
	if err != nil || m.Status != "unsubscribed" || m.UnsubHow != "link" {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err := s.Confirm(ctx, m.ID, ""); err == nil {
		t.Fatal("old confirmation link revived an unsubscribed member")
	}
	m, send, _ = s.Subscribe(ctx, l, "reader@mail.org", Signup{Source: "web again"})
	if !send || m.Status != "pending" || m.Source != "web again" || !m.UnsubAt.IsZero() {
		t.Fatalf("resubscribe %+v", m)
	}
	evs, _ := s.Events.List(ctx, events.Query{ListID: l.ID})
	var kinds []string
	for _, e := range evs {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "subscribed,subscribed,confirmed,unsubscribed,subscribed" {
		t.Fatalf("events %v", kinds)
	}
}

func TestImportExportSuppressForget(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	csvIn := "Email,Name\nann@x.org,Ann\nbad-address\nbo@y.org,\nann@x.org,Ann\n"
	res, err := s.ImportCSV(ctx, l, strings.NewReader(csvIn), true, Signup{Source: "import from beehiiv", Consent: "migrated"})
	if err != nil || res.Added != 2 || res.Existing != 1 || len(res.Invalid) != 1 || len(res.Pending) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	res, _ = s.ImportCSV(ctx, l, strings.NewReader("cy@z.org\n"), false, Signup{Source: "csv"})
	if res.Added != 1 || len(res.Pending) != 1 {
		t.Fatalf("unconfirmed import %+v", res)
	}
	var buf bytes.Buffer
	n, err := s.ExportCSV(ctx, "bakers", &buf)
	if err != nil || n != 3 || !strings.Contains(buf.String(), "ann@x.org,Ann,confirmed,import from beehiiv") {
		t.Fatalf("%d %v\n%s", n, err, buf.String())
	}
	if err := s.Suppress(ctx, "ann@x.org", "hard bounce"); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Contact(ctx, "ann@x.org")
	if c.Suppressed != "hard bounce" || len(c.Lists) != 1 {
		t.Fatalf("%+v", c)
	}
	if err := s.Forget(ctx, "ann@x.org"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Contact(ctx, "ann@x.org"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("forgotten contact still there")
	}
	ms, _ := s.Members(ctx, "bakers", Query{Status: "confirmed"})
	if len(ms) != 1 || ms[0].Email != "bo@y.org" {
		t.Fatalf("members %+v", ms)
	}
}

func TestCheckHealthPauses(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	var issueID int64
	res, _ := s.DB.W.Exec("INSERT INTO issues(list_id, slug, subject, markdown, status, created, updated) VALUES(?, 'w', 's', 'm', 'sent', 0, 0)", l.ID)
	issueID, _ = res.LastInsertId()
	for i := 0; i < 300; i++ {
		m, _, _ := s.Add(ctx, l, fmt.Sprintf("r%d@x.org", i), "", true, Signup{})
		s.DB.W.Exec("INSERT INTO outbound(kind, domain_id, issue_id, contact_id, env_from, rcpt, status, next_at, created) VALUES('issue', ?, ?, ?, '', ?, 'sent', 0, ?)",
			l.DomainID, issueID, m.ContactID, m.Email, c.Now().Unix())
	}
	if reason, _ := s.CheckHealth(ctx, l.ID); reason != "" {
		t.Fatalf("paused a healthy list: %s", reason)
	}
	s.Events.Record(ctx, events.Event{Kind: events.Complained, ListID: l.ID})
	s.Events.Record(ctx, events.Event{Kind: events.Complained, ListID: l.ID})
	reason, err := s.CheckHealth(ctx, l.ID)
	if err != nil || !strings.Contains(reason, "complaint rate") {
		t.Fatalf("%q %v", reason, err)
	}
	l, _ = s.Get(ctx, "bakers")
	if l.Paused == "" {
		t.Fatal("not paused")
	}
	l, _ = s.Pause(ctx, "bakers", "")
	if l.Paused != "" {
		t.Fatal("not resumed")
	}
}

func TestConfirmationExpires(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	m, _, _ := s.Subscribe(ctx, l, "late@x.org", Signup{})
	c.Add(31 * 24 * time.Hour)
	if _, err := s.Confirm(ctx, m.ID, ""); !errors.Is(err, ErrExpired) {
		t.Fatalf("old link: %v", err)
	}
	if _, send, _ := s.Subscribe(ctx, l, "late@x.org", Signup{}); !send {
		t.Fatal("no fresh confirmation")
	}
	if _, err := s.Confirm(ctx, m.ID, ""); err != nil {
		t.Fatalf("fresh link: %v", err)
	}
}

func TestSoftBounces(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	m, _, err := s.Add(ctx, l, "gone@example.org", "", true, Signup{})
	if err != nil {
		t.Fatal(err)
	}
	// Two soft bounces on consecutive days, then mail gets through: counting starts over.
	for i := 0; i < 2; i++ {
		if gone, err := s.SoftBounce(ctx, m.ContactID, "4.2.2 mailbox full"); err != nil || gone {
			t.Fatal(gone, err)
		}
		c.t = c.t.Add(24 * time.Hour)
	}
	s.Engaged(ctx, m.ContactID)
	// Two bounces two weeks apart suppress.
	s.SoftBounce(ctx, m.ContactID, "4.2.2 mailbox full")
	c.t = c.t.Add(15 * 24 * time.Hour)
	if gone, err := s.SoftBounce(ctx, m.ContactID, "4.2.2 mailbox full"); err != nil || !gone {
		t.Fatal("not suppressed after two weeks", gone, err)
	}
	if ct, _ := s.Contact(ctx, "gone@example.org"); ct == nil || !strings.Contains(ct.Suppressed, "2 soft bounces") {
		t.Fatalf("%+v", ct)
	}
	// Three in a row suppress at once.
	m2, _, _ := s.Add(ctx, l, "full@example.org", "", true, Signup{})
	for i := 0; i < 3; i++ {
		gone, _ := s.SoftBounce(ctx, m2.ContactID, "4.2.2")
		if gone != (i == 2) {
			t.Fatal("bounce", i, gone)
		}
	}
}

// Readers who left, complained or were erased never come back through an
// import; a new double opt-in brings them back.
func TestOptOutSurvivesImport(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	old := Signup{Source: "migration", Consent: "Old form"}
	m, _, _ := s.Add(ctx, l, "left@x.org", "", true, old)
	s.Unsubscribe(ctx, m.ID, 0, "link")
	s.Add(ctx, l, "angry@x.org", "", true, old)
	s.Suppress(ctx, "angry@x.org", "complaint")
	s.Add(ctx, l, "gone@x.org", "", true, old)
	if err := s.Forget(ctx, "gone@x.org"); err != nil {
		t.Fatal(err)
	}
	res, err := s.ImportCSV(ctx, l, strings.NewReader("email\nleft@x.org\nangry@x.org\nGone@x.org\nnew@x.org\n"), true, old)
	if err != nil || res.OptedOut != 3 || res.Added != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if again, _ := s.Find(ctx, "bakers", "left@x.org"); again.Status != "unsubscribed" {
		t.Fatalf("import revived an unsubscribed reader: %s", again.Status)
	}
	// Signing up again and confirming is fresh consent.
	p, send, err := s.Subscribe(ctx, l, "gone@x.org", Signup{Source: "web"})
	if err != nil || !send {
		t.Fatalf("signup after erasure: %v %v", send, err)
	}
	if _, err := s.Confirm(ctx, p.ID, "198.51.100.1"); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.R.QueryRow("SELECT count(*) FROM forgotten").Scan(&n)
	if n != 0 {
		t.Fatal("confirmation kept the erasure hash")
	}
}

// Someone filling in forms with a victim's address gets the victim at most
// MaxConfirms confirmation requests a month.
func TestConfirmationCap(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	sent := 0
	for range 6 {
		if _, send, err := s.Subscribe(ctx, l, "victim@x.org", Signup{Source: "web"}); err != nil {
			t.Fatal(err)
		} else if send {
			sent++
		}
		c.t = c.t.Add(2 * ConfirmCooldown)
	}
	if sent != MaxConfirms {
		t.Fatalf("%d confirmation requests, want %d", sent, MaxConfirms)
	}
	c.t = c.t.Add(ConfirmWindow)
	if _, send, _ := s.Subscribe(ctx, l, "victim@x.org", Signup{Source: "web"}); !send {
		t.Fatal("cap never lifts")
	}
}

// A platform export keeps each reader's own opt-in date and IP: the consent
// proof and the footer's "you subscribed on" stay true after a move.
func TestImportKeepsOptIn(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	csvText := "Email Address,First Name,Last Name,OPTIN_TIME,OPTIN_IP,CONFIRM_TIME,CONFIRM_IP\n" +
		"leon@x.org,Leon,Muster,2025-03-01 10:00:00,198.51.100.4,2025-03-01 10:05:00,198.51.100.5\n"
	if _, err := s.ImportCSV(ctx, l, strings.NewReader(csvText), true, Signup{Source: "Mailchimp", Consent: "Mailchimp form"}); err != nil {
		t.Fatal(err)
	}
	m, err := s.Find(ctx, "bakers", "leon@x.org")
	if err != nil {
		t.Fatal(err)
	}
	if m.SignupAt.Format(time.DateTime) != "2025-03-01 10:00:00" || m.SignupIP != "198.51.100.4" || m.ConfirmAt.Format(time.DateTime) != "2025-03-01 10:05:00" || m.ConfirmIP != "198.51.100.5" || m.Name != "Leon Muster" {
		t.Fatalf("%+v", m)
	}
}

// A stale list confirms again: readers turn pending and get a mail; a resume
// starts the list's health afresh.
func TestReconfirmAndResume(t *testing.T) {
	ctx := context.Background()
	s, l, c := setup(t)
	s.Add(ctx, l, "old@x.org", "", true, Signup{Source: "2019", Consent: "checkout box"})
	m, send, err := s.Subscribe(ctx, l, "old@x.org", Signup{Source: "reconfirm", Reconfirm: true})
	if err != nil || !send || m.Status != "pending" {
		t.Fatalf("reconfirm: %v %v %+v", send, err, m)
	}
	s.Pause(ctx, "bakers", "bounces")
	c.Add(time.Hour)
	s.Pause(ctx, "bakers", "")
	var resumed int64
	s.DB.R.QueryRow("SELECT resumed FROM lists WHERE slug = 'bakers'").Scan(&resumed)
	if resumed != c.Now().Unix() {
		t.Fatalf("resumed %d", resumed)
	}
}

func TestImportSkipsSuppressed(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	s.Suppress(ctx, "gone@x.org", "hard bounce: 550 5.1.1")
	res, err := s.ImportCSV(ctx, l, strings.NewReader("email\ngone@x.org\nnew@x.org\n"), false, Signup{Source: "import"})
	if err != nil || res.OptedOut != 1 || len(res.Pending) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

// Moving a list's website keeps the old host served: unsubscribe links in
// mail already sent point there.
func TestOldHostsStayServed(t *testing.T) {
	ctx := context.Background()
	s, l, _ := setup(t)
	if _, err := s.Update(ctx, "bakers", Input{Host: ptr("updates.example.com")}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ByHost(ctx, "news.example.com"); err != nil || got.ID != l.ID {
		t.Fatalf("old host: %v %v", got, err)
	}
	hosts, _ := s.Hosts(ctx, 0)
	if strings.Join(hosts, ",") != "updates.example.com,news.example.com" {
		t.Fatalf("hosts %v", hosts)
	}
}
