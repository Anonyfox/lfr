package issues

import (
	"context"
	"errors"
	"github.com/Anonyfox/lfr/internal/lists"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/outbound"
)

func TestWelcomeSwitchAndSendingCancellation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, created, err := e.s.Put(ctx, "bakers", Draft{Markdown: "# First welcome", Slug: "first"}, true)
	if err != nil || !created {
		t.Fatalf("first: %v %v", created, err)
	}
	b, created, err := e.s.Put(ctx, "bakers", Draft{Markdown: "# Second welcome", Slug: "second"}, true)
	if err != nil || !created {
		t.Fatalf("second: %v %v", created, err)
	}
	old, err := e.s.Get(ctx, a.ID)
	if err != nil || old.Welcome {
		t.Fatalf("two welcome issues: %+v %v", old, err)
	}
	if _, err := e.s.Schedule(ctx, b.ID, e.now.Add(time.Hour)); err == nil {
		t.Fatal("welcome issue scheduled as a batch")
	}
	// Promoting an existing draft removes the welcome role from its predecessor.
	a, created, err = e.s.Put(ctx, "bakers", Draft{Markdown: "# First welcome", Slug: "first"}, true)
	if err != nil || created || !a.Welcome {
		t.Fatalf("promotion: %+v %v %v", a, created, err)
	}
	b, err = e.s.Get(ctx, b.ID)
	if err != nil || b.Welcome {
		t.Fatalf("old welcome still active: %+v %v", b, err)
	}
	// Demoting it makes an ordinary schedulable draft again.
	a, _, err = e.s.Put(ctx, "bakers", Draft{Markdown: "# First welcome", Slug: "first"}, false)
	if err != nil || a.Welcome {
		t.Fatal(err)
	}
	m, _, err := e.s.Lists.Add(ctx, e.l, "reader@example.org", "", true, lists.Signup{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Schedule(ctx, a.ID, e.now); err != nil {
		t.Fatal(err)
	}
	e.s.Tick(ctx)
	if _, _, err := e.s.Put(ctx, "bakers", Draft{Markdown: "# Changed", Slug: "first"}, false); err == nil {
		t.Fatal("sending issue changed")
	}
	if err := e.s.Remove(ctx, a.ID); err == nil {
		t.Fatal("sending issue deleted")
	}
	canceled, err := e.s.Cancel(ctx, a.ID)
	if err != nil || canceled.Status != "canceled" || canceled.Finished.IsZero() {
		t.Fatalf("cancel: %+v %v", canceled, err)
	}
	st, err := e.s.Stats(ctx, a.ID)
	if err != nil || st.Canceled != 1 || st.Queued != 0 {
		t.Fatalf("queued messages survived cancellation: %+v %v", st, err)
	}
	// Builds also recheck membership: a queued message never overrides a later opt-out.
	if _, err := e.s.Lists.Unsubscribe(ctx, m.ID, 0, "test"); err != nil {
		t.Fatal(err)
	}
	_, err = e.s.Builder(e.r)(ctx, outbound.Row{IssueID: b.ID, MemberID: m.ID, ContactID: m.ContactID, Rcpt: m.Email})
	if !errors.Is(err, outbound.ErrSkip) {
		t.Fatalf("unsubscribed member rendered: %v", err)
	}
}
