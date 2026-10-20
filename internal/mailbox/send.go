package mailbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/outbound"
)

// Outbox queues messages for delivery.
type Outbox interface {
	EnqueueTx(ctx context.Context, tx outbound.Execer, items ...outbound.Item) (int, error)
	Wake()
}

// SendRequest is a message to send from a mailbox.
type SendRequest struct {
	From       string     `json:"from"` // a mailbox address (or one of its aliases)
	To         []string   `json:"to"`
	Cc         []string   `json:"cc,omitempty"`
	Bcc        []string   `json:"bcc,omitempty"`
	ReplyTo    string     `json:"reply_to,omitempty"`
	Subject    string     `json:"subject"`
	Text       string     `json:"text"`
	HTML       string     `json:"html,omitempty"`
	Files      []msg.File `json:"-"`
	InReplyTo  string     `json:"-"`
	References []string   `json:"-"`
}

const maxRecipients = 100

// MaxMessage is the largest message lfr sends: Outlook's limit, below
// Gmail's 25 MB, counted after attachments are encoded.
const MaxMessage = 20 << 20

func parseList(field string, l []string) ([]msg.Address, error) {
	var out []msg.Address
	for _, s := range l {
		for _, part := range strings.Split(s, ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			a, err := addr.Parse(part)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", field, err)
			}
			name := ""
			if i := strings.Index(part, "<"); i > 0 {
				name = strings.Trim(strings.TrimSpace(part[:i]), `"`)
			}
			out = append(out, msg.Address{Name: name, Email: a.String()})
		}
	}
	return out, nil
}

// senderOf resolves From to a mailbox, accepting an alias of it.
func (s *Service) senderOf(ctx context.Context, from string) (*Mailbox, string, error) {
	a, err := addr.Parse(from)
	if err != nil {
		return nil, "", fmt.Errorf("from: %v", err)
	}
	id, ok, err := s.Resolve(ctx, a.String())
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", fmt.Errorf("from: %s is no mailbox or alias here; create it with lfr mailbox add %s", a, a)
	}
	mb, err := s.ByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	// Sending as a catch-all-only address would forge arbitrary senders: allow
	// only the mailbox itself and its aliases.
	if !strings.EqualFold(mb.Address, a.String()) && !contains(mb.Aliases, a.String()) && !strings.EqualFold(addr.Base(a.Local)+"@"+a.Domain, mb.Address) {
		return nil, "", fmt.Errorf("from: %s only reaches %s through the catch-all; send as %s", a, mb.Address, mb.Address)
	}
	return mb, a.String(), nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// Send composes a message, files it in the sender's sent folder and queues
// it for every recipient. It returns the ID of the sent-folder copy.
func (s *Service) Send(ctx context.Context, out Outbox, req SendRequest) (int64, error) {
	mb, from, err := s.senderOf(ctx, req.From)
	if err != nil {
		return 0, err
	}
	to, err := parseList("to", req.To)
	if err != nil {
		return 0, err
	}
	cc, err := parseList("cc", req.Cc)
	if err != nil {
		return 0, err
	}
	bcc, err := parseList("bcc", req.Bcc)
	if err != nil {
		return 0, err
	}
	all := append(append(append([]msg.Address{}, to...), cc...), bcc...)
	if len(all) == 0 {
		return 0, errors.New("no recipients")
	}
	if len(all) > maxRecipients {
		return 0, fmt.Errorf("%d recipients; at most %d per message (use a list and an issue for more)", len(all), maxRecipients)
	}
	if strings.TrimSpace(req.Subject) == "" {
		return 0, errors.New("no subject")
	}
	if strings.TrimSpace(req.Text) == "" && strings.TrimSpace(req.HTML) == "" {
		return 0, errors.New("empty body")
	}
	text := req.Text
	if text == "" {
		text = msg.HTMLToText(req.HTML)
	}
	d := msg.Draft{
		From: msg.Address{Name: mb.Name, Email: from}, To: to, Cc: cc,
		Subject: req.Subject, Date: s.Now(), Text: text, HTML: req.HTML, Files: req.Files,
		InReplyTo: req.InReplyTo, References: req.References,
	}
	if req.ReplyTo != "" {
		rt, err := parseList("reply-to", []string{req.ReplyTo})
		if err != nil {
			return 0, err
		}
		d.ReplyTo = rt
	}
	raw, err := msg.Compose(d)
	if err != nil {
		return 0, err
	}
	if len(raw) > MaxMessage {
		return 0, fmt.Errorf("the message is %d MB with its attachments encoded; Gmail accepts at most 25 MB and Outlook 20 MB, so it would bounce. Share large files as a link", len(raw)>>20)
	}
	id, err := s.Deliver(ctx, Delivery{MailboxID: mb.ID, Folder: "sent", Raw: raw, Seen: true})
	if err != nil {
		return 0, err
	}
	var hash string
	s.DB.R.QueryRowContext(ctx, "SELECT blob FROM messages WHERE id = ?", id).Scan(&hash)
	items := make([]outbound.Item, 0, len(all))
	seen := map[string]bool{}
	for _, a := range all {
		if seen[a.Email] {
			continue
		}
		seen[a.Email] = true
		items = append(items, outbound.Item{Kind: outbound.KindMail, DomainID: mb.DomainID, Blob: hash, EnvFrom: mb.Address, Rcpt: a.Email})
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		_, err := out.EnqueueTx(ctx, tx, items...)
		return err
	})
	if err != nil {
		return 0, err
	}
	out.Wake()
	return id, nil
}

// ReplyRequest answers a stored message.
type ReplyRequest struct {
	Text  string     `json:"text"`
	HTML  string     `json:"html,omitempty"`
	All   bool       `json:"all"`
	Quote bool       `json:"quote"`
	Files []msg.File `json:"-"`
}

// Reply answers message id from the mailbox that holds it.
func (s *Service) Reply(ctx context.Context, out Outbox, id int64, r ReplyRequest) (int64, error) {
	m, err := s.Read(ctx, id, false)
	if err != nil {
		return 0, err
	}
	if m.Folder == "sent" {
		return 0, fmt.Errorf("message %d is one you sent; reply to a received message", id)
	}
	target := m.From
	if m.ReplyTo != "" {
		target = m.ReplyTo
	}
	// Answer from the address the sender wrote to: a reply to hello@ comes
	// from hello@, when hello@ is an alias of this mailbox. Addresses that
	// only reach it through the catch-all cannot send, so those replies come
	// from the mailbox itself.
	from := m.Mailbox
	if own, ok, _ := s.Resolve(ctx, m.Mailbox); ok {
		for _, a := range append(append([]string{}, m.To...), m.Cc...) {
			p, err := addr.Parse(a)
			if err != nil {
				continue
			}
			base := addr.Base(p.Local) + "@" + p.Domain
			if mb, _, err := s.senderOf(ctx, base); err == nil && mb.ID == own {
				from = base
				break
			}
		}
	}
	req := SendRequest{From: from, To: []string{target}, Text: r.Text, HTML: r.HTML, Files: r.Files}
	if r.All {
		for _, a := range append(append([]string{}, m.To...), m.Cc...) {
			if !strings.EqualFold(a, m.Mailbox) && !strings.EqualFold(a, target) {
				req.Cc = append(req.Cc, a)
			}
		}
	}
	req.Subject = m.Subject
	if !strings.HasPrefix(strings.ToLower(req.Subject), "re:") {
		req.Subject = "Re: " + req.Subject
	}
	if m.MessageID != "" {
		req.InReplyTo = m.MessageID
		req.References = append(append([]string{}, m.References...), m.MessageID)
		if len(m.References) == 0 && m.InReplyTo != "" {
			req.References = []string{m.InReplyTo, m.MessageID}
		}
	}
	if r.Quote && req.Text != "" {
		who := m.From
		if m.FromName != "" {
			who = m.FromName + " <" + m.From + ">"
		}
		var q strings.Builder
		for _, line := range strings.Split(strings.TrimRight(m.Text, "\n"), "\n") {
			q.WriteString("> " + line + "\n")
		}
		said := "On " + m.Date.Format("Mon, 2 Jan 2006 15:04 MST") + ", " + who + " wrote:"
		req.Text = strings.TrimRight(req.Text, "\n") + "\n\n" + said + "\n" + q.String()
		if req.HTML != "" {
			quoted := strings.ReplaceAll(html.EscapeString(strings.TrimRight(m.Text, "\n")), "\n", "<br>")
			req.HTML += `<p style="margin:16px 0 8px;color:#52525b;">` + html.EscapeString(said) + `</p>` +
				`<blockquote style="margin:0;padding:0 0 0 12px;border-left:2px solid #d4d4d8;color:#52525b;">` + quoted + `</blockquote>`
		}
	}
	sent, err := s.Send(ctx, out, req)
	if err != nil {
		return 0, err
	}
	s.Flag(ctx, id, "answered", true)
	return sent, nil
}

// ForwardItem prepares a delivered message for the mailbox's forwarding
// address. The copy is sent as the mailbox (so SPF, DKIM and DMARC pass for
// lfr's domain) with the original sender as Reply-To.
func (s *Service) ForwardItem(ctx context.Context, mb *Mailbox, raw []byte, p *msg.Parsed) (outbound.Item, error) {
	name := p.From.Name
	if name == "" {
		name = p.From.Email
	}
	set := [][2]string{
		{"From", msg.FormatAddress(name+" via "+mb.Address, mb.Address)},
		{"X-Forwarded-For", strings.Join(append(p.Header.Values("X-Forwarded-For"), mb.Address), ", ")},
	}
	if p.MessageID == "" {
		set = append(set, [2]string{"Message-ID", "<" + msg.NewMessageID(addr.DomainOf(mb.Address)) + ">"})
	}
	if p.From.Email != "" && len(p.ReplyTo) == 0 {
		set = append(set, [2]string{"Reply-To", p.From.String()})
	}
	fwd, err := msg.Rewrite(raw, set, []string{"DKIM-Signature", "ARC-Seal", "ARC-Message-Signature", "ARC-Authentication-Results",
		"Return-Path", "Sender", "List-Unsubscribe", "List-Unsubscribe-Post"})
	if err != nil {
		return outbound.Item{}, err
	}
	hash, err := s.Blobs.Put(fwd)
	if err != nil {
		return outbound.Item{}, err
	}
	return outbound.Item{Kind: outbound.KindMail, DomainID: mb.DomainID, Blob: hash, EnvFrom: mb.Address, Rcpt: mb.Forward}, nil
}
