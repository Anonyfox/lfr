// Package events records what happens (mail received, subscriptions, sends,
// bounces, opens, clicks) and streams it live to `lfr events --follow`.
package events

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/store"
)

// Kinds of events.
const (
	Received      = "received"     // a message arrived in a mailbox
	Subscribed    = "subscribed"   // a signup is pending confirmation
	Confirmed     = "confirmed"    // a membership was confirmed
	Unsubscribed  = "unsubscribed" // a member left a list
	Sent          = "sent"         // an outbound message was handed over
	Failed        = "failed"       // an outbound message failed for good
	Deferred      = "deferred"     // an outbound attempt failed temporarily
	Bounced       = "bounced"      // a recipient address does not exist
	Complained    = "complained"   // a recipient marked a message as spam
	Opened        = "opened"
	Clicked       = "clicked"
	IssueStarted  = "issue_started"
	IssueFinished = "issue_finished"
	Paused        = "paused" // a list stopped sending, see detail
)

// Event is one record.
type Event struct {
	ID         int64     `json:"id"`
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	ListID     int64     `json:"list_id,omitempty"`
	IssueID    int64     `json:"issue_id,omitempty"`
	ContactID  int64     `json:"contact_id,omitempty"`
	OutboundID int64     `json:"outbound_id,omitempty"`
	MessageID  int64     `json:"message_id,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

// Bus stores events and fans them out to subscribers.
type Bus struct {
	DB  *store.DB
	Now func() time.Time

	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func null(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// Record stores e (stamping its time) and publishes it.
func (b *Bus) Record(ctx context.Context, e Event) error {
	return b.record(ctx, b.DB.W, e)
}

// RecordTx stores e inside tx; it is published once Publish is called after commit.
func (b *Bus) RecordTx(ctx context.Context, tx *sql.Tx, e *Event) error {
	if e.At.IsZero() {
		e.At = b.Now()
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO events(at, kind, list_id, issue_id, contact_id, outbound_id, message_id, detail)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, e.At.Unix(), e.Kind, null(e.ListID), null(e.IssueID), null(e.ContactID), null(e.OutboundID), null(e.MessageID), e.Detail)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (b *Bus) record(ctx context.Context, x execer, e Event) error {
	if e.At.IsZero() {
		e.At = b.Now()
	}
	res, err := x.ExecContext(ctx, `INSERT INTO events(at, kind, list_id, issue_id, contact_id, outbound_id, message_id, detail)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, e.At.Unix(), e.Kind, null(e.ListID), null(e.IssueID), null(e.ContactID), null(e.OutboundID), null(e.MessageID), e.Detail)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	b.Publish(e)
	return nil
}

// Publish sends e to every subscriber that keeps up; slow ones miss events
// rather than stalling the server.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe returns a channel of new events and a function ending the subscription.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[chan Event]struct{}{}
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

// Query selects stored events.
type Query struct {
	Since   time.Time
	Kinds   []string
	ListID  int64
	IssueID int64
	Limit   int
}

// List returns stored events, newest last.
func (b *Bus) List(ctx context.Context, q Query) ([]Event, error) {
	where := "WHERE at >= ?"
	args := []any{q.Since.Unix()}
	if q.ListID != 0 {
		where += " AND list_id = ?"
		args = append(args, q.ListID)
	}
	if q.IssueID != 0 {
		where += " AND issue_id = ?"
		args = append(args, q.IssueID)
	}
	if len(q.Kinds) > 0 {
		where += " AND kind IN (?" + repeat(",?", len(q.Kinds)-1) + ")"
		for _, k := range q.Kinds {
			args = append(args, k)
		}
	}
	if q.Limit <= 0 || q.Limit > 10000 {
		q.Limit = 100
	}
	args = append(args, q.Limit)
	rows, err := b.DB.R.QueryContext(ctx, `SELECT id, at, kind, coalesce(list_id, 0), coalesce(issue_id, 0), coalesce(contact_id, 0),
		coalesce(outbound_id, 0), coalesce(message_id, 0), detail FROM (SELECT * FROM events `+where+` ORDER BY id DESC LIMIT ?) ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Kind, &e.ListID, &e.IssueID, &e.ContactID, &e.OutboundID, &e.MessageID, &e.Detail); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
