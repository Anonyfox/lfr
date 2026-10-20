package mailbox

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/store"
)

type outbox struct{ items []outbound.Item }

func (o *outbox) EnqueueTx(_ context.Context, _ outbound.Execer, items ...outbound.Item) (int, error) {
	o.items = append(o.items, items...)
	return len(items), nil
}
func (o *outbox) Wake() {}

func newService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	bs, _ := blob.Open(filepath.Join(dir, "msg"))
	now := func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	ds := &domains.Service{DB: db, Now: now}
	if _, err := ds.Add(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Add(ctx, "other.org"); err != nil {
		t.Fatal(err)
	}
	return &Service{DB: db, Blobs: bs, Now: now}
}

func TestResolveRules(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	jo, err := s.Add(ctx, "Jo@example.com", "Jo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(ctx, "jo@example.com", ""); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := s.Add(ctx, "dmarc-reports@example.com", ""); err == nil {
		t.Fatal("reserved local accepted")
	}
	if _, err := s.Add(ctx, "x@unknown.net", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown domain: %v", err)
	}
	if err := s.AddAlias(ctx, "hello@example.com", "jo@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAlias(ctx, "hi@other.org", "jo@example.com"); err == nil {
		t.Fatal("cross-domain alias accepted")
	}
	if err := s.AddAlias(ctx, "jo@example.com", "jo@example.com"); !errors.Is(err, ErrExists) {
		t.Fatal("alias shadowing a mailbox accepted")
	}
	cases := map[string]bool{
		"jo@example.com": true, "JO@EXAMPLE.COM": true, "jo+news@example.com": true,
		"hello@example.com": true, "hello+x@example.com": true, "nobody@example.com": false, "jo@other.org": false,
	}
	for rcpt, want := range cases {
		id, ok, err := s.Resolve(ctx, rcpt)
		if err != nil || ok != want || ok && id != jo.ID {
			t.Errorf("Resolve(%s) = %d %v %v", rcpt, id, ok, err)
		}
	}
	if err := s.SetCatchall(ctx, "example.com", "jo@example.com"); err != nil {
		t.Fatal(err)
	}
	if id, ok, _ := s.Resolve(ctx, "anything@example.com"); !ok || id != jo.ID {
		t.Fatal("catch-all")
	}
	if err := s.SetCatchall(ctx, "other.org", "jo@example.com"); err == nil {
		t.Fatal("foreign catch-all accepted")
	}
	s.SetCatchall(ctx, "example.com", "off")
	if _, ok, _ := s.Resolve(ctx, "anything@example.com"); ok {
		t.Fatal("catch-all still on")
	}
	list, _ := s.List(ctx, "example.com")
	if len(list) != 2 { // postmaster and jo
		t.Fatalf("mailboxes %d", len(list))
	}
}

func raw(from, to, subject, id, inReplyTo, body string) []byte {
	b, _ := msg.Compose(msg.Draft{From: msg.Address{Email: from}, To: []msg.Address{{Email: to}}, Subject: subject,
		MessageID: id, InReplyTo: inReplyTo, Text: body, Date: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)})
	return b
}

func TestDeliverSearchThreadMoveDelete(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	mb, _ := s.Add(ctx, "jo@example.com", "")
	id1, err := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("ann@x.org", "jo@example.com", "Sourdough starter", "a1@x.org", "", "My starter smells of acetone. Rescue?")})
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("ann@x.org", "jo@example.com", "Re: Sourdough starter", "a2@x.org", "a1@x.org", "Fed it twice, better now.")})
	s.Deliver(ctx, Delivery{MailboxID: mb.ID, Folder: "junk", Spam: 0.9, Raw: raw("spam@bad.biz", "jo@example.com", "WIN", "s@bad.biz", "", "cheap pills")})

	inbox, _ := s.Messages(ctx, Query{Mailbox: "jo@example.com"})
	if len(inbox) != 2 || inbox[0].ID != id2 || inbox[0].Thread != inbox[1].Thread {
		t.Fatalf("inbox %+v", inbox)
	}
	for q, want := range map[string]int{"acetone": 1, "sourd": 2, "subject:starter": 2, "from:ann": 2, `"fed it twice"`: 1, "pills": 1, "nothing-like-this": 0, `"unbalanced`: 0, "AND OR NOT ( )": 0} {
		got, err := s.Messages(ctx, Query{Search: q})
		if err != nil || len(got) != want {
			t.Errorf("search %q = %d results, %v; want %d", q, len(got), err, want)
		}
	}
	m, err := s.Read(ctx, id1, true)
	if err != nil || !strings.Contains(m.Text, "acetone") || !m.Seen {
		t.Fatalf("read %+v %v", m, err)
	}
	unread, _ := s.Messages(ctx, Query{Mailbox: "jo@example.com", Unread: true})
	if len(unread) != 1 {
		t.Fatalf("unread %d", len(unread))
	}
	if old, err := s.Move(ctx, id1, "spam"); err != nil || old != "inbox" {
		t.Fatal(old, err)
	}
	junk, _ := s.Messages(ctx, Query{Folder: "junk"})
	if len(junk) != 2 {
		t.Fatalf("junk %d", len(junk))
	}
	if err := s.Delete(ctx, id1, false); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Read(ctx, id1, false); m.Folder != "trash" {
		t.Fatalf("folder %s", m.Folder)
	}
	if err := s.Delete(ctx, id1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(ctx, id1, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after purge: %v", err)
	}
	if got, _ := s.Messages(ctx, Query{Search: "acetone"}); len(got) != 0 {
		t.Fatal("purged message still indexed")
	}
	if err := s.Remove(ctx, "jo@example.com"); err != nil {
		t.Fatal(err)
	}
	var n int
	s.Blobs.Walk(func(string) error { n++; return nil })
	if n != 0 {
		t.Fatalf("%d blobs left after removing the mailbox", n)
	}
}

func TestSendReplyForward(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	mb, _ := s.Add(ctx, "jo@example.com", "Jo Example")
	s.AddAlias(ctx, "hello@example.com", "jo@example.com")
	out := &outbox{}
	id, err := s.Send(ctx, out, SendRequest{From: "hello@example.com", To: []string{"Ann <ann@x.org>, bo@y.org"}, Bcc: []string{"ann@x.org", "secret@z.org"}, Subject: "Hi", Text: "Hello there"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.items) != 3 || out.items[0].EnvFrom != "jo@example.com" || out.items[0].Blob == "" {
		t.Fatalf("items %+v", out.items)
	}
	sent, _ := s.Read(ctx, id, false)
	if sent.Folder != "sent" || sent.From != "hello@example.com" || strings.Contains(strings.Join(sent.To, ","), "secret") {
		t.Fatalf("sent copy %+v", sent)
	}
	rawSent, _ := s.Raw(ctx, id)
	if strings.Contains(string(rawSent), "secret@z.org") {
		t.Fatal("Bcc leaked into the message")
	}
	if _, err := s.Send(ctx, out, SendRequest{From: "nobody@example.com", To: []string{"a@b.org"}, Subject: "x", Text: "y"}); err == nil {
		t.Fatal("sent from a nonexistent mailbox")
	}
	s.SetCatchall(ctx, "example.com", "jo@example.com")
	if _, err := s.Send(ctx, out, SendRequest{From: "ceo@example.com", To: []string{"a@b.org"}, Subject: "x", Text: "y"}); err == nil {
		t.Fatal("sent as a catch-all address")
	}

	in, _ := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("ann@x.org", "jo@example.com", "Question", "q1@x.org", "", "Line one\nLine two")})
	out.items = nil
	rid, err := s.Reply(ctx, out, in, ReplyRequest{Text: "Answer", Quote: true})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := s.Read(ctx, rid, false)
	if r.Subject != "Re: Question" || r.InReplyTo != "q1@x.org" || !strings.Contains(r.Text, "> Line two") || len(out.items) != 1 || out.items[0].Rcpt != "ann@x.org" {
		t.Fatalf("reply %+v items %+v", r, out.items)
	}
	orig, _ := s.Read(ctx, in, false)
	if !orig.Answered || orig.Thread != r.Thread {
		t.Fatalf("original %+v", orig)
	}
	// A message to the alias is answered from the alias, quoted in both parts.
	viaAlias, _ := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("ann@x.org", "hello@example.com", "Shipping?", "q2@x.org", "", "Do you ship?")})
	aid, err := s.Reply(ctx, out, viaAlias, ReplyRequest{Text: "Yes.", HTML: "<p>Yes.</p>", Quote: true})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Read(ctx, aid, false)
	if a.From != "hello@example.com" || !strings.Contains(a.HTML, "Do you ship?") || strings.Contains(a.HTML, "</html><p") {
		t.Fatalf("alias reply from %s html %q", a.From, a.HTML)
	}
	// Mail that came in through the catch-all is answered from the mailbox.
	viaCatchall, _ := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("ann@x.org", "termin@example.com", "Termin?", "q3@x.org", "", "Dienstag?")})
	cid, err := s.Reply(ctx, out, viaCatchall, ReplyRequest{Text: "Ja.", Quote: true})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Read(ctx, cid, false); c.From != "jo@example.com" {
		t.Fatalf("catch-all reply from %s", c.From)
	}

	fwdTo := "me@elsewhere.net"
	mb, _ = s.Update(ctx, "jo@example.com", nil, &fwdTo)
	rawIn := raw("ann@x.org", "jo@example.com", "Fwd me", "f1@x.org", "", "body")
	rawIn = append([]byte("DKIM-Signature: v=1; d=x.org; b=abc\r\n"), rawIn...)
	item, err := s.ForwardItem(ctx, mb, rawIn, msg.Parse(rawIn))
	if err != nil {
		t.Fatal(err)
	}
	fwd, _ := s.Blobs.Get(item.Blob)
	p := msg.Parse(fwd)
	if item.Rcpt != fwdTo || p.From.Email != "jo@example.com" || len(p.ReplyTo) != 1 || p.ReplyTo[0].Email != "ann@x.org" || strings.Contains(string(fwd), "d=x.org") {
		t.Fatalf("forward %+v\n%s", item, fwd)
	}
	self := "jo@example.com"
	if _, err := s.Update(ctx, "jo@example.com", nil, &self); err == nil {
		t.Fatal("forward to itself accepted")
	}
}

func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"hello":            `"hello"*`,
		"from:ann starter": `people:"ann"* AND "starter"*`,
		`"fed it" x`:       `"fed it" AND "x"*`,
		"a-b":              `"a b"`,
		"*":                `""`,
	}
	for in, want := range cases {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %s, want %s", in, got, want)
		}
	}
}

// FuzzSearch checks that no search text, however odd, makes SQLite reject the query.
func FuzzSearch(f *testing.F) {
	s := newService(&testing.T{})
	for _, seed := range []string{"hello", `"a b" from:x`, "AND OR NOT", "*", `"`, "subject:", "ä ö ü", "a-b_c.d@e"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, q string) {
		if _, err := s.Messages(context.Background(), Query{Search: q}); err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
	})
}

func TestSenderCheckAndCalendar(t *testing.T) {
	for verdict, want := range map[string]string{
		`{"auth":{"dmarc":"pass","from_domain":"bank.de"}}`:                                                       "verified: the mail",
		`{"auth":{"dmarc":"none","from_domain":"handwerk.de","spf":"pass","spf_domain":"handwerk.de"}}`:           "verified: handwerk.de",
		`{"auth":{"dmarc":"none","from_domain":"handwerk.de","dkim":[{"domain":"handwerk.de","result":"pass"}]}}`: "verified: handwerk.de",
		`{"auth":{"dmarc":"none","from_domain":"handwerk.de","spf":"pass","spf_domain":"mailer.example"}}`:        "not verified",
		`{"auth":{"dmarc":"fail","from_domain":"bank.de"}}`:                                                       "FORGED",
	} {
		if got := senderCheck(verdict); !strings.HasPrefix(got, want) {
			t.Errorf("%s: %q", verdict, got)
		}
	}
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Qualitätszirkel\\, Physio\r\nDTSTART:20261105T180000\r\nDTEND:20261105T200000\r\nLOCATION:Praxis Nord\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if got := calendarEvent([]byte(ics)); got != "Qualitätszirkel, Physio, Thu 2026-11-05 18:00 until Thu 2026-11-05 20:00, Praxis Nord" {
		t.Fatalf("%q", got)
	}
}
