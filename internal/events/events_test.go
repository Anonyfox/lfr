package events_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/store"
)

func TestDurableLogAndTransactionalPublication(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	bus := events.Bus{DB: db, Now: func() time.Time { return now }}
	ch, stop := bus.Subscribe()
	defer stop()
	if err := bus.Record(ctx, events.Event{Kind: events.Received, Detail: "first"}); err != nil {
		t.Fatal(err)
	}
	first := <-ch
	if first.ID == 0 || !first.At.Equal(now) {
		t.Fatalf("unstamped event: %+v", first)
	}
	tx, err := db.W.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	event := events.Event{Kind: events.Sent, Detail: "committed", At: now.Add(time.Hour)}
	if err := bus.RecordTx(ctx, tx, &event); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		t.Fatalf("published before commit: %+v", e)
	default:
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	bus.Publish(event)
	if got := <-ch; got.ID != event.ID {
		t.Fatalf("wrong publication: %+v", got)
	}
	tx, err = db.W.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	rolled := events.Event{Kind: events.Failed}
	if err := bus.RecordTx(ctx, tx, &rolled); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		q       events.Query
		details []string
	}{
		{"all", events.Query{}, []string{"first", "committed"}},
		{"newest", events.Query{Limit: 1}, []string{"committed"}},
		{"kinds", events.Query{Kinds: []string{events.Sent, events.Failed}}, []string{"committed"}},
		{"since", events.Query{Since: now.Add(time.Minute)}, []string{"committed"}},
		{"list", events.Query{ListID: 999}, nil},
		{"issue", events.Query{IssueID: 999}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bus.List(ctx, tc.q)
			if err != nil || len(got) != len(tc.details) {
				t.Fatalf("query: %+v %v", got, err)
			}
			for i, e := range got {
				if e.Detail != tc.details[i] {
					t.Fatalf("order/filter: %+v", got)
				}
			}
		})
	}
}

func TestSlowSubscriberDoesNotBlockAndStopIsIdempotent(t *testing.T) {
	bus := events.Bus{}
	slow, stop := bus.Subscribe()
	fast, done := bus.Subscribe()
	defer done()
	for i := 0; i < 300; i++ {
		bus.Publish(events.Event{ID: int64(i + 1)})
		if got := <-fast; got.ID != int64(i+1) {
			t.Fatalf("fast subscriber: %+v", got)
		}
	}
	stop()
	stop()
	n := 0
	for range slow {
		n++
	}
	if n != 256 {
		t.Fatalf("buffer bound: %d", n)
	}
	bus.Publish(events.Event{ID: 301})
	if got := <-fast; got.ID != 301 {
		t.Fatal("stopping slow subscriber affected fast subscriber")
	}
}
