// Package outbound is the delivery queue: every message lfr sends is a row
// that survives restarts, is built and DKIM-signed at send time, handed to a
// relay or straight to the recipient's MX, and retried with backoff until it
// is delivered, refused for good or five days old.
package outbound

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/store"
)

// Kinds of outbound rows.
const (
	KindIssue   = "issue"   // one recipient of a newsletter issue
	KindConfirm = "confirm" // a double opt-in confirmation request
	KindMail    = "mail"    // a message sent from a mailbox (its raw content is a blob)
)

// Item is a message to queue.
type Item struct {
	Kind      string
	DomainID  int64
	IssueID   int64
	ContactID int64
	MemberID  int64
	Blob      string
	EnvFrom   string // empty: a VERP bounce address on the signing domain
	Rcpt      string
	At        time.Time // earliest send time; zero for now
}

// Row is a queued message being sent.
type Row struct {
	ID        int64
	Kind      string
	DomainID  int64
	IssueID   int64
	ContactID int64
	MemberID  int64
	Blob      string
	EnvFrom   string
	Rcpt      string
	Attempts  int
	Created   time.Time
}

// Builder renders the unsigned message for a row. Returning an error wrapping
// ErrSkip cancels the row (e.g. the recipient unsubscribed meanwhile).
type Builder func(ctx context.Context, r Row) ([]byte, error)

// ErrSkip cancels a row instead of sending it.
var ErrSkip = errors.New("skip")

// Hooks react to final outcomes, e.g. to suppress a bounced address.
type Hooks struct {
	// Failed runs when delivery failed for good; f.Bounce says the address
	// does not exist.
	Failed func(ctx context.Context, r Row, f *Failure)
	// GaveUp runs when temporary failures outlasted MaxAge: a soft bounce.
	GaveUp func(ctx context.Context, r Row, reason string)
	Sent   func(ctx context.Context, r Row)
}

// Queue runs deliveries.
type Queue struct {
	DB        *store.DB
	Domains   *domains.Service
	Events    *events.Bus
	Builders  map[string]Builder
	Hooks     Hooks
	Bounce    func(r Row, domain string) string                              // VERP envelope sender
	Transport func(ctx context.Context) (t Transport, key string, err error) // key changes when the configuration does
	Rate      func(ctx context.Context) (perSecond, conns int)
	Now       func() time.Time
	Log       *slog.Logger
	MaxAge    time.Duration // give up after this long; default 5 days

	wake     chan struct{}
	initOnce sync.Once

	tmu       sync.Mutex
	transport Transport
	down      string // why the transport refuses everything, empty while it works
	downSince time.Time
	tkey      string
}

func (q *Queue) init() {
	q.initOnce.Do(func() {
		q.wake = make(chan struct{}, 1)
		if q.MaxAge == 0 {
			q.MaxAge = 5 * 24 * time.Hour
		}
	})
}

// Wake makes the queue look for due rows now.
func (q *Queue) Wake() {
	q.init()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Execer is a transaction or database that runs statements.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func null(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// EnqueueTx adds items inside a transaction; call Wake after commit. Issue rows
// already queued for the same contact are skipped.
func (q *Queue) EnqueueTx(ctx context.Context, tx Execer, items ...Item) (int, error) {
	n := 0
	now := q.Now()
	for _, it := range items {
		at := it.At
		if at.IsZero() {
			at = now
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO outbound(kind, domain_id, issue_id, contact_id, member_id, blob, env_from, rcpt, status, next_at, created)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?) ON CONFLICT DO NOTHING`,
			it.Kind, it.DomainID, null(it.IssueID), null(it.ContactID), null(it.MemberID), it.Blob, it.EnvFrom, strings.ToLower(it.Rcpt), at.Unix(), now.Unix())
		if err != nil {
			return n, err
		}
		k, _ := res.RowsAffected()
		n += int(k)
	}
	return n, nil
}

// Enqueue adds items and wakes the queue.
func (q *Queue) Enqueue(ctx context.Context, items ...Item) error {
	err := q.DB.Tx(ctx, func(tx *sql.Tx) error {
		_, err := q.EnqueueTx(ctx, tx, items...)
		return err
	})
	if err == nil {
		q.Wake()
	}
	return err
}

// Run delivers until ctx ends.
func (q *Queue) Run(ctx context.Context) error {
	q.init()
	q.recoverInterrupted(ctx)
	defer func() {
		q.tmu.Lock()
		if q.transport != nil {
			q.transport.Close()
		}
		q.tmu.Unlock()
	}()
	for {
		n, next, err := q.runBatch(ctx)
		if err != nil && ctx.Err() == nil {
			q.Log.Error("outbound batch", "err", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		if n > 0 {
			continue
		}
		wait := 30 * time.Second
		if !next.IsZero() {
			if d := next.Sub(q.Now()); d < wait {
				wait = max(d, 100*time.Millisecond)
			}
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-q.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

func (q *Queue) currentTransport(ctx context.Context) (Transport, error) {
	t, key, err := q.Transport(ctx)
	if err != nil {
		return nil, err
	}
	q.tmu.Lock()
	defer q.tmu.Unlock()
	// Keep one transport while its configuration is unchanged, so relay connections are reused.
	if q.transport != nil && q.tkey == key {
		t.Close()
		return q.transport, nil
	}
	if q.transport != nil {
		q.transport.Close()
	}
	q.transport, q.tkey = t, key
	return t, nil
}

// recoverInterrupted settles rows a crash cut off mid-transaction. The
// receiver may already have them, and resending risks a duplicate in every
// such inbox, so they count as sent with a note.
func (q *Queue) recoverInterrupted(ctx context.Context) {
	q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'sent', sent_at = next_at, last_error = 'interrupted while sending; delivery unconfirmed' WHERE status = 'sending'")
}

// runBatch sends due rows and returns how many it handled and when the next
// row is due.
func (q *Queue) runBatch(ctx context.Context) (int, time.Time, error) {
	now := q.Now()
	rows, err := q.DB.R.QueryContext(ctx, `SELECT o.id, o.kind, o.domain_id, coalesce(o.issue_id, 0), coalesce(o.contact_id, 0), coalesce(o.member_id, 0),
		o.blob, o.env_from, o.rcpt, o.attempts, o.created
		FROM outbound o LEFT JOIN issues w ON w.id = o.issue_id LEFT JOIN lists l ON l.id = w.list_id
		WHERE o.status = 'queued' AND o.next_at <= ? AND coalesce(l.paused, '') = ''
		ORDER BY o.kind = 'issue', o.next_at, o.id LIMIT 200`, now.Unix()) // a person's mail and confirmations never wait behind an issue
	if err != nil {
		return 0, time.Time{}, err
	}
	var batch []Row
	for rows.Next() {
		var r Row
		var created int64
		if err := rows.Scan(&r.ID, &r.Kind, &r.DomainID, &r.IssueID, &r.ContactID, &r.MemberID, &r.Blob, &r.EnvFrom, &r.Rcpt, &r.Attempts, &created); err != nil {
			rows.Close()
			return 0, time.Time{}, err
		}
		r.Created = time.Unix(created, 0)
		batch = append(batch, r)
	}
	rows.Close()
	if len(batch) == 0 {
		var next sql.NullInt64
		q.DB.R.QueryRowContext(ctx, `SELECT min(o.next_at) FROM outbound o LEFT JOIN issues w ON w.id = o.issue_id LEFT JOIN lists l ON l.id = w.list_id
			WHERE o.status = 'queued' AND coalesce(l.paused, '') = ''`).Scan(&next)
		if next.Valid {
			return 0, time.Unix(next.Int64, 0), nil
		}
		return 0, time.Time{}, nil
	}
	t, err := q.currentTransport(ctx)
	if err != nil {
		// No usable transport: retry everything later without counting attempts.
		q.Log.Error("outbound transport", "err", err)
		q.setDown(err.Error())
		for _, r := range batch {
			q.DB.W.ExecContext(ctx, "UPDATE outbound SET next_at = ?, last_error = ? WHERE id = ?", now.Add(5*time.Minute).Unix(), err.Error(), r.ID)
		}
		return 0, now.Add(5 * time.Minute), nil
	}
	perSecond, conns := q.Rate(ctx)
	limiter := newLimiter(perSecond, q.Now)
	jobs := make(chan Row)
	var wg sync.WaitGroup
	var down atomic.Bool // the transport failed as a whole: stop this batch
	for i := 0; i < max(conns, 1); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				if down.Load() {
					continue
				}
				if q.deliver(ctx, t, r) {
					down.Store(true)
				}
			}
		}()
	}
	sent := 0
	for _, r := range batch {
		if down.Load() || limiter.Wait(ctx) != nil {
			break
		}
		select {
		case jobs <- r:
			sent++
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if down.Load() {
		// Wait for the operator or the relay instead of spinning through the queue.
		return 0, now.Add(time.Minute), nil
	}
	return sent, time.Time{}, nil
}

func (q *Queue) domain(ctx context.Context, id int64) (*domains.Domain, error) {
	return q.Domains.ByID(ctx, id)
}

// deliver builds, signs and sends one row and records the outcome. It
// reports whether the transport itself failed, which stops the batch.
func (q *Queue) deliver(ctx context.Context, t Transport, r Row) (transportDown bool) {
	d, err := q.domain(ctx, r.DomainID)
	if err != nil {
		q.finish(ctx, r, "failed", "signing domain is gone: "+err.Error())
		return
	}
	build := q.Builders[r.Kind]
	if build == nil {
		q.finish(ctx, r, "failed", "no builder for "+r.Kind)
		return
	}
	raw, err := build(ctx, r)
	if errors.Is(err, ErrSkip) {
		q.finish(ctx, r, "canceled", err.Error())
		return
	}
	if err != nil {
		q.retry(ctx, r, &Failure{Temporary: true, Message: "build: " + err.Error()})
		return
	}
	headers := mailauth.SignedHeaders
	if r, ok := t.(interface{ Relayed() bool }); ok && r.Relayed() {
		headers = mailauth.RelayHeaders
	}
	signed, err := mailauth.SignHeaders(raw, d.Name, []mailauth.Key{{Selector: d.EdSelector(), Signer: d.Signers()[0]}, {Selector: d.RSASelector(), Signer: d.Signers()[1]}}, q.Now(), headers)
	if err != nil {
		q.retry(ctx, r, &Failure{Temporary: true, Message: err.Error()})
		return
	}
	from := r.EnvFrom
	if from == "" && q.Bounce != nil {
		from = q.Bounce(r, d.Name)
	}
	// Lease the row. A row canceled, or whose list paused, since the batch
	// was read stays unsent.
	res, err := q.DB.W.ExecContext(ctx, `UPDATE outbound SET status = 'sending', next_at = ? WHERE id = ? AND status = 'queued'
		AND NOT EXISTS (SELECT 1 FROM issues w JOIN lists l ON l.id = w.list_id WHERE w.id = outbound.issue_id AND l.paused != '')`, q.Now().Unix(), r.ID)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	err = t.Send(sctx, from, r.Rcpt, signed)
	cancel()
	// The outcome is recorded even while shutting down: a message the
	// receiver took must never be sent again.
	ctx = context.WithoutCancel(ctx)
	if err == nil {
		q.setDown("")
		q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'sent', sent_at = ?, attempts = attempts + 1, last_error = '' WHERE id = ?", q.Now().Unix(), r.ID)
		if r.Kind == KindMail {
			q.Events.Record(ctx, events.Event{Kind: events.Sent, OutboundID: r.ID, ContactID: r.ContactID, Detail: r.Rcpt})
		}
		if q.Hooks.Sent != nil {
			q.Hooks.Sent(ctx, r)
		}
		return
	}
	f := Classify(err)
	if f.Transport {
		// Not this message's fault: keep its attempts and age, try again soon.
		q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'queued', next_at = ?, last_error = ? WHERE id = ?", q.Now().Add(time.Minute).Unix(), f.Error(), r.ID)
		q.Log.Error("outbound transport down", "transport", t.Name(), "err", f.Error())
		q.setDown(f.Error())
		return true
	}
	if f.Temporary {
		q.retry(ctx, r, f)
		return
	}
	status := "failed"
	if f.Bounce {
		status = "bounced"
	}
	q.finish(ctx, r, status, f.Error())
	if q.Hooks.Failed != nil {
		q.Hooks.Failed(ctx, r, f)
	}
	return false
}

// Backoff is the wait before attempt n+1.
func Backoff(attempts int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour}
	if attempts < len(steps) {
		return steps[attempts]
	}
	return 6 * time.Hour
}

func (q *Queue) retry(ctx context.Context, r Row, f *Failure) {
	now := q.Now()
	if now.Sub(r.Created) > q.MaxAge {
		reason := "gave up after " + q.MaxAge.String() + ": " + f.Error()
		q.finish(ctx, r, "failed", reason)
		if q.Hooks.GaveUp != nil {
			q.Hooks.GaveUp(ctx, r, reason)
		}
		return
	}
	next := now.Add(Backoff(r.Attempts))
	q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'queued', attempts = attempts + 1, next_at = ?, last_error = ? WHERE id = ?", next.Unix(), f.Error(), r.ID)
	q.Log.Warn("delivery deferred", "id", r.ID, "rcpt", r.Rcpt, "attempt", r.Attempts+1, "err", f.Error())
	if r.Attempts == 0 || r.Kind == KindMail {
		q.Events.Record(ctx, events.Event{Kind: events.Deferred, OutboundID: r.ID, IssueID: r.IssueID, ContactID: r.ContactID, Detail: r.Rcpt + ": " + f.Error()})
	}
}

func (q *Queue) finish(ctx context.Context, r Row, status, reason string) {
	q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = ?, attempts = attempts + 1, last_error = ? WHERE id = ?", status, reason, r.ID)
	if status == "canceled" {
		return
	}
	kind := events.Failed
	if status == "bounced" {
		kind = events.Bounced
	}
	q.Log.Warn("delivery "+status, "id", r.ID, "rcpt", r.Rcpt, "reason", reason)
	q.Events.Record(ctx, events.Event{Kind: kind, OutboundID: r.ID, IssueID: r.IssueID, ContactID: r.ContactID, Detail: r.Rcpt + ": " + reason})
}

// limiter spaces sends evenly.
type limiter struct {
	interval time.Duration
	next     time.Time
	now      func() time.Time
	mu       sync.Mutex
}

func newLimiter(perSecond int, now func() time.Time) *limiter {
	return &limiter{interval: time.Second / time.Duration(max(perSecond, 1)), now: now}
}

func (l *limiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	wait := l.next.Sub(now)
	l.next = l.next.Add(l.interval)
	l.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Status summarizes the queue.
type Status struct {
	Queued   int    `json:"queued"`
	Retrying int    `json:"retrying"`
	Sent24h  int    `json:"sent_24h"`
	Failed24 int    `json:"failed_24h"`
	Bounce24 int    `json:"bounced_24h"`
	Oldest   string `json:"oldest_queued,omitempty"`
	// Down says why the relay or direct delivery refuses everything, and
	// since when; mail waits without using up its attempts.
	Down      string `json:"transport_down,omitempty"`
	DownSince string `json:"transport_down_since,omitempty"`
}

func (q *Queue) setDown(why string) {
	q.tmu.Lock()
	defer q.tmu.Unlock()
	if why != "" && q.down == "" {
		q.downSince = q.Now()
	}
	q.down = why
}

// Stats reports queue counts.
func (q *Queue) Stats(ctx context.Context) (Status, error) {
	var s Status
	day := q.Now().Add(-24 * time.Hour).Unix()
	var oldest sql.NullInt64
	err := q.DB.R.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM outbound WHERE status = 'queued'),
		(SELECT count(*) FROM outbound WHERE status = 'queued' AND attempts > 0),
		(SELECT count(*) FROM outbound WHERE status = 'sent' AND sent_at >= ?1),
		(SELECT count(*) FROM outbound WHERE status = 'failed' AND created >= ?1),
		(SELECT count(*) FROM outbound WHERE status = 'bounced' AND created >= ?1),
		(SELECT min(created) FROM outbound WHERE status = 'queued')`, day).Scan(&s.Queued, &s.Retrying, &s.Sent24h, &s.Failed24, &s.Bounce24, &oldest)
	if oldest.Valid {
		s.Oldest = time.Unix(oldest.Int64, 0).UTC().Format(time.RFC3339)
	}
	q.tmu.Lock()
	if q.down != "" {
		s.Down, s.DownSince = q.down, q.downSince.UTC().Format(time.RFC3339)
	}
	q.tmu.Unlock()
	return s, err
}

// Entry is one queue row as listed.
type Entry struct {
	ID       int64     `json:"id"`
	Kind     string    `json:"kind"`
	Rcpt     string    `json:"rcpt"`
	Status   string    `json:"status"`
	Attempts int       `json:"attempts"`
	Next     time.Time `json:"next"`
	Error    string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
}

// List shows rows of a status ("queued" by default).
func (q *Queue) List(ctx context.Context, status string, limit int) ([]Entry, error) {
	if status == "" {
		status = "queued"
	}
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	rows, err := q.DB.R.QueryContext(ctx, `SELECT id, kind, rcpt, status, attempts, next_at, last_error, created FROM outbound
		WHERE status = ? ORDER BY id DESC LIMIT ?`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var next, created int64
		if err := rows.Scan(&e.ID, &e.Kind, &e.Rcpt, &e.Status, &e.Attempts, &next, &e.Error, &created); err != nil {
			return nil, err
		}
		e.Next, e.Created = time.Unix(next, 0).UTC(), time.Unix(created, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// Retry makes queued rows (one, or all when id is 0) due now; failed rows are requeued too.
func (q *Queue) Retry(ctx context.Context, id int64) (int, error) {
	now := q.Now().Unix()
	var res sql.Result
	var err error
	if id == 0 {
		res, err = q.DB.W.ExecContext(ctx, "UPDATE outbound SET next_at = ? WHERE status = 'queued'", now)
	} else {
		res, err = q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'queued', next_at = ?, created = CASE WHEN status = 'failed' THEN ? ELSE created END WHERE id = ? AND status IN ('queued', 'failed')", now, now, id)
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if id != 0 && n == 0 {
		return 0, fmt.Errorf("queue entry %d: %w (only queued or failed entries can be retried)", id, store.ErrNotFound)
	}
	q.Wake()
	return int(n), nil
}

// Drop cancels a queued row.
func (q *Queue) Drop(ctx context.Context, id int64) error {
	res, err := q.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'canceled', last_error = 'dropped by operator' WHERE id = ? AND status = 'queued'", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("queued entry %d: %w", id, store.ErrNotFound)
	}
	return nil
}
