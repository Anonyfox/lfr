package mailbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Anonyfox/lfr/internal/mailauth"
	"github.com/Anonyfox/lfr/internal/msg"
	"github.com/Anonyfox/lfr/internal/store"
)

// Summary is a message as listed.
type Summary struct {
	ID          int64     `json:"id"`
	Mailbox     string    `json:"mailbox"`
	Folder      string    `json:"folder"`
	From        string    `json:"from"`
	FromName    string    `json:"from_name,omitempty"`
	To          []string  `json:"to,omitempty"`
	Cc          []string  `json:"cc,omitempty"`
	Subject     string    `json:"subject"`
	Snippet     string    `json:"snippet"`
	Date        time.Time `json:"date"`
	Received    time.Time `json:"received"`
	Seen        bool      `json:"seen"`
	Flagged     bool      `json:"flagged"`
	Answered    bool      `json:"answered"`
	Attachments int       `json:"attachments"`
	Spam        float64   `json:"spam"`
	Thread      string    `json:"thread"`
	Size        int       `json:"size"`
	MessageID   string    `json:"message_id,omitempty"`
}

// Message is a message as read.
type Message struct {
	Summary
	ReplyTo string           `json:"reply_to,omitempty"`
	Text    string           `json:"text"`
	HTML    string           `json:"html,omitempty"`
	Files   []msg.Attachment `json:"attachments_list,omitempty"`
	Verdict json.RawMessage  `json:"verdict,omitempty"` // authentication and spam details
	// SenderCheck says in one line whether the From address is proven: a
	// "Rechnung" from an unproven sender is the classic phishing mail.
	SenderCheck string `json:"sender_check,omitempty"`
	// Events summarizes calendar invitations: what, when, where.
	Events     []string `json:"events,omitempty"`
	Trained    string   `json:"trained,omitempty"`
	Blob       string   `json:"-"`
	Features   string   `json:"-"`
	InReplyTo  string   `json:"in_reply_to,omitempty"`
	References []string `json:"references,omitempty"`
}

// Delivery is one message to store.
type Delivery struct {
	MailboxID int64
	Folder    string
	Raw       []byte
	Parsed    *msg.Parsed // parsed when nil
	Spam      float64
	Verdict   string
	Features  string
	Seen      bool
}

const maxIndexedBody = 100 << 10

// Deliver stores a message in a mailbox and indexes it for search.
func (s *Service) Deliver(ctx context.Context, d Delivery) (int64, error) {
	p := d.Parsed
	if p == nil {
		p = msg.Parse(d.Raw)
	}
	hash, err := s.Blobs.Put(d.Raw)
	if err != nil {
		return 0, err
	}
	now := s.Now()
	date := p.Date
	if date.IsZero() || date.After(now.Add(24*time.Hour)) {
		date = now
	}
	var to, cc []string
	for _, a := range p.To {
		to = append(to, a.Email)
	}
	for _, a := range p.Cc {
		cc = append(cc, a.Email)
	}
	var replyTo string
	if len(p.ReplyTo) > 0 {
		replyTo = p.ReplyTo[0].Email
	}
	folder := d.Folder
	if folder == "" {
		folder = "inbox"
	}
	var id int64
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		thread := s.thread(ctx, tx, d.MailboxID, p)
		res, err := tx.ExecContext(ctx, `INSERT INTO messages(mailbox_id, folder, blob, size, received, date, message_id, in_reply_to, refs, thread,
			from_addr, from_name, to_addrs, cc_addrs, reply_to, subject, snippet, attachments, seen, spam, verdict, features)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.MailboxID, folder, hash, len(d.Raw), now.Unix(), date.Unix(), p.MessageID, p.InReplyTo, strings.Join(p.References, " "), thread,
			p.From.Email, p.From.Name, strings.Join(to, ","), strings.Join(cc, ","), replyTo, p.Subject, msg.Snippet(p.Text, 200),
			len(p.Attachments), d.Seen, d.Spam, d.Verdict, d.Features)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		people := strings.Join(append(append([]string{p.From.Name, p.From.Email}, to...), cc...), " ")
		body := p.Text
		if len(body) > maxIndexedBody {
			body = body[:maxIndexedBody]
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO messages_fts(rowid, subject, people, body) VALUES(?, ?, ?, ?)", id, p.Subject, people, body)
		return err
	})
	if err != nil {
		s.dropUnreferenced(ctx, []string{hash})
		return 0, err
	}
	return id, nil
}

// thread picks the conversation a message belongs to: that of a message it
// answers, else the first reference, else itself.
func (s *Service) thread(ctx context.Context, tx *sql.Tx, mailboxID int64, p *msg.Parsed) string {
	refs := append([]string{}, p.References...)
	if p.InReplyTo != "" {
		refs = append(refs, p.InReplyTo)
	}
	for i := len(refs) - 1; i >= 0; i-- {
		var t string
		if tx.QueryRowContext(ctx, "SELECT thread FROM messages WHERE mailbox_id = ? AND message_id = ? LIMIT 1", mailboxID, refs[i]).Scan(&t) == nil && t != "" {
			return t
		}
	}
	if len(refs) > 0 {
		return refs[0]
	}
	if p.MessageID != "" {
		return p.MessageID
	}
	return fmt.Sprintf("lfr-%d", s.Now().UnixNano())
}

const selectSummary = `SELECT m.id, b.local || '@' || d.name, m.folder, m.from_addr, m.from_name, m.to_addrs, m.cc_addrs, m.subject, m.snippet,
	m.date, m.received, m.seen, m.flagged, m.answered, m.attachments, m.spam, m.thread, m.size, m.message_id
	FROM messages m JOIN mailboxes b ON b.id = m.mailbox_id JOIN domains d ON d.id = b.domain_id`

func scanSummary(row interface{ Scan(...any) error }) (Summary, error) {
	var m Summary
	var to, cc string
	var date, received int64
	err := row.Scan(&m.ID, &m.Mailbox, &m.Folder, &m.From, &m.FromName, &to, &cc, &m.Subject, &m.Snippet,
		&date, &received, &m.Seen, &m.Flagged, &m.Answered, &m.Attachments, &m.Spam, &m.Thread, &m.Size, &m.MessageID)
	if to != "" {
		m.To = strings.Split(to, ",")
	}
	if cc != "" {
		m.Cc = strings.Split(cc, ",")
	}
	m.Date = time.Unix(date, 0).UTC()
	m.Received = time.Unix(received, 0).UTC()
	return m, err
}

// Query selects messages to list.
type Query struct {
	Mailbox string // address; empty for every mailbox
	Folder  string // empty: inbox
	Unread  bool
	Thread  string
	Search  string // full-text query
	Limit   int
	Before  int64 // only messages with a smaller ID, for paging
}

// Messages returns matching messages, newest first.
func (s *Service) Messages(ctx context.Context, q Query) ([]Summary, error) {
	var where []string
	var args []any
	if q.Mailbox != "" {
		mb, err := s.Get(ctx, q.Mailbox)
		if err != nil {
			return nil, err
		}
		where = append(where, "m.mailbox_id = ?")
		args = append(args, mb.ID)
	}
	switch {
	case q.Folder == "all":
	case q.Folder != "":
		f, ok := ValidFolder(q.Folder)
		if !ok {
			return nil, fmt.Errorf("unknown folder %q; folders are %s", q.Folder, strings.Join(Folders, ", "))
		}
		where = append(where, "m.folder = ?")
		args = append(args, f)
	case q.Search == "" && q.Thread == "":
		where = append(where, "m.folder = 'inbox'")
	default:
		where = append(where, "m.folder != 'trash'")
	}
	if q.Unread {
		where = append(where, "m.seen = 0")
	}
	if q.Thread != "" {
		where = append(where, "m.thread = ?")
		args = append(args, q.Thread)
	}
	if q.Before > 0 {
		where = append(where, "m.id < ?")
		args = append(args, q.Before)
	}
	query := selectSummary
	if q.Search != "" {
		// Newest first, like every mail client: ranking every hit by
		// relevance costs a second on a few thousand messages.
		where = append(where, "m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)")
		args = append(args, ftsQuery(q.Search))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY m.id DESC"
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 50
	}
	query += fmt.Sprintf(" LIMIT %d", q.Limit)
	rows, err := s.DB.R.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		m, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ftsQuery turns free text into a safe FTS5 query: every word must match as
// a prefix; from:, to: and subject: limit a word to a column; quoted phrases stay phrases.
func ftsQuery(s string) string {
	var terms []string
	for _, tok := range tokenize(s) {
		col := ""
		for _, prefix := range []string{"from:", "to:", "subject:"} {
			if strings.HasPrefix(strings.ToLower(tok), prefix) {
				col = map[string]string{"from:": "people", "to:": "people", "subject:": "subject"}[prefix]
				tok = tok[len(prefix):]
			}
		}
		phrase := strings.HasPrefix(tok, `"`)
		tok = strings.Trim(tok, `"`)
		words := strings.FieldsFunc(tok, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if len(words) == 0 {
			continue
		}
		var t string
		if phrase || len(words) > 1 {
			t = `"` + strings.Join(words, " ") + `"`
		} else {
			t = `"` + words[0] + `"*`
		}
		if col != "" {
			t = col + ":" + t
		}
		terms = append(terms, t)
	}
	if len(terms) == 0 {
		return `""`
	}
	return strings.Join(terms, " AND ")
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			cur.WriteRune(r)
			if quoted {
				out = append(out, cur.String())
				cur.Reset()
			}
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// Read returns a full message; seen marks it read.
func (s *Service) Read(ctx context.Context, id int64, seen bool) (*Message, error) {
	var m Message
	var err error
	row := s.DB.R.QueryRowContext(ctx, `SELECT m.id, b.local || '@' || d.name, m.folder, m.from_addr, m.from_name, m.to_addrs, m.cc_addrs, m.subject, m.snippet,
		m.date, m.received, m.seen, m.flagged, m.answered, m.attachments, m.spam, m.thread, m.size, m.message_id,
		m.reply_to, m.verdict, m.trained, m.blob, m.features, m.in_reply_to, m.refs
		FROM messages m JOIN mailboxes b ON b.id = m.mailbox_id JOIN domains d ON d.id = b.domain_id WHERE m.id = ?`, id)
	var to, cc, refs, verdict string
	var date, received int64
	err = row.Scan(&m.ID, &m.Mailbox, &m.Folder, &m.From, &m.FromName, &to, &cc, &m.Subject, &m.Snippet,
		&date, &received, &m.Seen, &m.Flagged, &m.Answered, &m.Attachments, &m.Spam, &m.Thread, &m.Size, &m.MessageID,
		&m.ReplyTo, &verdict, &m.Trained, &m.Blob, &m.Features, &m.InReplyTo, &refs)
	if verdict != "" {
		m.Verdict = json.RawMessage(verdict)
		m.SenderCheck = senderCheck(verdict)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("message %d: %w", id, store.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if to != "" {
		m.To = strings.Split(to, ",")
	}
	if cc != "" {
		m.Cc = strings.Split(cc, ",")
	}
	m.References = strings.Fields(refs)
	m.Date = time.Unix(date, 0).UTC()
	m.Received = time.Unix(received, 0).UTC()
	raw, err := s.Blobs.Get(m.Blob)
	if err != nil {
		return nil, fmt.Errorf("message %d: stored content is missing: %w", id, err)
	}
	p := msg.Parse(raw)
	m.Text, m.HTML, m.Files = p.Text, p.HTML, p.Attachments
	for _, part := range p.Parts {
		if strings.HasPrefix(strings.ToLower(part.Type), "text/calendar") {
			if ev := calendarEvent(part.Body); ev != "" {
				m.Events = append(m.Events, ev)
			}
		}
	}
	if seen && !m.Seen {
		s.DB.W.ExecContext(ctx, "UPDATE messages SET seen = 1 WHERE id = ?", id)
		m.Seen = true
	}
	return &m, nil
}

// Raw returns the stored message as received or sent.
func (s *Service) Raw(ctx context.Context, id int64) ([]byte, error) {
	var hash string
	err := s.DB.R.QueryRowContext(ctx, "SELECT blob FROM messages WHERE id = ?", id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("message %d: %w", id, store.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return s.Blobs.Get(hash)
}

// Move puts a message into folder and returns the folder it came from.
func (s *Service) Move(ctx context.Context, id int64, folder string) (string, error) {
	f, ok := ValidFolder(folder)
	if !ok {
		return "", fmt.Errorf("unknown folder %q; folders are %s", folder, strings.Join(Folders, ", "))
	}
	var old string
	err := s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT folder FROM messages WHERE id = ?", id).Scan(&old); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("message %d: %w", id, store.ErrNotFound)
			}
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE messages SET folder = ? WHERE id = ?", f, id)
		return err
	})
	return old, err
}

// SetTrained records how a message was used to train the spam filter.
func (s *Service) SetTrained(ctx context.Context, id int64, label string) error {
	_, err := s.DB.W.ExecContext(ctx, "UPDATE messages SET trained = ? WHERE id = ?", label, id)
	return err
}

// Flag sets seen, flagged or answered.
func (s *Service) Flag(ctx context.Context, id int64, flag string, on bool) error {
	col := map[string]string{"seen": "seen", "read": "seen", "flagged": "flagged", "answered": "answered"}[flag]
	if col == "" {
		return fmt.Errorf("unknown flag %q; flags are seen, flagged, answered", flag)
	}
	res, err := s.DB.W.ExecContext(ctx, "UPDATE messages SET "+col+" = ? WHERE id = ?", on, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("message %d: %w", id, store.ErrNotFound)
	}
	return nil
}

// Delete moves a message to trash, or removes it for good when purge is set
// or it already is in trash.
func (s *Service) Delete(ctx context.Context, id int64, purge bool) error {
	var folder, hash string
	err := s.DB.R.QueryRowContext(ctx, "SELECT folder, blob FROM messages WHERE id = ?", id).Scan(&folder, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("message %d: %w", id, store.ErrNotFound)
	}
	if err != nil {
		return err
	}
	if !purge && folder != "trash" {
		_, err := s.Move(ctx, id, "trash")
		return err
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM messages_fts WHERE rowid = ?", id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM messages WHERE id = ?", id)
		return err
	})
	if err == nil {
		s.dropUnreferenced(ctx, []string{hash})
	}
	return err
}

// Purge removes messages that sat in folder longer than age, returning how many.
func (s *Service) Purge(ctx context.Context, folder string, age time.Duration) (int, error) {
	cutoff := s.Now().Add(-age).Unix()
	rows, err := s.DB.R.QueryContext(ctx, "SELECT id FROM messages WHERE folder = ? AND received < ? LIMIT 5000", folder, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.Delete(ctx, id, true); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// Counts returns message counts per folder of a mailbox.
func (s *Service) Counts(ctx context.Context, mailboxID int64) (map[string]int, error) {
	rows, err := s.DB.R.QueryContext(ctx, "SELECT folder, count(*) FROM messages WHERE mailbox_id = ? GROUP BY folder", mailboxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var f string
		var n int
		rows.Scan(&f, &n)
		out[f] = n
	}
	return out, rows.Err()
}

// senderCheck reads a stored verdict into one line about the From address.
// DMARC decides when the domain publishes it; without a record, an SPF or
// DKIM pass for the From domain still proves the sender.
func senderCheck(verdict string) string {
	var v struct {
		Auth mailauth.Result `json:"auth"`
	}
	if json.Unmarshal([]byte(verdict), &v) != nil || v.Auth.FromDomain == "" {
		return ""
	}
	a, d := v.Auth, v.Auth.FromDomain
	switch {
	case a.DMARC == "pass":
		return "verified: the mail really comes from " + d + " (DMARC pass)"
	case a.DMARC == "none" && (a.DKIMPass(d) || a.SPF == "pass" && mailauth.Aligned(a.SPFDomain, d, false)):
		return "verified: " + d + " signed or sent it (SPF/DKIM pass; the domain publishes no DMARC)"
	case a.DMARC == "fail":
		return "FORGED: " + d + " says mail without its signature is not from it (DMARC fail); do not trust links, attachments or requests"
	}
	return "not verified: nothing proves the mail comes from " + d + " (DMARC " + a.DMARC + "); treat links, attachments and payment requests with care"
}

// calendarEvent summarizes the first event of an iCalendar body.
func calendarEvent(body []byte) string {
	fields := map[string]string{}
	// Unfold continuation lines (RFC 5545 §3.1), then read NAME[;params]:value.
	text := strings.NewReplacer("\r\n ", "", "\r\n\t", "", "\n ", "").Replace(string(body))
	inEvent := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "BEGIN:VEVENT":
			inEvent = true
		case line == "END:VEVENT":
			inEvent = false
		case inEvent:
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			name, _, _ = strings.Cut(name, ";")
			if _, seen := fields[name]; !seen {
				fields[name] = strings.NewReplacer(`\,`, ",", `\;`, ";", `\n`, " ").Replace(value)
			}
		}
	}
	if fields["SUMMARY"] == "" && fields["DTSTART"] == "" {
		return ""
	}
	out := fields["SUMMARY"]
	if t := icsTime(fields["DTSTART"]); t != "" {
		out += ", " + t
		if e := icsTime(fields["DTEND"]); e != "" {
			out += " until " + e
		}
	}
	if fields["LOCATION"] != "" {
		out += ", " + fields["LOCATION"]
	}
	return out
}

func icsTime(v string) string {
	for _, layout := range []string{"20060102T150405Z", "20060102T150405", "20060102"} {
		if t, err := time.Parse(layout, v); err == nil {
			switch layout {
			case "20060102":
				return t.Format("Mon 2006-01-02")
			case "20060102T150405Z":
				return t.Local().Format("Mon 2006-01-02 15:04 MST")
			}
			return t.Format("Mon 2006-01-02 15:04")
		}
	}
	return ""
}
