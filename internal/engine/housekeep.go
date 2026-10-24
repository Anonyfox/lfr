package engine

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/mailbox"
)

// housekeep prunes old data. It runs at start and every hour.
func (e *Engine) housekeep(ctx context.Context) {
	now := e.Opts.Now()
	for _, f := range []struct{ folder, key string }{{"trash", "retention.trash_days"}, {"junk", "retention.junk_days"}} {
		days := e.Config.Int(ctx, f.key)
		if n, err := e.Mailbox.Purge(ctx, f.folder, time.Duration(days)*24*time.Hour); err != nil {
			e.Log.Warn("purge", "folder", f.folder, "err", err)
		} else if n > 0 {
			e.Log.Info("purged", "folder", f.folder, "messages", n)
		}
	}
	cutoff := now.AddDate(0, 0, -e.Config.Int(ctx, "retention.events_days")).Unix()
	// Consent history (subscribed, confirmed, unsubscribed, complained) stays; activity ages out.
	e.DB.W.ExecContext(ctx, `DELETE FROM events WHERE at < ? AND kind IN ('opened', 'clicked', 'sent', 'deferred', 'failed', 'bounced', 'received', 'issue_started', 'issue_finished')`, cutoff)
	e.DB.W.ExecContext(ctx, `DELETE FROM outbound WHERE kind IN ('confirm', 'mail') AND status NOT IN ('queued', 'sending') AND created < ?`, now.AddDate(0, 0, -90).Unix())
	// Signups never confirmed are no consent; their IP and browser go once
	// the link expired (GDPR Art. 5(1)(e)).
	e.DB.W.ExecContext(ctx, `DELETE FROM members WHERE status = 'pending' AND max(confirm_sent, signup_at) < ?`, now.Add(-lists.ConfirmValidity).Unix())
	if _, err := e.DB.W.ExecContext(ctx, `UPDATE members SET signup_ip = '', signup_ua = '', confirm_ip = ''
		WHERE status = 'unsubscribed' AND unsub_at < ?
		AND (signup_ip != '' OR signup_ua != '' OR confirm_ip != '')`,
		now.AddDate(0, 0, -e.Config.Int(ctx, "retention.unsubscribed_days")).Unix()); err != nil {
		e.Log.Warn("clear unsubscribed member metadata", "err", err)
	}
	if n, err := e.Issues.Archive(ctx, time.Unix(cutoff, 0)); err != nil {
		e.Log.Warn("archive issues", "err", err)
	} else if n > 0 {
		e.Log.Info("archived issue statistics", "issues", n)
	}
	e.sweepBlobs(ctx, now)
	e.DB.W.ExecContext(ctx, "PRAGMA optimize")
}

// sweepBlobs removes stored messages nothing refers to anymore. Fresh files
// are spared: a delivery may be between writing the file and its row.
func (e *Engine) sweepBlobs(ctx context.Context, now time.Time) {
	removed := 0
	e.Blobs.Walk(func(hash string) error {
		p := filepath.Join(e.Opts.Data, BlobDir, hash[:2], hash+".eml")
		if info, err := os.Stat(p); err != nil || now.Sub(info.ModTime()) < 2*time.Hour {
			return nil
		}
		if n, err := mailbox.BlobRefs(ctx, e.DB.R, hash); err == nil && n == 0 {
			e.Blobs.Remove(hash)
			removed++
		}
		return nil
	})
	if removed > 0 {
		e.Log.Info("removed unreferenced messages", "count", removed)
	}
}
