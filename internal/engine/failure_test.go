package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/feedback"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/outbound"
)

func TestExhaustedPersonalDeliveryFilesAnOfflineDSN(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	h.call(t, api.OpDomainAdd, api.Name{Name: "example.com"}, nil)
	h.call(t, api.OpMailboxAdd, api.MailboxAdd{Address: "jo@example.com", Name: "Jo"}, nil)
	raw := []byte("From: jo@example.com\r\nTo: full@reader.org\r\nSubject: Meeting\r\n\r\nPrivate message body")
	hash, err := h.e.Blobs.Put(raw)
	if err != nil {
		t.Fatal(err)
	}
	// The queue invokes this callback after its bounded retry lifetime. Invoke
	// that boundary directly rather than sleeping for five days.
	h.e.onGiveUp(ctx, outbound.Row{Kind: outbound.KindMail, EnvFrom: "jo@example.com", Rcpt: "full@reader.org", Blob: hash, Created: h.e.Opts.Now()}, "451 mailbox full\r\ntry later")
	var messages []mailbox.Summary
	h.call(t, api.OpMailList, api.MailList{Mailbox: "jo@example.com"}, &messages)
	if len(messages) != 1 || !strings.Contains(messages[0].Subject, "Undelivered mail") {
		t.Fatalf("sender not notified: %+v", messages)
	}
	notice, err := h.e.Mailbox.Raw(ctx, messages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notice), "Status: 4.4.7") || !strings.Contains(string(notice), "Auto-Submitted: auto-replied") || strings.Contains(string(notice), "Private message body") {
		t.Fatalf("unsafe DSN: %s", notice)
	}
	var queued struct{ Entries []outbound.Entry }
	h.call(t, api.OpQueueList, api.QueueList{}, &queued)
	if len(queued.Entries) != 0 {
		t.Fatal("delivery notice was enqueued and can bounce")
	}
}

func TestControlMasksRelaySecretsAndPinsSNSTopic(t *testing.T) {
	h := start(t)
	ctx := context.Background()
	h.call(t, api.OpConfigSet, api.ConfigSet{Name: "relay.password", Value: "private-secret"}, nil)
	var entry json.RawMessage
	h.call(t, api.OpConfigGet, api.Name{Name: "relay.password"}, &entry)
	if strings.Contains(string(entry), "private-secret") || !strings.Contains(string(entry), "(set)") {
		t.Fatalf("secret exposed: %s", entry)
	}
	h.call(t, api.OpConfigSet, api.ConfigSet{Name: "acme.email", Value: "owner@example.com"}, nil)
	if h.e.snsTopic(ctx, "arn:topic:attacker", false) {
		t.Fatal("unpinned notification accepted")
	}
	if !h.e.snsTopic(ctx, "arn:topic:owner", true) || !h.e.snsTopic(ctx, "arn:topic:owner", false) || h.e.snsTopic(ctx, "arn:topic:attacker", true) {
		t.Fatal("SNS topic pinning failed")
	}
	// Unmatched webhook recipients must never become suppressed contacts.
	h.e.onWebhook(ctx, []feedback.Event{{Kind: "complaint", Email: "unknown@reader.org"}, {Kind: "bounce", Email: "unknown@reader.org", Hard: true, Source: "bounce+forged@example.com"}})
	var count int
	if err := h.e.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM contacts WHERE email = 'unknown@reader.org'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unmatched feedback created contact: %d %v", count, err)
	}
}
