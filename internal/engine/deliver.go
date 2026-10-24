package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/domains"
	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/feedback"
	"github.com/Anonyfox/lfr/internal/inbound"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/mailbox"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/token"
	"github.com/Anonyfox/lfr/internal/web"
)

// accept decides at RCPT time whether lfr takes mail for rcpt.
func (e *Engine) accept(ctx context.Context, rcpt string) (bool, error) {
	a, err := addr.Parse(rcpt)
	if err != nil {
		return false, nil
	}
	if _, err := e.Domains.Get(ctx, a.Domain); err != nil {
		return false, nil
	}
	switch {
	case strings.HasPrefix(a.Local, "bounce+"):
		_, err := e.Tokens.Parse(token.Bounce, strings.TrimPrefix(a.Local, "bounce+"), 1)
		return err == nil, nil
	case strings.HasPrefix(a.Local, "unsub+"):
		_, err := e.Tokens.Parse(token.Unsub, strings.TrimPrefix(a.Local, "unsub+"), 2)
		return err == nil, nil
	case a.Local == domains.DMARCReports, a.Local == domains.TLSReports:
		return true, nil
	}
	_, ok, err := e.Mailbox.Resolve(ctx, a.String())
	return ok, err
}

// knownContact reports whether a mailbox here wrote to email before.
func (e *Engine) knownContact(ctx context.Context, email string) bool {
	var n int
	e.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM (SELECT 1 FROM outbound WHERE rcpt = ? AND kind = 'mail' LIMIT 1)", strings.ToLower(email)).Scan(&n)
	return n > 0
}

// deliver routes an accepted message to each recipient's handler.
func (e *Engine) deliver(ctx context.Context, env *inbound.Envelope) error {
	delivered := map[int64]bool{}
	sample, _ := json.Marshal(env.Verdict.Sample)
	verdict, _ := json.Marshal(map[string]any{"auth": env.Auth, "spam": env.Verdict, "ip": env.IP.String(), "helo": env.Helo, "tls": env.TLS, "mail_from": env.From})
	complaintChecked := false
	for _, rcpt := range env.Rcpts {
		a, err := addr.Parse(rcpt)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(a.Local, "bounce+"):
			ids, err := e.Tokens.Parse(token.Bounce, strings.TrimPrefix(a.Local, "bounce+"), 1)
			if err == nil {
				e.handleBounce(ctx, ids[0], env.Parsed)
			}
			if !complaintChecked {
				complaintChecked = true
				e.handleComplaintReport(ctx, env)
			}
		case strings.HasPrefix(a.Local, "unsub+"):
			ids, err := e.Tokens.Parse(token.Unsub, strings.TrimPrefix(a.Local, "unsub+"), 2)
			if err == nil {
				if _, err := e.Lists.Unsubscribe(ctx, ids[0], ids[1], "mail"); err != nil {
					e.Log.Warn("unsubscribe by mail", "member", ids[0], "err", err)
				}
			}
		case a.Local == domains.DMARCReports:
			e.storeReport(ctx, "dmarc", env.Parsed)
		case a.Local == domains.TLSReports:
			e.storeReport(ctx, "tlsrpt", env.Parsed)
		default:
			id, ok, err := e.Mailbox.Resolve(ctx, a.String())
			if err != nil {
				return err
			}
			if !ok || delivered[id] {
				continue
			}
			delivered[id] = true
			if err := e.toMailbox(ctx, id, env, string(verdict), string(sample)); err != nil {
				return err
			}
			e.replyUnsubscribe(ctx, a.String(), env)
			if !complaintChecked {
				complaintChecked = true
				e.handleComplaintReport(ctx, env)
			}
		}
	}
	return nil
}

// forwardable reports whether a message may be forwarded. Bounces, other
// automatic mail and messages this mailbox forwarded before stay put: a
// forward that bounces would otherwise come back and go out again forever
// (RFC 3834 §2, RFC 5321 §6.3).
func forwardable(env *inbound.Envelope, mailbox string) bool {
	h := env.Parsed.Header
	if env.From == "" || strings.HasPrefix(strings.ToLower(h.Get("Content-Type")), "multipart/report") {
		return false
	}
	if a := strings.ToLower(strings.TrimSpace(h.Get("Auto-Submitted"))); a != "" && a != "no" {
		return false
	}
	for _, v := range h.Values("X-Forwarded-For") {
		for _, a := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(a), mailbox) {
				return false
			}
		}
	}
	return true
}

func (e *Engine) toMailbox(ctx context.Context, mailboxID int64, env *inbound.Envelope, verdict, sample string) error {
	folder := "inbox"
	if env.Verdict.P >= e.Config.Float(ctx, "spam.junk") || env.Auth.DMARC == "fail" && env.Auth.DMARCPolicy == "quarantine" {
		folder = "junk"
	}
	id, err := e.Mailbox.Deliver(ctx, mailbox.Delivery{MailboxID: mailboxID, Folder: folder, Raw: env.Raw, Parsed: env.Parsed,
		Spam: env.Verdict.P, Verdict: verdict, Features: sample})
	if err != nil {
		return err
	}
	mb, err := e.Mailbox.ByID(ctx, mailboxID)
	if err != nil {
		return err
	}
	e.Events.Record(ctx, events.Event{Kind: events.Received, MessageID: id,
		Detail: fmt.Sprintf("%s → %s [%s] %s", env.Parsed.From.Email, mb.Address, folder, msg.Snippet(env.Parsed.Subject, 80))})
	if mb.Forward != "" && folder == "inbox" && forwardable(env, mb.Address) {
		item, err := e.Mailbox.ForwardItem(ctx, mb, env.Raw, env.Parsed)
		if err != nil {
			e.Log.Warn("forward", "mailbox", mb.Address, "err", err)
			return nil
		}
		if err := e.Queue.Enqueue(ctx, item); err != nil {
			e.Log.Warn("forward", "mailbox", mb.Address, "err", err)
		}
	}
	return nil
}

// row is an outbound message as far as feedback needs it.
type row struct {
	id, issueID, contactID, memberID, listID int64
	kind, rcpt                               string
}

func (e *Engine) outboundRow(ctx context.Context, id int64) (row, bool) {
	var r row
	err := e.DB.R.QueryRowContext(ctx, `SELECT o.id, coalesce(o.issue_id, 0), coalesce(o.contact_id, 0), coalesce(o.member_id, 0),
		coalesce(w.list_id, (SELECT list_id FROM members WHERE id = o.member_id), 0), o.kind, o.rcpt
		FROM outbound o LEFT JOIN issues w ON w.id = o.issue_id WHERE o.id = ?`, id).
		Scan(&r.id, &r.issueID, &r.contactID, &r.memberID, &r.listID, &r.kind, &r.rcpt)
	return r, err == nil
}

// handleBounce reads a delivery status notification sent to a VERP address.
func (e *Engine) handleBounce(ctx context.Context, outboundID int64, p *msg.Parsed) {
	r, ok := e.outboundRow(ctx, outboundID)
	if !ok {
		return
	}
	rcpts, _, isDSN := feedback.DSN(p)
	if !isDSN {
		return // an auto-reply or out-of-office to the bounce address
	}
	for _, rc := range rcpts {
		// The report must name the recipient: a bare status for a VERP
		// address someone guessed or saw is not evidence about anyone.
		if !strings.EqualFold(rc.Email, r.rcpt) || rc.Action != "failed" {
			continue
		}
		if rc.Hard() {
			e.bounced(ctx, r, rc.Status+" "+rc.Diagnostic)
		} else {
			e.softBounced(ctx, r, rc.Status+" "+rc.Diagnostic)
		}
		return
	}
}

// softBounced records a temporary failure that ended a delivery; addresses
// that keep failing are suppressed (lists.SoftBounce).
func (e *Engine) softBounced(ctx context.Context, r row, reason string) {
	e.Events.Record(ctx, events.Event{Kind: events.Bounced, ListID: r.listID, IssueID: r.issueID, ContactID: r.contactID, OutboundID: r.id, Detail: r.rcpt + ": soft: " + reason})
	if r.contactID == 0 {
		return
	}
	if gone, err := e.Lists.SoftBounce(ctx, r.contactID, strings.TrimSpace(reason)); err != nil {
		e.Log.Warn("soft bounce", "rcpt", r.rcpt, "err", err)
	} else if gone && r.listID != 0 {
		if why, _ := e.Lists.CheckHealth(ctx, r.listID); why != "" {
			e.Log.Warn("list paused", "list", r.listID, "reason", why)
		}
	}
}

func (e *Engine) bounced(ctx context.Context, r row, reason string) {
	e.DB.W.ExecContext(ctx, "UPDATE outbound SET status = 'bounced', last_error = ? WHERE id = ? AND status IN ('sent', 'queued')", reason, r.id)
	e.Events.Record(ctx, events.Event{Kind: events.Bounced, ListID: r.listID, IssueID: r.issueID, ContactID: r.contactID, OutboundID: r.id, Detail: r.rcpt + ": " + reason})
	if r.contactID != 0 {
		e.suppressAfterBounce(ctx, r, reason)
	}
}

func (e *Engine) suppressAfterBounce(ctx context.Context, r row, reason string) {
	if err := e.Lists.Suppress(ctx, r.rcpt, "hard bounce: "+strings.TrimSpace(reason)); err != nil {
		e.Log.Warn("suppress", "rcpt", r.rcpt, "err", err)
	}
	if r.listID != 0 {
		if why, _ := e.Lists.CheckHealth(ctx, r.listID); why != "" {
			e.Log.Warn("list paused", "list", r.listID, "reason", why)
		}
	}
}

// handleComplaintReport acts on an abuse report (feedback loop) about an issue.
func (e *Engine) handleComplaintReport(ctx context.Context, env *inbound.Envelope) {
	// Feedback loops of mailbox providers sign their reports; an
	// unauthenticated report could unsubscribe anyone whose Message-ID the
	// sender saw.
	if env.Auth.DMARC != "pass" {
		return
	}
	c, ok := feedback.ARF(env.Parsed)
	if !ok {
		return
	}
	var r row
	found := false
	if local, _, ok := strings.Cut(c.OriginalID, "@"); ok {
		tok, _, _ := strings.Cut(local, ".")
		if ids, err := e.Tokens.Parse(token.Message, tok, 1); err == nil {
			r, found = e.outboundRow(ctx, ids[0])
		}
	}
	if !found && c.ListUnsub != "" {
		if i := strings.Index(c.ListUnsub, "/u/"); i >= 0 {
			tok := c.ListUnsub[i+3:]
			if j := strings.IndexAny(tok, ">?/ ,"); j >= 0 {
				tok = tok[:j]
			}
			if ids, err := e.Tokens.Parse(token.Unsub, tok, 2); err == nil {
				if m, err := e.Lists.Member(ctx, ids[0]); err == nil {
					r = row{memberID: m.ID, contactID: m.ContactID, listID: m.ListID, issueID: ids[1], rcpt: m.Email}
					found = true
				}
			}
		}
	}
	if !found {
		e.Log.Info("complaint report without a reference lfr knows", "type", c.Type, "recipient", c.Recipient)
		return
	}
	e.complained(ctx, r, "feedback report: "+c.Type)
}

func (e *Engine) complained(ctx context.Context, r row, detail string) {
	if r.memberID != 0 {
		e.Lists.Unsubscribe(ctx, r.memberID, r.issueID, "complaint")
	}
	if r.rcpt != "" {
		e.Lists.Suppress(ctx, r.rcpt, "complaint")
	}
	e.Events.Record(ctx, events.Event{Kind: events.Complained, ListID: r.listID, IssueID: r.issueID, ContactID: r.contactID, OutboundID: r.id, Detail: r.rcpt + ": " + detail})
	if r.listID != 0 {
		if why, _ := e.Lists.CheckHealth(ctx, r.listID); why != "" {
			e.Log.Warn("list paused", "list", r.listID, "reason", why)
		}
	}
}

// snsTopic pins the SNS topic the ses webhook trusts to the first one
// confirmed: anyone can sign messages with a topic of their own.
func (e *Engine) snsTopic(ctx context.Context, arn string, confirm bool) bool {
	pinned, _ := e.Config.Get(ctx, "relay.sns_topic")
	if pinned == "" && confirm && arn != "" {
		return e.Config.Set(ctx, "relay.sns_topic", arn) == nil
	}
	return pinned != "" && pinned == arn
}

// onWebhook applies a relay's bounce, complaint and unsubscribe notifications.
// Each must match a message lfr sent to that address: through the VERP
// envelope sender when the relay reports it (Amazon SES), else the latest
// one. Notifications about anything else are ignored, so a leaked webhook
// URL cannot suppress arbitrary addresses.
func (e *Engine) onWebhook(ctx context.Context, evs []feedback.Event) {
	for _, ev := range evs {
		r, ok := e.webhookRow(ctx, ev)
		if !ok {
			e.Log.Info("webhook about no message sent here", "kind", ev.Kind, "rcpt", ev.Email)
			continue
		}
		switch ev.Kind {
		case "bounce":
			if ev.Hard {
				e.bounced(ctx, r, strings.TrimSuffix("relay: "+ev.Note, ": "))
			}
		case "complaint":
			e.complained(ctx, r, strings.TrimSuffix("relay: "+ev.Note, ": "))
		case "unsubscribe":
			if r.memberID != 0 {
				e.Lists.Unsubscribe(ctx, r.memberID, r.issueID, "relay")
			}
		}
	}
}

func (e *Engine) webhookRow(ctx context.Context, ev feedback.Event) (row, bool) {
	if local, _, ok := strings.Cut(ev.Source, "@"); ok && strings.HasPrefix(local, "bounce+") {
		ids, err := e.Tokens.Parse(token.Bounce, strings.TrimPrefix(local, "bounce+"), 1)
		if err != nil {
			return row{}, false
		}
		r, ok := e.outboundRow(ctx, ids[0])
		return r, ok && strings.EqualFold(r.rcpt, ev.Email)
	}
	var id int64
	err := e.DB.R.QueryRowContext(ctx, `SELECT id FROM outbound WHERE rcpt = ? AND status IN ('sent', 'bounced') ORDER BY id DESC LIMIT 1`, ev.Email).Scan(&id)
	if err != nil {
		return row{}, false
	}
	return e.outboundRow(ctx, id)
}

func (e *Engine) storeReport(ctx context.Context, kind string, p *msg.Parsed) {
	var rep feedback.Report
	var err error
	if kind == "dmarc" {
		rep, err = feedback.DMARC(p)
	} else {
		rep, err = feedback.TLSRPT(p)
	}
	if err != nil {
		e.Log.Info("report without readable content", "kind", kind, "from", p.From.Email)
		return
	}
	_, err = e.DB.W.ExecContext(ctx, `INSERT INTO reports(kind, org, report, domain, begin_at, end_at, received, pass, fail, detail)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		kind, rep.Org, rep.ID, rep.Domain, rep.Begin.Unix(), rep.End.Unix(), e.Opts.Now().Unix(), rep.Pass, rep.Fail, string(rep.Rows))
	if err != nil {
		e.Log.Warn("store report", "err", err)
	}
}

// Signup records a web signup and sends the confirmation, within the
// per-list hourly limit that keeps lfr from being used to bomb inboxes.
func (e *Engine) Signup(ctx context.Context, l *lists.List, email string, s lists.Signup) error {
	if e.Lists.ConfirmsLastHour(ctx, l.ID) >= e.Config.Int(ctx, "web.confirm_per_list") {
		return web.ErrLimited
	}
	m, send, err := e.Lists.Subscribe(ctx, l, email, s)
	if err != nil {
		return err
	}
	if send {
		return e.sendConfirm(ctx, l, m)
	}
	return nil
}

func (e *Engine) sendConfirm(ctx context.Context, l *lists.List, m *lists.Member) error {
	if strings.HasPrefix(m.Suppressed, "complaint") {
		return nil // whoever signs up a complainer does not get to mail them
	}
	e.Lists.MarkConfirmSent(ctx, m.ID)
	return e.Queue.Enqueue(ctx, outbound.Item{Kind: outbound.KindConfirm, DomainID: l.DomainID, ContactID: m.ContactID, MemberID: m.ID, Rcpt: m.Email})
}

// buildConfirm renders a double opt-in request.
func (e *Engine) buildConfirm(ctx context.Context, r outbound.Row) ([]byte, error) {
	m, err := e.Lists.Member(ctx, r.MemberID)
	if err != nil {
		return nil, fmt.Errorf("%w: member gone", outbound.ErrSkip)
	}
	if m.Status != "pending" {
		return nil, fmt.Errorf("%w: member is %s", outbound.ErrSkip, m.Status)
	}
	if m.Suppressed != "" {
		return nil, fmt.Errorf("%w: address suppressed (%s)", outbound.ErrSkip, m.Suppressed)
	}
	l, err := e.Lists.ByID(ctx, m.ListID)
	if err != nil {
		return nil, fmt.Errorf("%w: list gone", outbound.ErrSkip)
	}
	// The link belongs to this signup: after an unsubscribe and a new
	// signup, an old confirmation mail confirms nothing.
	link := e.BaseURL(l) + "/confirm/" + e.Tokens.Make(token.Confirm, m.ID, m.SignupAt.Unix())
	lang := l.Language
	intro := fmt.Sprintf(l.Text("confirm.intro"), l.Name)
	action := l.Text("confirm.action")
	ignore := l.Text("confirm.ignore")
	expect := ""
	if l.Cadence != "" {
		expect = fmt.Sprintf(l.Text("confirm.cadence"), l.Cadence)
	}
	md := intro + "\n\n[" + action + " →](" + link + ")\n\n" + expect + ignore
	doc, _ := emailmd.Render(md)
	footer := template.HTMLEscapeString(l.Name)
	if l.Address != "" {
		footer += "<br>" + template.HTMLEscapeString(l.Address)
	}
	if l.ImprintURL != "" {
		footer += `<br><a href="` + template.HTMLEscapeString(l.ImprintURL) + `" style="color:#52525b;">` + template.HTMLEscapeString(l.Text("footer.imprint")) + "</a>"
	}
	html, err := emailmd.Email(emailmd.Layout{Lang: lang, Title: fmt.Sprintf(l.Text("confirm.subject"), l.Name), Brand: l.Name,
		Body: template.HTML(emailmd.Inline(doc.HTML, emailmd.DefaultTheme)), FooterHTML: template.HTML(footer)})
	if err != nil {
		return nil, err
	}
	text := intro + "\n\n" + action + ":\n" + link + "\n\n" + expect + ignore + "\n\n-- \n" + l.Name
	if l.Address != "" {
		text += "\n" + l.Address
	}
	return msg.Compose(msg.Draft{
		From: msg.Address{Name: l.FromName, Email: l.From}, To: []msg.Address{{Email: m.Email}},
		Subject: fmt.Sprintf(l.Text("confirm.subject"), l.Name), Date: e.Opts.Now(),
		MessageID: e.Tokens.Make(token.Message, r.ID) + ".c@" + l.Domain,
		Text:      text, HTML: html,
		Header: [][2]string{{"Auto-Submitted", "auto-generated"}, {"Form-Sub", formSub(m.SignupIP)}},
	})
}

// unsubscribeWords are whole replies that mean "take me off this list".
var unsubscribeWords = map[string]bool{"unsubscribe": true, "stop": true, "remove": true, "remove me": true, "unsubscribe me": true,
	"abmelden": true, "abbestellen": true, "austragen": true, "bitte abmelden": true, "abmeldung": true}

// replyUnsubscribe honors "unsubscribe" sent to a list's sender or reply-to
// address (M3AAWG 4.6, lfr:E055): a reply whose subject or first line is the
// word, from an address that passes DMARC, leaves every list using rcpt.
// The message stays in the mailbox, so the owner sees it.
func (e *Engine) replyUnsubscribe(ctx context.Context, rcpt string, env *inbound.Envelope) {
	p := env.Parsed
	if p == nil || p.From.Email == "" {
		return
	}
	// Only a DMARC pass proves the reply comes from the reader. Without it
	// the reader still gets what they asked for: their own unsubscribe
	// link, by mail to the address on the list, once a day at most. A
	// forged request can do no more than send a reader their own link.
	authentic := env.Auth.DMARC == "pass"
	subject := strings.ToLower(strings.TrimSpace(replyPrefix.ReplaceAllString(p.Subject, "")))
	first := ""
	for _, line := range strings.Split(p.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			first = strings.ToLower(strings.Trim(line, ".!"))
			break
		}
	}
	if !unsubscribeWords[strings.Trim(subject, ".!")] && !unsubscribeWords[first] {
		return
	}
	ls, err := e.Lists.All(ctx)
	if err != nil {
		return
	}
	sender := strings.ToLower(p.From.Email)
	for _, l := range ls {
		if !strings.EqualFold(l.From, rcpt) && !strings.EqualFold(l.ReplyTo, rcpt) {
			continue
		}
		m, err := e.Lists.Find(ctx, l.Slug, sender)
		if err != nil || m.Status == "unsubscribed" {
			continue
		}
		if authentic {
			if _, err := e.Lists.Unsubscribe(ctx, m.ID, 0, "reply"); err != nil {
				e.Log.Warn("unsubscribe by reply", "list", l.Slug, "err", err)
			}
			continue
		}
		e.sendUnsubLink(ctx, l, m)
	}
}

// sendUnsubLink answers an unverified unsubscribe reply with the reader's
// one-click link.
func (e *Engine) sendUnsubLink(ctx context.Context, l *lists.List, m *lists.Member) {
	e.mu.Lock()
	if e.unsubLinks == nil {
		e.unsubLinks = map[int64]time.Time{}
	}
	last, seen := e.unsubLinks[m.ID]
	now := e.Opts.Now()
	if seen && now.Sub(last) < 24*time.Hour {
		e.mu.Unlock()
		return
	}
	e.unsubLinks[m.ID] = now
	e.mu.Unlock()
	link := e.BaseURL(l) + "/u/" + e.Tokens.Make(token.Unsub, m.ID, 0)
	_, err := e.Mailbox.Send(ctx, e.Queue, mailbox.SendRequest{From: l.From, To: []string{m.Email},
		Subject: fmt.Sprintf(l.Text("unsub.reply.subject"), l.Name),
		Text:    fmt.Sprintf(l.Text("unsub.reply.body"), l.Name, link)})
	if err != nil {
		e.Log.Warn("unsubscribe link by mail", "list", l.Slug, "err", err)
	}
}

var replyPrefix = regexp.MustCompile(`(?i)^\s*((re|aw|antw|sv|wg|fw|fwd)\s*(\[\d+\])?\s*:\s*)+`)

// formSub is the Form-Sub header of a confirmation request: the message
// answers a web form, submitted from this (partly masked) address, so
// receivers can spot list bombing (draft-levine-mailbomb-header, lfr:E052).
func formSub(ip string) string {
	addr, err := netip.ParseAddr(ip)
	switch {
	case err != nil:
		return "v=1; ip=none"
	case addr.Is4() || addr.Is4In6():
		b := addr.Unmap().As4()
		return fmt.Sprintf("v=1; ip4=%d.%d.x.x", b[0], b[1])
	}
	b := addr.As16()
	return fmt.Sprintf("v=1; ip6=%x:%x:%x::x", uint16(b[0])<<8|uint16(b[1]), uint16(b[2])<<8|uint16(b[3]), uint16(b[4])<<8|uint16(b[5]))
}
