package lists

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/store"
)

// Member is one address on one list.
type Member struct {
	ID         int64     `json:"id"`
	ListID     int64     `json:"-"`
	List       string    `json:"list"`
	ContactID  int64     `json:"contact_id"`
	Email      string    `json:"email"`
	Name       string    `json:"name,omitempty"`
	Status     string    `json:"status"`
	Source     string    `json:"source,omitempty"`
	Consent    string    `json:"consent,omitempty"`
	SignupAt   time.Time `json:"signup_at"`
	SignupIP   string    `json:"signup_ip,omitempty"`
	ConfirmAt  time.Time `json:"confirm_at,omitzero"`
	ConfirmIP  string    `json:"confirm_ip,omitempty"`
	UnsubAt    time.Time `json:"unsubscribed_at,omitzero"`
	UnsubHow   string    `json:"unsubscribed_how,omitempty"`
	Suppressed string    `json:"suppressed,omitempty"` // why the address gets nothing at all
}

// Signup describes where a subscription came from: the consent proof.
type Signup struct {
	IP, UserAgent, Source, Consent string
	// From an import: when and from where the reader opted in on the old
	// platform, so consent proof and footers keep the real date.
	At, ConfirmedAt time.Time
	ConfirmIP       string
	// Reconfirm asks readers already on the list to confirm again, for a
	// stale import: they turn pending and get a confirmation mail.
	Reconfirm bool
}

const selectMember = `SELECT m.id, m.list_id, l.slug, c.id, c.email, c.name, m.status, m.source, m.consent, m.signup_at, m.signup_ip,
	m.confirm_at, m.confirm_ip, m.unsub_at, m.unsub_how, c.suppress_reason
	FROM members m JOIN contacts c ON c.id = m.contact_id JOIN lists l ON l.id = m.list_id`

func scanMember(row interface{ Scan(...any) error }) (*Member, error) {
	var m Member
	var signup, confirm, unsub int64
	err := row.Scan(&m.ID, &m.ListID, &m.List, &m.ContactID, &m.Email, &m.Name, &m.Status, &m.Source, &m.Consent, &signup, &m.SignupIP,
		&confirm, &m.ConfirmIP, &unsub, &m.UnsubHow, &m.Suppressed)
	m.SignupAt = time.Unix(signup, 0).UTC()
	if confirm > 0 {
		m.ConfirmAt = time.Unix(confirm, 0).UTC()
	}
	if unsub > 0 {
		m.UnsubAt = time.Unix(unsub, 0).UTC()
	}
	return &m, err
}

// Member loads a membership by ID.
func (s *Service) Member(ctx context.Context, id int64) (*Member, error) {
	m, err := scanMember(s.DB.R.QueryRowContext(ctx, selectMember+" WHERE m.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("member %d: %w", id, store.ErrNotFound)
	}
	return m, err
}

// Find loads the membership of email on a list.
func (s *Service) Find(ctx context.Context, slug, email string) (*Member, error) {
	e, err := addr.Email(email)
	if err != nil {
		return nil, err
	}
	m, err := scanMember(s.DB.R.QueryRowContext(ctx, selectMember+" WHERE l.slug = ? AND c.email = ?", strings.ToLower(slug), e))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%s on %s: %w", e, slug, store.ErrNotFound)
	}
	return m, err
}

func (s *Service) contact(ctx context.Context, tx *sql.Tx, email, name string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM contacts WHERE email = ?", email).Scan(&id)
	if err == nil {
		if name != "" {
			tx.ExecContext(ctx, "UPDATE contacts SET name = ? WHERE id = ? AND name = ''", name, id)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO contacts(email, name, created) VALUES(?, ?, ?)", email, name, s.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ConfirmCooldown is the least time between two confirmation mails to one
// address for one list; MaxConfirms caps them per address over
// ConfirmWindow, across all lists.
const (
	ConfirmCooldown = time.Hour
	MaxConfirms     = 3
	ConfirmWindow   = 30 * 24 * time.Hour
)

// ConfirmValidity is how long a confirmation link works.
const ConfirmValidity = 30 * 24 * time.Hour

// ErrOptedOut means an address left, complained, bounced or was erased, so
// only a new double opt-in may add it.
var ErrOptedOut = errors.New("unsubscribed, complained, bounced or erased; only a new signup with confirmation adds it again")

// forgottenHash identifies an erased address without keeping it.
func forgottenHash(email string) string {
	sum := sha256.Sum256([]byte("lfr forgotten\x00" + strings.ToLower(email)))
	return hex.EncodeToString(sum[:])
}

// readerEmail checks an address a list can deliver to. Local parts beyond
// ASCII need SMTPUTF8 on every hop, which lfr's sending side does not
// negotiate; such a reader would sign up and never get a single issue.
func readerEmail(email string) (string, error) {
	e, err := addr.Email(email)
	if err != nil {
		return "", err
	}
	for _, r := range e[:strings.LastIndexByte(e, '@')] {
		if r > 127 {
			return "", addr.Invalid("invalid address %q: characters beyond ASCII before the @ cannot receive newsletters yet", email)
		}
	}
	return e, nil
}

// ErrExpired means a confirmation link is too old.
var ErrExpired = errors.New("this confirmation link has expired; sign up again")

// Subscribe records a signup. It returns the membership and whether a
// confirmation mail should go out now. Signing up again while confirmed
// changes nothing; while pending it resends the confirmation after the
// cooldown; after unsubscribing it starts over with a new confirmation.
func (s *Service) Subscribe(ctx context.Context, l *List, email string, in Signup) (*Member, bool, error) {
	e, err := readerEmail(email)
	if err != nil {
		return nil, false, err
	}
	now := s.Now()
	if in.Consent == "" {
		in.Consent = l.ConsentText()
	}
	var id int64
	send := false
	var ev *events.Event
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		cid, err := s.contact(ctx, tx, e, "")
		if err != nil {
			return err
		}
		var status string
		var confirmSent int64
		err = tx.QueryRowContext(ctx, "SELECT id, status, confirm_sent FROM members WHERE list_id = ? AND contact_id = ?", l.ID, cid).Scan(&id, &status, &confirmSent)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			var res sql.Result
			res, err = tx.ExecContext(ctx, `INSERT INTO members(list_id, contact_id, status, source, consent, signup_at, signup_ip, signup_ua, confirm_sent)
				VALUES(?, ?, 'pending', ?, ?, ?, ?, ?, ?)`, l.ID, cid, in.Source, in.Consent, now.Unix(), in.IP, trim(in.UserAgent, 300), now.Unix())
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			send = true
		case err != nil:
			return err
		case status == "confirmed" && in.Reconfirm:
			_, err = tx.ExecContext(ctx, `UPDATE members SET status = 'pending', source = ?, consent = ?, signup_at = ?, signup_ip = ?, signup_ua = '',
				confirm_at = 0, confirm_ip = '', confirm_sent = ? WHERE id = ?`, in.Source, in.Consent, now.Unix(), in.IP, now.Unix(), id)
			send = true
		case status == "confirmed":
			err = nil
		case status == "pending":
			if now.Sub(time.Unix(confirmSent, 0)) >= ConfirmCooldown {
				// The proof of consent follows the signup that is confirmed.
				send = true
				_, err = tx.ExecContext(ctx, "UPDATE members SET confirm_sent = ?, source = ?, consent = ?, signup_ip = ?, signup_ua = ? WHERE id = ?",
					now.Unix(), in.Source, in.Consent, in.IP, trim(in.UserAgent, 300), id)
			}
		default: // unsubscribed: a new signup with fresh consent
			_, err = tx.ExecContext(ctx, `UPDATE members SET status = 'pending', source = ?, consent = ?, signup_at = ?, signup_ip = ?, signup_ua = ?,
				confirm_at = 0, confirm_ip = '', unsub_at = 0, unsub_how = '', confirm_sent = ? WHERE id = ?`,
				in.Source, in.Consent, now.Unix(), in.IP, trim(in.UserAgent, 300), now.Unix(), id)
			send = true
		}
		if err != nil {
			return err
		}
		if send {
			// However many forms someone fills in with this address, it
			// gets at most a few confirmation requests a month: lfr must not
			// become a way to flood an inbox (subscription bombing).
			var recent int
			tx.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE contact_id = ? AND kind = ? AND at > ?",
				cid, events.Subscribed, now.Add(-ConfirmWindow).Unix()).Scan(&recent)
			if recent >= MaxConfirms {
				send = false
				return nil
			}
			ev = &events.Event{Kind: events.Subscribed, ListID: l.ID, ContactID: cid, Detail: e}
			return s.Events.RecordTx(ctx, tx, ev)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if ev != nil {
		s.Events.Publish(*ev)
	}
	m, err := s.Member(ctx, id)
	return m, send, err
}

// Confirm completes a double opt-in. Confirming twice is harmless.
func (s *Service) Confirm(ctx context.Context, memberID int64, ip string) (*Member, error) {
	m, err := s.Member(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if m.Status == "confirmed" {
		return m, nil
	}
	if m.Status == "unsubscribed" {
		return nil, errors.New("this subscription was cancelled; sign up again")
	}
	var sent int64
	s.DB.R.QueryRowContext(ctx, "SELECT max(confirm_sent, signup_at) FROM members WHERE id = ?", m.ID).Scan(&sent)
	if s.Now().Sub(time.Unix(sent, 0)) > ConfirmValidity {
		return nil, ErrExpired
	}
	now := s.Now().Unix()
	var ev events.Event
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE members SET status = 'confirmed', confirm_at = ?, confirm_ip = ? WHERE id = ?", now, ip, m.ID); err != nil {
			return err
		}
		// A confirmed signup is fresh consent: earlier bounces, complaints
		// and erasure no longer apply. An operator's block stays.
		if _, err := tx.ExecContext(ctx, "UPDATE contacts SET suppressed = 0, suppress_reason = '' WHERE id = ? AND suppress_reason NOT LIKE 'blocked%'", m.ContactID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM forgotten WHERE hash = ?", forgottenHash(m.Email)); err != nil {
			return err
		}
		ev = events.Event{Kind: events.Confirmed, ListID: m.ListID, ContactID: m.ContactID, Detail: m.Email}
		return s.Events.RecordTx(ctx, tx, &ev)
	})
	if err != nil {
		return nil, err
	}
	s.Events.Publish(ev)
	return s.Member(ctx, memberID)
}

// Unsubscribe ends a membership; how records the channel (link, header, mail, operator).
func (s *Service) Unsubscribe(ctx context.Context, memberID, issueID int64, how string) (*Member, error) {
	m, err := s.Member(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if m.Status == "unsubscribed" {
		return m, nil
	}
	var ev events.Event
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE members SET status = 'unsubscribed', unsub_at = ?, unsub_how = ? WHERE id = ?", s.Now().Unix(), how, m.ID); err != nil {
			return err
		}
		// Stop anything still queued for this list.
		if _, err := tx.ExecContext(ctx, `UPDATE outbound SET status = 'canceled', last_error = 'unsubscribed'
			WHERE status = 'queued' AND contact_id = ? AND issue_id IN (SELECT id FROM issues WHERE list_id = ?)`, m.ContactID, m.ListID); err != nil {
			return err
		}
		ev = events.Event{Kind: events.Unsubscribed, ListID: m.ListID, IssueID: issueID, ContactID: m.ContactID, Detail: m.Email + " via " + how}
		return s.Events.RecordTx(ctx, tx, &ev)
	})
	if err != nil {
		return nil, err
	}
	s.Events.Publish(ev)
	return s.Member(ctx, memberID)
}

// Add puts an address on a list directly. confirmed asserts that consent
// exists outside lfr (e.g. a migrated list); otherwise it is a normal
// pending signup and the second return value says to send a confirmation.
func (s *Service) Add(ctx context.Context, l *List, email, name string, confirmed bool, in Signup) (*Member, bool, error) {
	if !confirmed {
		// The operator adds these, not the readers themselves: an address
		// that bounced, complained or was blocked gets no mail at all.
		var suppressed int
		s.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM contacts WHERE email = ? AND suppressed != 0", strings.ToLower(strings.TrimSpace(email))).Scan(&suppressed)
		if suppressed > 0 {
			return nil, false, ErrOptedOut
		}
		return s.Subscribe(ctx, l, email, in)
	}
	e, err := readerEmail(email)
	if err != nil {
		return nil, false, err
	}
	now := s.Now().Unix()
	var id int64
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		// Someone who left, complained, bounced or asked to be erased stays
		// off: an import asserts old consent, which their objection ended
		// (GDPR Art. 7(3), Art. 21(3); CAN-SPAM §7704(a)(4)). Only a new
		// double opt-in brings them back.
		var n int
		tx.QueryRowContext(ctx, "SELECT count(*) FROM forgotten WHERE hash = ?", forgottenHash(e)).Scan(&n)
		if n > 0 {
			return ErrOptedOut
		}
		tx.QueryRowContext(ctx, `SELECT count(*) FROM contacts c LEFT JOIN members m ON m.contact_id = c.id AND m.list_id = ?
			WHERE c.email = ? AND (c.suppressed != 0 OR m.status = 'unsubscribed')`, l.ID, e).Scan(&n)
		if n > 0 {
			return ErrOptedOut
		}
		cid, err := s.contact(ctx, tx, e, strings.TrimSpace(name))
		if err != nil {
			return err
		}
		signedUp, confirmedAt := now, now
		if !in.At.IsZero() {
			signedUp, confirmedAt = in.At.Unix(), in.At.Unix()
		}
		if !in.ConfirmedAt.IsZero() {
			confirmedAt = in.ConfirmedAt.Unix()
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO members(list_id, contact_id, status, source, consent, signup_at, signup_ip, confirm_at, confirm_ip)
			VALUES(?, ?, 'confirmed', ?, ?, ?, ?, ?, ?)
			ON CONFLICT(list_id, contact_id) DO UPDATE SET status = 'confirmed', confirm_at = excluded.confirm_at, confirm_ip = excluded.confirm_ip,
				source = excluded.source, consent = excluded.consent, signup_at = excluded.signup_at, signup_ip = excluded.signup_ip
			WHERE members.status = 'pending'`,
			l.ID, cid, in.Source, in.Consent, signedUp, in.IP, confirmedAt, in.ConfirmIP)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			id, _ = res.LastInsertId()
		}
		return tx.QueryRowContext(ctx, "SELECT id FROM members WHERE list_id = ? AND contact_id = ?", l.ID, cid).Scan(&id)
	})
	if err != nil {
		return nil, false, err
	}
	m, err := s.Member(ctx, id)
	return m, false, err
}

// MarkConfirmSent records that a confirmation mail was queued.
func (s *Service) MarkConfirmSent(ctx context.Context, memberID int64) {
	s.DB.W.ExecContext(ctx, "UPDATE members SET confirm_sent = ? WHERE id = ?", s.Now().Unix(), memberID)
}

// ConfirmsLastHour counts confirmation mails a list sent in the last hour.
func (s *Service) ConfirmsLastHour(ctx context.Context, listID int64) int {
	var n int
	s.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM members WHERE list_id = ? AND confirm_sent >= ?", listID, s.Now().Add(-time.Hour).Unix()).Scan(&n)
	return n
}

// Query selects members.
type Query struct {
	Status string // pending, confirmed, unsubscribed; empty for all
	Search string // part of the address
	Limit  int
	After  int64
}

// Members lists the members of a list.
func (s *Service) Members(ctx context.Context, slug string, q Query) ([]*Member, error) {
	l, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	where := "WHERE m.list_id = ? AND m.id > ?"
	args := []any{l.ID, q.After}
	if q.Status != "" {
		where += " AND m.status = ?"
		args = append(args, q.Status)
	}
	if q.Search != "" {
		where += " AND c.email LIKE ? ESCAPE '\\'"
		args = append(args, "%"+strings.NewReplacer("%", "\\%", "_", "\\_").Replace(strings.ToLower(q.Search))+"%")
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	args = append(args, q.Limit)
	rows, err := s.DB.R.QueryContext(ctx, selectMember+" "+where+" ORDER BY m.id LIMIT ?", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Member
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Suppress stops all mail to an address, on every list.
func (s *Service) Suppress(ctx context.Context, email, reason string) error {
	e, err := addr.Email(email)
	if err != nil {
		return err
	}
	return s.DB.Tx(ctx, func(tx *sql.Tx) error {
		cid, err := s.contact(ctx, tx, e, "")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE contacts SET suppressed = ?, suppress_reason = ? WHERE id = ?", s.Now().Unix(), reason, cid); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE outbound SET status = 'canceled', last_error = 'suppressed: ' || ? WHERE status = 'queued' AND contact_id = ? AND kind != 'mail'", reason, cid)
		return err
	})
}

// Unsuppress lets an address receive mail again.
func (s *Service) Unsuppress(ctx context.Context, email string) error {
	e, err := addr.Email(email)
	if err != nil {
		return err
	}
	res, err := s.DB.W.ExecContext(ctx, "UPDATE contacts SET suppressed = 0, suppress_reason = '' WHERE email = ?", e)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("contact %s: %w", e, store.ErrNotFound)
	}
	return nil
}

// Forget erases an address and everything recorded about it (GDPR art. 17).
func (s *Service) Forget(ctx context.Context, email string) error {
	e, err := addr.Email(email)
	if err != nil {
		return err
	}
	return s.DB.Tx(ctx, func(tx *sql.Tx) error {
		var cid int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM contacts WHERE email = ?", e).Scan(&cid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("contact %s: %w", e, store.ErrNotFound)
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM events WHERE contact_id = ?", cid); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM outbound WHERE contact_id = ? OR rcpt = ?", cid, e); err != nil {
			return err
		}
		// Events about the address that were not tied to the contact, such
		// as relay notifications, mention it in their detail.
		if _, err := tx.ExecContext(ctx, "DELETE FROM events WHERE instr(lower(detail), ?) > 0", strings.ToLower(e)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO forgotten(hash, at) VALUES(?, ?) ON CONFLICT DO NOTHING", forgottenHash(e), s.Now().Unix()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM contacts WHERE id = ?", cid)
		return err
	})
}

// Contact is an address with its memberships.
type Contact struct {
	Email      string    `json:"email"`
	Name       string    `json:"name,omitempty"`
	Created    time.Time `json:"created"`
	Suppressed string    `json:"suppressed,omitempty"`
	Lists      []*Member `json:"lists"`
	Sent       int       `json:"sent"`
	Opens      int       `json:"opens"`
	Clicks     int       `json:"clicks"`
}

// Contact shows everything lfr knows about an address.
func (s *Service) Contact(ctx context.Context, email string) (*Contact, error) {
	e, err := addr.Email(email)
	if err != nil {
		return nil, err
	}
	var c Contact
	var id, created int64
	err = s.DB.R.QueryRowContext(ctx, "SELECT id, email, name, created, suppress_reason FROM contacts WHERE email = ?", e).Scan(&id, &c.Email, &c.Name, &created, &c.Suppressed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("contact %s: %w", e, store.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	c.Created = time.Unix(created, 0).UTC()
	rows, err := s.DB.R.QueryContext(ctx, selectMember+" WHERE c.id = ? ORDER BY l.slug", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		c.Lists = append(c.Lists, m)
	}
	s.DB.R.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM outbound WHERE contact_id = ?1 AND status = 'sent'),
		(SELECT count(*) FROM events WHERE contact_id = ?1 AND kind = 'opened'),
		(SELECT count(*) FROM events WHERE contact_id = ?1 AND kind = 'clicked')`, id).Scan(&c.Sent, &c.Opens, &c.Clicks)
	return &c, rows.Err()
}

// ExportCSV writes the members of a list with their consent records.
func (s *Service) ExportCSV(ctx context.Context, slug string, w io.Writer) (int, error) {
	cw := csv.NewWriter(w)
	cw.Write([]string{"email", "name", "status", "source", "signup_at", "signup_ip", "confirm_at", "confirm_ip", "unsubscribed_at", "consent"})
	var after int64
	n := 0
	for {
		ms, err := s.Members(ctx, slug, Query{After: after, Limit: 1000})
		if err != nil {
			return n, err
		}
		if len(ms) == 0 {
			break
		}
		for _, m := range ms {
			cw.Write([]string{m.Email, m.Name, m.Status, m.Source, ts(m.SignupAt), m.SignupIP, ts(m.ConfirmAt), m.ConfirmIP, ts(m.UnsubAt), m.Consent})
			after = m.ID
			n++
		}
	}
	cw.Flush()
	return n, cw.Error()
}

func ts(t time.Time) string {
	if t.IsZero() || t.Unix() == 0 {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ImportResult counts an import.
type ImportResult struct {
	Added     int      `json:"added"`
	Existing  int      `json:"existing"`
	OptedOut  int      `json:"opted_out"` // left, complained, bounced or erased: kept off
	Reconfirm int      `json:"reconfirm"` // on the list already, asked to confirm again
	Pending   []int64  `json:"-"`         // memberships that need a confirmation mail
	Invalid   []string `json:"invalid,omitempty"`
}

// ImportCSV reads addresses (first column, or a column named email; an
// optional name column) and adds them. Without confirmed every address gets
// a confirmation request instead.
func (s *Service) ImportCSV(ctx context.Context, l *List, r io.Reader, confirmed bool, in Signup) (ImportResult, error) {
	var res ImportResult
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	emailCol, nameCol, lastCol, atCol, confirmCol, ipCol, confirmIPCol := 0, -1, -1, -1, -1, -1, -1
	first := true
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		if len(rec) == 0 {
			continue
		}
		if first {
			first = false
			header := false
			for i, h := range rec {
				switch strings.ToLower(strings.TrimSpace(h)) {
				case "email", "e-mail", "email address", "mail":
					emailCol, header = i, true
				case "name", "full name", "first name", "first_name", "fname":
					nameCol = i
				case "last name", "last_name", "lname":
					lastCol = i
				case "optin_time", "opt-in time", "signup_date", "signup date", "subscribed_at", "subscription date", "created_at", "created", "date_added", "date added":
					atCol = i
				case "confirm_time", "confirmed_at", "confirmation date":
					confirmCol = i
				case "optin_ip", "signup_ip", "ip", "ip_address", "ip address":
					ipCol = i
				case "confirm_ip":
					confirmIPCol = i
				}
			}
			if header {
				continue
			}
		}
		if emailCol >= len(rec) {
			continue
		}
		email := strings.TrimSpace(rec[emailCol])
		if email == "" {
			continue
		}
		cell := func(i int) string {
			if i >= 0 && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		name := strings.TrimSpace(cell(nameCol) + " " + cell(lastCol))
		row := in
		row.At, row.ConfirmedAt = csvTime(cell(atCol)), csvTime(cell(confirmCol))
		if ip := cell(ipCol); ip != "" {
			row.IP = ip
		}
		row.ConfirmIP = cell(confirmIPCol)
		before, _ := s.Find(ctx, l.Slug, email)
		m, send, err := s.Add(ctx, l, email, name, confirmed, row)
		if errors.Is(err, ErrOptedOut) {
			res.OptedOut++
			continue
		}
		if err != nil {
			res.Invalid = append(res.Invalid, email)
			continue
		}
		switch {
		case before != nil && before.Status == "confirmed" && m.Status == "pending":
			res.Reconfirm++
		case before != nil && before.Status == m.Status:
			res.Existing++
		default:
			res.Added++
		}
		if send {
			res.Pending = append(res.Pending, m.ID)
		}
	}
	return res, nil
}

// csvTime reads the dates platforms export; zero when there is none, or
// when it lies in the future.
func csvTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02", "01/02/2006"} {
		if t, err := time.Parse(layout, v); err == nil && t.Before(time.Now().Add(24*time.Hour)) {
			return t.UTC()
		}
	}
	return time.Time{}
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Soft bounce limits: an address whose deliveries keep failing temporarily
// is gone in practice. M3AAWG removes it after two consecutive bounces over
// two weeks or more, Spamhaus after three in a row (lfr:E054, lfr:E057).
const (
	SoftBounceLimit  = 3
	SoftBounceWindow = 14 * 24 * time.Hour
)

// SoftBounce counts a temporary failure that ended a delivery to a contact
// and suppresses the address once the limits are reached.
func (s *Service) SoftBounce(ctx context.Context, contactID int64, reason string) (suppressed bool, err error) {
	now := s.Now().Unix()
	var n int
	var first int64
	var email string
	err = s.DB.W.QueryRowContext(ctx, `UPDATE contacts SET soft_bounces = soft_bounces + 1,
		first_soft = CASE WHEN first_soft = 0 THEN ? ELSE first_soft END
		WHERE id = ? AND suppressed = 0 RETURNING soft_bounces, first_soft, email`, now, contactID).Scan(&n, &first, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if n >= SoftBounceLimit || n >= 2 && time.Duration(now-first)*time.Second >= SoftBounceWindow {
		return true, s.Suppress(ctx, email, fmt.Sprintf("%d soft bounces since %s: %s", n, time.Unix(first, 0).UTC().Format(time.DateOnly), reason))
	}
	return false, nil
}

// Engaged clears a contact's soft bounces: mail reached them.
func (s *Service) Engaged(ctx context.Context, contactID int64) {
	s.DB.W.ExecContext(ctx, "UPDATE contacts SET soft_bounces = 0, first_soft = 0 WHERE id = ? AND soft_bounces > 0", contactID)
}
