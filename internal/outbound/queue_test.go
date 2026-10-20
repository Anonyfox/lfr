package outbound

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/store"
)

type fakeTransport struct {
	mu    sync.Mutex
	sent  []string
	froms []string
	raws  [][]byte
	fail  func(rcpt string) error
}

func (f *fakeTransport) Send(_ context.Context, from, rcpt string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		if err := f.fail(rcpt); err != nil {
			return err
		}
	}
	f.sent = append(f.sent, rcpt)
	f.froms = append(f.froms, from)
	f.raws = append(f.raws, raw)
	return nil
}
func (f *fakeTransport) Close()       {}
func (f *fakeTransport) Name() string { return "fake" }

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newQueue(t *testing.T, ft *fakeTransport) (*Queue, *domains.Domain, *clock) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	ds := &domains.Service{DB: db, Now: c.Now}
	d, err := ds.Add(ctx, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	q := &Queue{
		DB: db, Domains: ds, Events: &events.Bus{DB: db, Now: c.Now}, Now: c.Now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Builders: map[string]Builder{KindMail: func(ctx context.Context, r Row) ([]byte, error) {
			return []byte("From: a@example.com\r\nTo: " + r.Rcpt + "\r\nSubject: hi\r\n\r\nbody\r\n"), nil
		}},
		Bounce:    func(r Row, domain string) string { return "bounce+x@" + domain },
		Transport: func(context.Context) (Transport, string, error) { return ft, "fake", nil },
		Rate:      func(context.Context) (int, int) { return 1000, 2 },
	}
	return q, d, c
}

func status(t *testing.T, q *Queue, rcpt string) (string, int) {
	var s string
	var n int
	q.DB.R.QueryRow("SELECT status, attempts FROM outbound WHERE rcpt = ?", rcpt).Scan(&s, &n)
	return s, n
}

func TestQueueDeliversSigned(t *testing.T) {
	ft := &fakeTransport{}
	q, d, _ := newQueue(t, ft)
	ctx := context.Background()
	if err := q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "B@Example.org"}, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "c@example.org", EnvFrom: "me@example.com"}); err != nil {
		t.Fatal(err)
	}
	n, _, err := q.runBatch(ctx)
	if err != nil || n != 2 {
		t.Fatalf("batch %d %v", n, err)
	}
	if s, _ := status(t, q, "b@example.org"); s != "sent" {
		t.Fatalf("status %s", s)
	}
	if len(ft.raws) != 2 || strings.Count(string(ft.raws[0]), "DKIM-Signature") != 2 {
		t.Fatal("not signed twice")
	}
	froms := strings.Join(ft.froms, ",")
	if !strings.Contains(froms, "bounce+x@example.com") || !strings.Contains(froms, "me@example.com") {
		t.Fatalf("envelope senders %v", ft.froms)
	}
	var events int
	q.DB.R.QueryRow("SELECT count(*) FROM events WHERE kind = 'sent'").Scan(&events)
	if events != 2 {
		t.Fatalf("sent events %d", events)
	}
}

func TestQueueRetriesThenGivesUp(t *testing.T) {
	ft := &fakeTransport{fail: func(string) error { return &Failure{Temporary: true, Message: "try later"} }}
	q, d, c := newQueue(t, ft)
	ctx := context.Background()
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "b@example.org"})
	q.runBatch(ctx)
	if s, n := status(t, q, "b@example.org"); s != "queued" || n != 1 {
		t.Fatalf("after first try %s %d", s, n)
	}
	if n, _, _ := q.runBatch(ctx); n != 0 {
		t.Fatal("retried before backoff")
	}
	c.Add(2 * time.Minute)
	q.runBatch(ctx)
	if _, n := status(t, q, "b@example.org"); n != 2 {
		t.Fatalf("attempts %d", n)
	}
	c.Add(6 * 24 * time.Hour)
	q.runBatch(ctx)
	if s, _ := status(t, q, "b@example.org"); s != "failed" {
		t.Fatalf("after max age %s", s)
	}
}

func TestQueueBounceHook(t *testing.T) {
	ft := &fakeTransport{fail: func(string) error { return &Failure{Bounce: true, Code: 550, Enhanced: "5.1.1", Message: "gone"} }}
	q, d, _ := newQueue(t, ft)
	var bounced []string
	q.Hooks.Failed = func(_ context.Context, r Row, f *Failure) {
		if f.Bounce {
			bounced = append(bounced, r.Rcpt)
		}
	}
	ctx := context.Background()
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "gone@example.org"})
	q.runBatch(ctx)
	if s, _ := status(t, q, "gone@example.org"); s != "bounced" || len(bounced) != 1 {
		t.Fatalf("status %s, hook %v", s, bounced)
	}
}

func TestQueueSkipAndRetryDrop(t *testing.T) {
	ft := &fakeTransport{}
	q, d, _ := newQueue(t, ft)
	q.Builders[KindConfirm] = func(context.Context, Row) ([]byte, error) { return nil, ErrSkip }
	ctx := context.Background()
	q.Enqueue(ctx, Item{Kind: KindConfirm, DomainID: d.ID, Rcpt: "x@example.org"})
	q.runBatch(ctx)
	if s, _ := status(t, q, "x@example.org"); s != "canceled" {
		t.Fatalf("status %s", s)
	}
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "later@example.org", At: q.Now().Add(time.Hour)})
	var id int64
	q.DB.R.QueryRow("SELECT id FROM outbound WHERE rcpt = 'later@example.org'").Scan(&id)
	if err := q.Drop(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := q.Drop(ctx, id); err == nil {
		t.Fatal("dropped twice")
	}
	st, err := q.Stats(ctx)
	if err != nil || st.Queued != 0 {
		t.Fatalf("stats %+v %v", st, err)
	}
}

func TestQueueRunWakes(t *testing.T) {
	ft := &fakeTransport{}
	q, d, _ := newQueue(t, ft)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { q.Run(ctx); close(done) }()
	q.Enqueue(context.Background(), Item{Kind: KindMail, DomainID: d.ID, Rcpt: "w@example.org"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s, _ := status(t, q, "w@example.org"); s == "sent" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not delivered after wake")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestQueueRelayOutageKeepsAttempts(t *testing.T) {
	calls := 0
	ft := &fakeTransport{fail: func(string) error {
		calls++
		return &Failure{Temporary: true, Transport: true, Message: "relay login failed"}
	}}
	q, d, _ := newQueue(t, ft)
	ctx := context.Background()
	for _, r := range []string{"a@x.org", "b@x.org", "c@x.org", "d@x.org"} {
		q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: r})
	}
	q.runBatch(ctx)
	if calls > 2 { // at most one per connection before the batch stops
		t.Fatalf("kept sending through a dead relay: %d calls", calls)
	}
	var attempts int
	q.DB.R.QueryRow("SELECT sum(attempts) FROM outbound").Scan(&attempts)
	if attempts != 0 {
		t.Fatalf("outage burned %d attempts", attempts)
	}
}

// A row a crash cut off mid-send is never sent twice, and a row canceled
// after its batch was read is never sent at all.
func TestQueueLease(t *testing.T) {
	ft := &fakeTransport{}
	q, d, _ := newQueue(t, ft)
	ctx := context.Background()
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "cut@example.org"})
	q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'sending' WHERE rcpt = 'cut@example.org'")
	q.recoverInterrupted(ctx)
	if s, _ := status(t, q, "cut@example.org"); s != "sent" {
		t.Fatalf("interrupted row %s", s)
	}
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "late@example.org"})
	q.Builders[KindMail] = func(_ context.Context, r Row) ([]byte, error) {
		q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'canceled' WHERE id = ?", r.ID)
		return []byte("Subject: x\r\n\r\nx\r\n"), nil
	}
	q.runBatch(ctx)
	if s, _ := status(t, q, "late@example.org"); s != "canceled" || len(ft.sent) != 0 {
		t.Fatalf("canceled row %s, sent %d", s, len(ft.sent))
	}
}

// A person's mail goes out before a newsletter batch queued earlier.
func TestMailBeforeIssues(t *testing.T) {
	ft := &fakeTransport{}
	q, d, _ := newQueue(t, ft)
	q.Builders[KindIssue] = q.Builders[KindMail]
	q.Rate = func(context.Context) (int, int) { return 1000, 1 }
	ctx := context.Background()
	for i := range 5 {
		q.Enqueue(ctx, Item{Kind: KindIssue, DomainID: d.ID, Rcpt: fmt.Sprintf("reader%d@example.org", i)})
	}
	q.Enqueue(ctx, Item{Kind: KindMail, DomainID: d.ID, Rcpt: "patient@example.org"})
	q.runBatch(ctx)
	if len(ft.sent) != 6 || ft.sent[0] != "patient@example.org" {
		t.Fatalf("order %v", ft.sent)
	}
}
