package mailbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/store"
)

func TestRenamePreservesMailAndDomainBoundaries(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	mb, err := s.Add(ctx, "jo@example.com", "Jo")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddAlias(ctx, "hello@example.com", mb.Address); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCatchall(ctx, "example.com", mb.Address); err != nil {
		t.Fatal(err)
	}
	message, err := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Raw: raw("a@reader.org", mb.Address, "Keep this", "keep@reader.org", "", "Permanent mail")})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := s.Rename(ctx, mb.Address, "joseph@example.com")
	if err != nil || renamed.ID != mb.ID {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	if id, ok, err := s.Resolve(ctx, "hello@example.com"); err != nil || !ok || id != mb.ID {
		t.Fatalf("same-domain alias lost: %d %v %v", id, ok, err)
	}
	for _, target := range []string{"invalid", "dmarc-reports@example.com", "postmaster@example.com", "x@missing.org"} {
		if _, err := s.Rename(ctx, renamed.Address, target); err == nil {
			t.Errorf("accepted target %q", target)
		}
	}
	moved, err := s.Rename(ctx, renamed.Address, "jo@other.org")
	if err != nil || moved.ID != mb.ID {
		t.Fatalf("cross-domain rename: %+v %v", moved, err)
	}
	for _, address := range []string{"hello@example.com", "nobody@example.com", "joseph@example.com"} {
		if _, ok, err := s.Resolve(ctx, address); err != nil || ok {
			t.Errorf("old domain still routes %s: %v %v", address, ok, err)
		}
	}
	got, err := s.Read(ctx, message, false)
	if err != nil || got.Mailbox != "jo@other.org" || got.Text != "Permanent mail" {
		t.Fatalf("mail lost: %+v %v", got, err)
	}
	if err := s.AddAlias(ctx, "hi@other.org", moved.Address); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAlias(ctx, "hi@other.org"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Resolve(ctx, "hi@other.org"); err != nil || ok {
		t.Fatalf("removed alias resolves: %v %v", ok, err)
	}
	if err := s.RemoveAlias(ctx, "hi@other.org"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing alias: %v", err)
	}
}

func TestRetentionPurgesOnlyOldMessagesInChosenFolder(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	mb, err := s.Add(ctx, "jo@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		folder string
		age    time.Duration
	}{{"trash", 48 * time.Hour}, {"trash", time.Hour}, {"inbox", 48 * time.Hour}, {"junk", 48 * time.Hour}} {
		current := s.Now()
		s.Now = func() time.Time { return current.Add(-tc.age) }
		if _, err := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Folder: tc.folder, Raw: raw("a@reader.org", mb.Address, tc.folder, "id@reader.org", "", tc.age.String())}); err != nil {
			t.Fatal(err)
		}
		s.Now = func() time.Time { return current }
	}
	n, err := s.Purge(ctx, "trash", 24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purged %d: %v", n, err)
	}
	counts, err := s.Counts(ctx, mb.ID)
	if err != nil || counts["trash"] != 1 || counts["junk"] != 1 || counts["inbox"] != 1 {
		t.Fatalf("retention affected other mail: %v %v", counts, err)
	}
	n, err = s.Purge(ctx, "trash", 24*time.Hour)
	if err != nil || n != 0 {
		t.Fatalf("repeat purge: %d %v", n, err)
	}
}
