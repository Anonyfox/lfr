package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/config"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/issues"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/store"
)

// op adapts a typed handler.
func op[A any](fn func(context.Context, A) (any, error)) ctl.Handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var a A
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, &ctl.Error{Code: ctl.CodeInvalid, Msg: "bad arguments: " + err.Error()}
			}
		}
		return fn(ctx, a)
	}
}

func invalid(format string, args ...any) error {
	return &ctl.Error{Code: ctl.CodeInvalid, Msg: fmt.Sprintf(format, args...)}
}

func (e *Engine) register() {
	h := e.Ctl.Handle
	h(api.OpStatus, op(func(ctx context.Context, _ struct{}) (any, error) { return e.Status(ctx) }))
	h(api.OpDoctor, op(func(ctx context.Context, _ struct{}) (any, error) { return e.Doctor(ctx), nil }))
	h(api.OpConfig, op(func(ctx context.Context, _ struct{}) (any, error) { return e.Config.All(ctx) }))
	h(api.OpConfigGet, op(func(ctx context.Context, a api.Name) (any, error) {
		k, ok := config.Lookup(a.Name)
		if !ok {
			return nil, invalid("unknown setting %q; settings are %s", a.Name, strings.Join(config.Names(), ", "))
		}
		v, err := e.Config.Get(ctx, a.Name)
		if k.Secret && v != "" {
			v = "(set)"
		}
		return config.Entry{Name: a.Name, Value: v, Help: k.Help}, err
	}))
	h(api.OpConfigSet, op(func(ctx context.Context, a api.ConfigSet) (any, error) {
		if _, ok := config.Lookup(a.Name); !ok {
			return nil, invalid("unknown setting %q; settings are %s", a.Name, strings.Join(config.Names(), ", "))
		}
		if err := e.Config.Set(ctx, a.Name, a.Value); err != nil {
			return nil, invalid("%v", err)
		}
		if strings.HasPrefix(a.Name, "acme.") || a.Name == "hostname" {
			e.Certs.Forget()
		}
		return e.Config.All(ctx)
	}))

	// Domains.
	h(api.OpDomainAdd, op(func(ctx context.Context, a api.Name) (any, error) {
		d, err := e.Domains.Add(ctx, a.Name)
		if err != nil {
			return nil, err
		}
		e.Certs.Forget()
		return e.domainInfo(ctx, d, false)
	}))
	h(api.OpDomainList, op(func(ctx context.Context, _ struct{}) (any, error) {
		ds, err := e.Domains.List(ctx)
		out := []api.DomainInfo{}
		for _, d := range ds {
			out = append(out, api.DomainInfo{Name: d.Name, Selector: d.Selector, Catchall: d.Catchall, Created: d.Created})
		}
		return out, err
	}))
	h(api.OpDomainRemove, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Domains.Remove(ctx, a.Name) }))
	h(api.OpDomainDNS, op(func(ctx context.Context, a api.DomainDNS) (any, error) {
		d, err := e.Domains.Get(ctx, a.Name)
		if err != nil {
			return nil, err
		}
		return e.domainInfo(ctx, d, a.Check)
	}))
	h(api.OpDomainRotate, op(func(ctx context.Context, a api.Name) (any, error) {
		d, err := e.Domains.Get(ctx, a.Name)
		if err != nil {
			return nil, err
		}
		if d.Next == nil {
			if d, err = e.Domains.PrepareRotation(ctx, a.Name); err != nil {
				return nil, err
			}
			return e.domainInfo(ctx, d, false)
		}
		// Switch only once both new keys resolve exactly; signing with a
		// key receivers cannot find fails DKIM for every message.
		inst := e.Instance(ctx)
		var next []domains.Record
		for _, r := range domains.Records(d, inst, nil) {
			if strings.HasPrefix(r.Name, d.Next.Selector) {
				next = append(next, r)
			}
		}
		for _, c := range e.checkRecords(ctx, next) {
			if c.Status != "ok" {
				return nil, invalid("%s is %s in DNS; publish it, wait for it to resolve (lfr domain dns %s --check), then run lfr domain rotate %s again", c.Record.Name, c.Status, d.Name, d.Name)
			}
		}
		if d, err = e.Domains.Rotate(ctx, a.Name); err != nil {
			return nil, err
		}
		return e.domainInfo(ctx, d, false)
	}))

	// Mailboxes.
	h(api.OpMailboxAdd, op(func(ctx context.Context, a api.MailboxAdd) (any, error) { return e.Mailbox.Add(ctx, a.Address, a.Name) }))
	h(api.OpMailboxList, op(func(ctx context.Context, a api.Name) (any, error) { return nonNil(e.Mailbox.List(ctx, a.Name)) }))
	h(api.OpMailboxRemove, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Mailbox.Remove(ctx, a.Name) }))
	h(api.OpMailboxRename, op(func(ctx context.Context, a api.Rename) (any, error) { return e.Mailbox.Rename(ctx, a.From, a.To) }))
	h(api.OpMailboxSet, op(func(ctx context.Context, a api.MailboxSet) (any, error) {
		return e.Mailbox.Update(ctx, a.Address, a.Name, a.Forward)
	}))
	h(api.OpAliasAdd, op(func(ctx context.Context, a api.Alias) (any, error) {
		return nil, e.Mailbox.AddAlias(ctx, a.Alias, a.Target)
	}))
	h(api.OpAliasRemove, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Mailbox.RemoveAlias(ctx, a.Name) }))
	h(api.OpCatchall, op(func(ctx context.Context, a api.Catchall) (any, error) {
		return nil, e.Mailbox.SetCatchall(ctx, a.Domain, a.Target)
	}))

	// Messages.
	h(api.OpMailList, op(func(ctx context.Context, a api.MailList) (any, error) {
		return nonNil(e.Mailbox.Messages(ctx, mailbox.Query{Mailbox: a.Mailbox, Folder: a.Folder, Unread: a.Unread, Thread: a.Thread, Search: a.Search, Limit: a.Limit, Before: a.Before}))
	}))
	h(api.OpMailRead, op(func(ctx context.Context, a api.MailRead) (any, error) {
		if a.Raw {
			raw, err := e.Mailbox.Raw(ctx, a.ID)
			return map[string]string{"raw": string(raw)}, err
		}
		return e.Mailbox.Read(ctx, a.ID, !a.KeepUnread)
	}))
	h(api.OpMailMove, op(func(ctx context.Context, a api.MailMove) (any, error) { return e.moveMessages(ctx, a) }))
	h(api.OpMailFlag, op(func(ctx context.Context, a api.MailFlag) (any, error) {
		for _, id := range a.IDs {
			if err := e.Mailbox.Flag(ctx, id, a.Flag, a.On); err != nil {
				return nil, err
			}
		}
		return map[string]int{"changed": len(a.IDs)}, nil
	}))
	h(api.OpMailDelete, op(func(ctx context.Context, a api.MailDelete) (any, error) {
		for _, id := range a.IDs {
			if err := e.Mailbox.Delete(ctx, id, a.Purge); err != nil {
				return nil, err
			}
		}
		return map[string]int{"deleted": len(a.IDs)}, nil
	}))
	h(api.OpMailSend, op(func(ctx context.Context, a api.MailSend) (any, error) { return e.send(ctx, a) }))
	h(api.OpMailReply, op(func(ctx context.Context, a api.MailReply) (any, error) { return e.reply(ctx, a) }))
	h(api.OpMailAttachment, op(func(ctx context.Context, a api.Attachment) (any, error) {
		raw, err := e.Mailbox.Raw(ctx, a.ID)
		if err != nil {
			return nil, err
		}
		att, data, ok := msg.AttachmentData(raw, a.Index)
		if !ok {
			return nil, &ctl.Error{Code: ctl.CodeNotFound, Msg: fmt.Sprintf("message %d has no attachment %d", a.ID, a.Index)}
		}
		return api.File{Name: att.Name, Type: att.Type, Data: data}, nil
	}))
	e.Ctl.Stream(api.OpWatch, func(ctx context.Context, raw json.RawMessage, send func(any) error) error {
		var a api.Watch
		json.Unmarshal(raw, &a)
		want := map[string]bool{}
		for _, k := range a.Kinds {
			want[k] = true
		}
		ch, stop := e.Events.Subscribe()
		defer stop()
		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case ev, ok := <-ch:
				if !ok {
					return nil
				}
				if len(want) == 0 || want[ev.Kind] {
					if err := send(ev); err != nil {
						return err
					}
				}
			case <-ping.C:
				// Keeps the connection known to be alive without a message to the reader.
			}
		}
	})
	h(api.OpEvents, op(func(ctx context.Context, a api.Events) (any, error) {
		q := events.Query{Kinds: a.Kinds, IssueID: a.Issue, Limit: a.Limit}
		if a.Since != "" {
			t, err := parseSince(a.Since, e.Opts.Now())
			if err != nil {
				return nil, invalid("%v", err)
			}
			q.Since = t
		}
		if a.List != "" {
			l, err := e.Lists.Get(ctx, a.List)
			if err != nil {
				return nil, err
			}
			q.ListID = l.ID
		}
		return nonNil(e.Events.List(ctx, q))
	}))

	// Lists and members.
	h(api.OpListAdd, op(func(ctx context.Context, a api.ListSet) (any, error) {
		in, err := listInput(a.ListFields)
		if err != nil {
			return nil, err
		}
		l, err := e.Lists.Create(ctx, a.Slug, in)
		if err != nil {
			return nil, err
		}
		// Replies to a newsletter must reach someone: give the sender a mailbox.
		if _, ok, _ := e.Mailbox.Resolve(ctx, l.From); !ok {
			e.Mailbox.Add(ctx, l.From, l.FromName)
		}
		e.Certs.Forget()
		return l, nil
	}))
	h(api.OpListList, op(func(ctx context.Context, _ struct{}) (any, error) { return nonNil(e.Lists.All(ctx)) }))
	h(api.OpListText, op(func(ctx context.Context, a api.Name) (any, error) {
		l, err := e.Lists.Get(ctx, a.Name)
		if err != nil {
			return nil, err
		}
		return l.TextValues(), nil
	}))
	h(api.OpListTextSet, op(func(ctx context.Context, a api.ListTexts) (any, error) {
		l, err := e.Lists.SetTexts(ctx, a.Slug, a.Values)
		if err != nil {
			return nil, invalid("%s", err)
		}
		return l.TextValues(), nil
	}))
	h(api.OpListShow, op(func(ctx context.Context, a api.Name) (any, error) { return e.Lists.Get(ctx, a.Name) }))
	h(api.OpListSet, op(func(ctx context.Context, a api.ListSet) (any, error) {
		in, err := listInput(a.ListFields)
		if err != nil {
			return nil, err
		}
		l, err := e.Lists.Update(ctx, a.Slug, in)
		if err == nil && a.From != nil {
			if _, ok, _ := e.Mailbox.Resolve(ctx, l.From); !ok {
				e.Mailbox.Add(ctx, l.From, l.FromName)
			}
		}
		e.Certs.Forget()
		return l, err
	}))
	h(api.OpListRemove, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Lists.Remove(ctx, a.Name) }))
	h(api.OpListPause, op(func(ctx context.Context, a api.ListPause) (any, error) {
		l, err := e.Lists.Pause(ctx, a.Slug, a.Reason)
		if err == nil && a.Reason == "" {
			e.Queue.Wake()
		}
		return l, err
	}))
	h(api.OpSubAdd, op(func(ctx context.Context, a api.SubAdd) (any, error) { return e.addMembers(ctx, a) }))
	h(api.OpSubList, op(func(ctx context.Context, a api.SubList) (any, error) {
		return nonNil(e.Lists.Members(ctx, a.List, lists.Query{Status: a.Status, Search: a.Search, Limit: a.Limit, After: a.After}))
	}))
	h(api.OpSubRemove, op(func(ctx context.Context, a api.SubRemove) (any, error) {
		m, err := e.Lists.Find(ctx, a.List, a.Email)
		if err != nil {
			return nil, err
		}
		return e.Lists.Unsubscribe(ctx, m.ID, 0, "operator")
	}))
	h(api.OpSubImport, op(func(ctx context.Context, a api.SubImport) (any, error) {
		l, err := e.Lists.Get(ctx, a.List)
		if err != nil {
			return nil, err
		}
		if a.Source == "" {
			a.Source = "import"
		}
		if a.Confirmed && a.Consent == "" {
			return nil, invalid("--confirmed says these readers agreed elsewhere; record what they agreed to with --consent \"<the text of that signup form>\" (GDPR Art. 7(1))")
		}
		if a.Confirmed && a.Reconfirm {
			return nil, invalid("--confirmed and --reconfirm contradict each other")
		}
		res, err := e.Lists.ImportCSV(ctx, l, strings.NewReader(a.CSV), a.Confirmed, lists.Signup{Source: a.Source, Consent: a.Consent, Reconfirm: a.Reconfirm})
		if err != nil {
			return nil, invalid("csv: %v", err)
		}
		out := api.SubAddResult{Added: res.Added, Existing: res.Existing, OptedOut: res.OptedOut, Reconfirm: res.Reconfirm, Invalid: res.Invalid}
		for _, id := range res.Pending {
			if m, err := e.Lists.Member(ctx, id); err == nil {
				if e.sendConfirm(ctx, l, m) == nil {
					out.Confirmation++
				}
			}
		}
		return out, nil
	}))
	h(api.OpSubExport, op(func(ctx context.Context, a api.Name) (any, error) {
		var buf bytes.Buffer
		n, err := e.Lists.ExportCSV(ctx, a.Name, &buf)
		return api.CSV{Rows: n, CSV: buf.String()}, err
	}))
	h(api.OpContactShow, op(func(ctx context.Context, a api.Name) (any, error) { return e.Lists.Contact(ctx, a.Name) }))
	h(api.OpContactForget, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Lists.Forget(ctx, a.Name) }))
	h(api.OpSuppress, op(func(ctx context.Context, a api.Suppress) (any, error) {
		reason := strings.TrimSuffix("blocked: "+a.Reason, ": ")
		emails := a.Emails
		if a.Email != "" {
			emails = append(emails, a.Email)
		}
		var res api.SubAddResult
		for _, em := range emails {
			if err := e.Lists.Suppress(ctx, em, reason); err != nil {
				if len(emails) == 1 {
					return nil, err
				}
				res.Invalid = append(res.Invalid, em)
				continue
			}
			res.Added++
		}
		return res, nil
	}))
	h(api.OpUnsuppress, op(func(ctx context.Context, a api.Name) (any, error) { return nil, e.Lists.Unsuppress(ctx, a.Name) }))

	// Issues.
	h(api.OpIssueCreate, op(func(ctx context.Context, a api.IssueDraft) (any, error) {
		return e.Issues.Create(ctx, a.List, issueDraft(a))
	}))
	h(api.OpIssuePut, op(func(ctx context.Context, a api.IssueDraft) (any, error) {
		if a.Media != nil {
			for _, f := range *a.Media {
				if _, err := issues.CheckMedia(f.Path, f.Data); err != nil {
					return nil, invalid("%v", err)
				}
			}
		}
		w, created, err := e.Issues.Put(ctx, a.List, issueDraft(a), a.Welcome)
		if err != nil {
			return nil, err
		}
		if a.Media != nil {
			if err := e.Issues.SetMedia(ctx, w.ID, *a.Media); err != nil {
				return nil, err
			}
			if w, err = e.Issues.Get(ctx, w.ID); err != nil {
				return nil, err
			}
		}
		return api.IssuePut{Issue: *w, Created: created}, nil
	}))
	h(api.OpIssueUpdate, op(func(ctx context.Context, a api.IssueDraft) (any, error) {
		return e.Issues.Update(ctx, a.ID, issueDraft(a))
	}))
	h(api.OpIssueList, op(func(ctx context.Context, a api.IssueList) (any, error) {
		l, err := e.Lists.Get(ctx, a.List)
		if err != nil {
			return nil, err
		}
		ws, err := e.Issues.List(ctx, l.ID, false, 1000)
		for _, w := range ws {
			w.Markdown = ""
		}
		return nonNil(ws, err)
	}))
	issue := func(fn func(ctx context.Context, id int64) (any, error)) ctl.Handler {
		return op(func(ctx context.Context, a api.IssueRef) (any, error) {
			id, err := e.issueID(ctx, a)
			if err != nil {
				return nil, err
			}
			return fn(ctx, id)
		})
	}
	h(api.OpIssueShow, issue(func(ctx context.Context, id int64) (any, error) { return e.Issues.Get(ctx, id) }))
	h(api.OpIssueLint, issue(func(ctx context.Context, id int64) (any, error) { return nonNil(e.Issues.Lint(ctx, e.Renderer, id)) }))
	h(api.OpIssuePreview, issue(func(ctx context.Context, id int64) (any, error) { return e.preview(ctx, id) }))
	h(api.OpIssueCancel, issue(func(ctx context.Context, id int64) (any, error) { return e.Issues.Cancel(ctx, id) }))
	h(api.OpIssueStats, issue(func(ctx context.Context, id int64) (any, error) { return e.Issues.Stats(ctx, id) }))
	h(api.OpIssueRemove, issue(func(ctx context.Context, id int64) (any, error) { return nil, e.Issues.Remove(ctx, id) }))
	h(api.OpIssueTest, op(func(ctx context.Context, a api.IssueTest) (any, error) {
		if len(a.To) == 0 {
			return nil, invalid("name at least one test address (--to you@example.com)")
		}
		id, err := e.issueID(ctx, a.IssueRef)
		if err != nil {
			return nil, err
		}
		return nil, e.Issues.Test(ctx, id, a.To)
	}))
	h(api.OpIssueSend, op(func(ctx context.Context, a api.IssueSend) (any, error) {
		id, err := e.issueID(ctx, a.IssueRef)
		if err != nil {
			return nil, err
		}
		findings, err := e.Issues.Lint(ctx, e.Renderer, id)
		if err != nil {
			return nil, err
		}
		if issues.Blocking(findings) {
			return findings, invalid("the issue has errors; fix them (lfr issue show %d) before sending", id)
		}
		warnings := 0
		for _, f := range findings {
			if f.Severity == "warning" {
				warnings++
			}
		}
		if warnings > 0 && !a.Force {
			return findings, invalid("the issue has %d warnings (lfr issue show %d); fix them or send with --force", warnings, id)
		}
		var at time.Time
		if a.At != "" {
			if at, err = time.Parse(time.RFC3339, a.At); err != nil {
				return nil, invalid("--at %q: use RFC 3339 like 2026-10-20T08:00:00+02:00", a.At)
			}
		}
		// Sent once, an issue is done: sending it to nobody wastes it.
		if at.IsZero() || !at.After(e.Opts.Now()) {
			if w, err := e.Issues.Get(ctx, id); err == nil {
				var readers int
				e.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM members WHERE list_id = ? AND status = 'confirmed'", w.ListID).Scan(&readers)
				if readers == 0 {
					return nil, invalid("list %s has no confirmed readers yet, so nothing would go out and the issue would count as sent; wait for signups, or schedule it with --at", w.List)
				}
			}
		}
		if !a.Force {
			other, err := e.Issues.Crowding(ctx, id, at)
			if err != nil {
				return nil, err
			}
			if other != nil {
				when := other.Started
				if when.IsZero() {
					when = other.Scheduled
				}
				return nil, invalid("issue %s (%s) goes out %s, closer than the list's minimum gap; readers who get two issues in a day unsubscribe. Send later with --at, or with --force",
					other.Slug, other.Status, when.Format(time.RFC3339))
			}
		}
		w, err := e.Issues.Schedule(ctx, id, at)
		if err != nil {
			return nil, err
		}
		if !w.Scheduled.After(e.Opts.Now()) {
			e.Issues.Tick(ctx)
			w, _ = e.Issues.Get(ctx, id)
		}
		return w, nil
	}))

	// Sites.
	h(api.OpSitePut, op(func(ctx context.Context, a api.SitePut) (any, error) {
		if _, err := e.Lists.Get(ctx, a.List); err != nil {
			return nil, err
		}
		if err := e.Sites.Put(a.List, a.Files); err != nil {
			return nil, invalid("%v", err)
		}
		return map[string]any{"list": a.List, "files": len(a.Files)}, nil
	}))
	h(api.OpSiteCheck, op(func(ctx context.Context, a api.Name) (any, error) {
		if _, err := e.Lists.Get(ctx, a.Name); err != nil {
			return nil, err
		}
		if err := e.Sites.Check(a.Name); err != nil {
			return nil, invalid("%v", err)
		}
		return map[string]string{"status": "ok"}, nil
	}))

	// Relay.
	h(api.OpRelaySet, op(func(ctx context.Context, a api.RelaySet) (any, error) {
		if a.URL != "" {
			if _, err := outbound.ParseRelay(a.URL, a.Password, "x"); err != nil {
				return nil, invalid("%v", err)
			}
		}
		for k, v := range map[string]string{"relay.url": a.URL, "relay.password": a.Password, "relay.spf": a.SPF} {
			if err := e.Config.Set(ctx, k, v); err != nil {
				return nil, err
			}
		}
		e.Queue.Wake()
		return e.relayInfo(ctx), nil
	}))
	h(api.OpRelayShow, op(func(ctx context.Context, _ struct{}) (any, error) { return e.relayInfo(ctx), nil }))
	h(api.OpRelayTest, op(func(ctx context.Context, _ struct{}) (any, error) { return e.testTransport(ctx), nil }))
	h(api.OpRelayWebhook, op(func(ctx context.Context, _ struct{}) (any, error) { return e.relayInfo(ctx).Webhooks, nil }))

	// Queue, spam, reports, backup.
	h(api.OpQueueList, op(func(ctx context.Context, a api.QueueList) (any, error) {
		entries, err := e.Queue.List(ctx, a.Status, a.Limit)
		if err != nil {
			return nil, err
		}
		st, _ := e.Queue.Stats(ctx)
		if entries == nil {
			entries = []outbound.Entry{}
		}
		return map[string]any{"stats": st, "entries": entries}, nil
	}))
	h(api.OpQueueRetry, op(func(ctx context.Context, a api.ID) (any, error) {
		n, err := e.Queue.Retry(ctx, a.ID)
		return map[string]int{"retrying": n}, err
	}))
	h(api.OpQueueDrop, op(func(ctx context.Context, a api.ID) (any, error) { return nil, e.Queue.Drop(ctx, a.ID) }))
	h(api.OpSpamStats, op(func(ctx context.Context, _ struct{}) (any, error) {
		return map[string]any{"stats": e.Spam.Stats(ctx), "weights": e.Spam.Weights(ctx)}, nil
	}))
	h(api.OpSpamReset, op(func(ctx context.Context, _ struct{}) (any, error) { return nil, e.Spam.Reset(ctx) }))
	h(api.OpReports, op(func(ctx context.Context, a api.Reports) (any, error) { return e.reports(ctx, a) }))
	e.Ctl.Stream(api.OpBackup, func(ctx context.Context, args json.RawMessage, send func(any) error) error {
		// A CLI from lfr 0.2.1 or older asks the server to write into a directory.
		var old struct {
			Dir string `json:"dir"`
		}
		if json.Unmarshal(args, &old) == nil && old.Dir != "" {
			return invalid("this server (lfr %s) streams backups to the CLI, and this lfr CLI is older; install the same version where you run lfr backup (curl -fsSL https://lfr.email/install.sh | sh) and run it again", e.Opts.Version)
		}
		return e.Backup(ctx, send)
	})
}

func nonNil[T any](v []T, err error) (any, error) {
	if v == nil {
		v = []T{}
	}
	return v, err
}

func listInput(f api.ListFields) (lists.Input, error) {
	in := lists.Input{Name: f.Name, Description: f.Description, From: f.From, FromName: f.FromName, ReplyTo: f.ReplyTo, Host: f.Host,
		Language: f.Language, Tracking: f.Tracking, ImprintURL: f.ImprintURL, PrivacyURL: f.PrivacyURL, Address: f.Address, Consent: f.Consent,
		Cadence: f.Cadence, Headline: f.Headline, Pitch: f.Pitch}
	if f.MinGap != nil {
		d, err := time.ParseDuration(*f.MinGap)
		if *f.MinGap == "0" {
			d, err = 0, nil
		}
		if err != nil {
			return in, invalid("--min-gap %q: use a duration like 20h or 0", *f.MinGap)
		}
		in.MinGap = &d
	}
	return in, nil
}

func issueDraft(a api.IssueDraft) issues.Draft {
	return issues.Draft{Markdown: a.Markdown, Subject: a.Subject, Slug: a.Slug, Preheader: a.Preheader, Summary: a.Summary}
}

// issueID resolves an issue named by ID or by list and slug.
func (e *Engine) issueID(ctx context.Context, r api.IssueRef) (int64, error) {
	if r.ID > 0 {
		return r.ID, nil
	}
	if r.List == "" || r.Slug == "" {
		return 0, invalid("name an issue by its ID or as <list>/<slug>")
	}
	l, err := e.Lists.Get(ctx, r.List)
	if err != nil {
		return 0, err
	}
	w, err := e.Issues.BySlug(ctx, l.ID, r.Slug)
	if errors.Is(err, store.ErrNotFound) {
		return 0, fmt.Errorf("issue %s/%s: %w; see lfr issue ls %s", r.List, r.Slug, store.ErrNotFound, r.List)
	}
	if err != nil {
		return 0, err
	}
	return w.ID, nil
}

func parseSince(s string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err == nil {
			return now.AddDate(0, 0, -n), nil
		}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since %q: use a duration like 24h or 7d, or RFC 3339", s)
	}
	return t, nil
}

func (e *Engine) domainInfo(ctx context.Context, d *domains.Domain, check bool) (api.DomainInfo, error) {
	inst := e.Instance(ctx)
	hosts, err := e.Lists.Hosts(ctx, d.ID)
	if err != nil {
		return api.DomainInfo{}, err
	}
	recs := domains.Records(d, inst, hosts)
	info := api.DomainInfo{Name: d.Name, Selector: d.Selector, Catchall: d.Catchall, Created: d.Created, Records: recs, Zone: domains.Zone(recs)}
	if d.Next != nil {
		info.Next = d.Next.Selector
	}
	if inst.IPv4 == "" {
		info.Notes = append(info.Notes, "The server's public IPv4 is unknown, so A records are missing: lfr config ipv4 203.0.113.7")
	}
	info.Notes = append(info.Notes,
		"Publish these records at your DNS host. On Cloudflare set every record to DNS only (grey cloud); website records may use the proxy (orange) once HTTPS works.",
		"Name the server "+inst.Hostname+" at your hosting provider so its reverse DNS (PTR) matches; DigitalOcean derives it from the droplet name.")
	if url, _ := e.Config.Get(ctx, "relay.url"); url != "" {
		info.Notes = append(info.Notes, "Your relay may ask for its own verification records for "+d.Name+" (DKIM CNAMEs, a return-path); add those as well.")
	}
	if check {
		info.Checks = e.checkRecords(ctx, recs)
	}
	return info, nil
}

func (e *Engine) moveMessages(ctx context.Context, a api.MailMove) (any, error) {
	folder, ok := mailbox.ValidFolder(a.Folder)
	if !ok {
		return nil, invalid("unknown folder %q; folders are %s", a.Folder, strings.Join(mailbox.Folders, ", "))
	}
	trained := 0
	for _, id := range a.IDs {
		old, err := e.Mailbox.Move(ctx, id, folder)
		if err != nil {
			return nil, err
		}
		label := ""
		switch {
		case folder == "junk" && old != "junk":
			label = "spam"
		case old == "junk" && (folder == "inbox" || folder == "archive"):
			label = "ham"
		}
		if label == "" {
			continue
		}
		if e.train(ctx, id, label) {
			trained++
		}
	}
	return map[string]any{"moved": len(a.IDs), "folder": folder, "trained": trained}, nil
}

// train teaches the spam filter from one message.
func (e *Engine) train(ctx context.Context, id int64, label string) bool {
	m, err := e.Mailbox.Read(ctx, id, false)
	if err != nil || m.Folder == "sent" {
		return false
	}
	raw, err := e.Blobs.Get(m.Blob)
	if err != nil {
		return false
	}
	if err := e.Spam.Train(ctx, msg.Parse(raw), m.Features, label, m.Trained); err != nil {
		e.Log.Warn("spam training", "message", id, "err", err)
		return false
	}
	e.Mailbox.SetTrained(ctx, id, label)
	return true
}

func toFiles(fs []api.File) []msg.File {
	var out []msg.File
	for _, f := range fs {
		out = append(out, msg.File{Name: f.Name, Type: f.Type, Data: f.Data})
	}
	return out
}

func bodies(markdown, text, html string) (string, string, error) {
	if markdown != "" {
		doc, err := emailmd.Render(markdown)
		if err != nil {
			return "", "", err
		}
		// Personal mail: styled text, no newsletter frame.
		out := `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>` +
			`<body style="margin:0;padding:16px;"><div style="max-width:640px;font-family:` + string(emailmd.DefaultTheme.Font) +
			`;font-size:16px;line-height:1.6;color:#27272a;">` + emailmd.Inline(doc.HTML, emailmd.DefaultTheme) + `</div></body></html>`
		return emailmd.Text(markdown), out, nil
	}
	return text, html, nil
}

func (e *Engine) send(ctx context.Context, a api.MailSend) (any, error) {
	text, html, err := bodies(a.Markdown, a.Text, a.HTML)
	if err != nil {
		return nil, err
	}
	id, err := e.Mailbox.Send(ctx, e.Queue, mailbox.SendRequest{From: a.From, To: a.To, Cc: a.Cc, Bcc: a.Bcc, ReplyTo: a.ReplyTo,
		Subject: a.Subject, Text: text, HTML: html, Files: toFiles(a.Files)})
	if err != nil {
		return nil, invalidIf(err)
	}
	return api.Sent{ID: id, Queued: len(a.To) + len(a.Cc) + len(a.Bcc), Message: "queued; lfr events --follow shows delivery"}, nil
}

func (e *Engine) reply(ctx context.Context, a api.MailReply) (any, error) {
	text, html, err := bodies(a.Markdown, a.Text, a.HTML)
	if err != nil {
		return nil, err
	}
	m, err := e.Mailbox.Read(ctx, a.ID, false)
	if err != nil {
		return nil, err
	}
	id, err := e.Mailbox.Reply(ctx, e.Queue, a.ID, mailbox.ReplyRequest{Text: text, HTML: html, All: a.All, Quote: !a.NoQuote, Files: toFiles(a.Files)})
	if err != nil {
		return nil, invalidIf(err)
	}
	// Answering someone is the strongest sign their mail was wanted.
	if m.Trained == "" && m.Folder != "junk" && m.Features != "" {
		e.train(ctx, a.ID, "ham")
	}
	return api.Sent{ID: id, Queued: 1, Message: "queued"}, nil
}

func invalidIf(err error) error {
	var ce *ctl.Error
	if errors.As(err, &ce) || classify(err) != ctl.CodeInternal {
		return err
	}
	return &ctl.Error{Code: ctl.CodeInvalid, Msg: err.Error()}
}

func (e *Engine) addMembers(ctx context.Context, a api.SubAdd) (any, error) {
	l, err := e.Lists.Get(ctx, a.List)
	if err != nil {
		return nil, err
	}
	if a.Source == "" {
		a.Source = "operator"
	}
	if a.Confirmed && a.Consent == "" {
		return nil, invalid("--confirmed says these readers agreed elsewhere; record what they agreed to with --consent \"<the text of that signup form>\" (GDPR Art. 7(1))")
	}
	var out api.SubAddResult
	for _, email := range a.Emails {
		before, _ := e.Lists.Find(ctx, l.Slug, email)
		m, send, err := e.Lists.Add(ctx, l, email, "", a.Confirmed, lists.Signup{Source: a.Source, Consent: a.Consent, Reconfirm: a.Reconfirm})
		if errors.Is(err, lists.ErrOptedOut) {
			out.OptedOut++
			continue
		}
		if err != nil {
			out.Invalid = append(out.Invalid, email)
			continue
		}
		switch {
		case before != nil && before.Status == "confirmed" && m.Status == "pending":
			out.Reconfirm++
		case before != nil && before.Status == m.Status:
			out.Existing++
		default:
			out.Added++
		}
		if send && e.sendConfirm(ctx, l, m) == nil {
			out.Confirmation++
		}
	}
	return out, nil
}

func (e *Engine) preview(ctx context.Context, id int64) (any, error) {
	w, err := e.Issues.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	l, err := e.Lists.ByID(ctx, w.ListID)
	if err != nil {
		return nil, err
	}
	raw, err := e.Renderer.Message(l, w, issues.Recipient{Email: "reader@example.org"}, false)
	if err != nil {
		return nil, err
	}
	p := msg.Parse(raw)
	return api.Preview{Subject: p.Subject, HTML: p.HTML, Text: p.Text, Size: len(raw), WebURL: e.Renderer.IssueURL(l, w)}, nil
}

func (e *Engine) relayInfo(ctx context.Context) api.RelayInfo {
	url, _ := e.Config.Get(ctx, "relay.url")
	spf, _ := e.Config.Get(ctx, "relay.spf")
	info := api.RelayInfo{Mode: "direct", URL: url, SPF: spf, Webhooks: map[string]string{}}
	if url != "" {
		info.Mode = "relay"
	}
	base := "https://" + e.Hostname(ctx)
	for _, p := range []string{"ses", "smtp2go", "generic"} {
		info.Webhooks[p] = base + e.Web.HookPath(p)
	}
	return info
}

// testTransport connects to the relay or to a large mail host's port 25.
func (e *Engine) testTransport(ctx context.Context) api.Check {
	url, _ := e.Config.Get(ctx, "relay.url")
	if url == "" {
		if err := outbound.Probe(ctx, "gmail-smtp-in.l.google.com"); err != nil {
			return api.Check{Name: "outbound", Status: "fail", Detail: "port 25 to gmail-smtp-in.l.google.com: " + err.Error(),
				Fix: "your hosting blocks outgoing port 25; configure a relay: lfr relay smtps://user@host:2465"}
		}
		return api.Check{Name: "outbound", Status: "ok", Detail: "direct delivery: port 25 reachable"}
	}
	pass, _ := e.Config.Get(ctx, "relay.password")
	r, err := outbound.ParseRelay(url, pass, e.Hostname(ctx))
	if err != nil {
		return api.Check{Name: "outbound", Status: "fail", Detail: err.Error(), Fix: "lfr relay <url> with a valid URL"}
	}
	defer r.Close()
	if err := r.Ping(ctx); err != nil {
		return api.Check{Name: "outbound", Status: "fail", Detail: "relay " + url + ": " + err.Error(),
			Fix: "check user, password and port; DigitalOcean blocks 25, 465 and 587, so use the relay's alternate port (2465, 2587 or 2525)"}
	}
	return api.Check{Name: "outbound", Status: "ok", Detail: "relay " + url + " is reachable and accepts us"}
}

func (e *Engine) reports(ctx context.Context, a api.Reports) (any, error) {
	if a.Limit <= 0 {
		a.Limit = 50
	}
	rows, err := e.DB.R.QueryContext(ctx, `SELECT kind, org, domain, begin_at, end_at, pass, fail, received, detail FROM reports
		WHERE (? = '' OR kind = ?) AND (? = '' OR domain = ?) ORDER BY end_at DESC LIMIT ?`, a.Kind, a.Kind, a.Domain, a.Domain, a.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []api.Report{}
	for rows.Next() {
		var r api.Report
		var begin, end, received int64
		var detail string
		if err := rows.Scan(&r.Kind, &r.Org, &r.Domain, &begin, &end, &r.Pass, &r.Fail, &received, &detail); err != nil {
			return nil, err
		}
		r.Begin, r.End, r.Received = time.Unix(begin, 0).UTC(), time.Unix(end, 0).UTC(), time.Unix(received, 0).UTC()
		if json.Valid([]byte(detail)) {
			r.Rows = json.RawMessage(detail)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BackupDone marks a complete backup.
const BackupDone = "BACKUP-COMPLETE"
