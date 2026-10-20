// Package issues is the newsletter itself: issues written in
// Markdown, checked before sending, rendered per recipient with signed
// unsubscribe, click and open links, queued to every confirmed member, and
// counted afterwards.
package issues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/lists"
	"github.com/Anonyfox/lfr/internal/outbound"
	"github.com/Anonyfox/lfr/internal/store"
)

// Issue is one issue of a list.
type Issue struct {
	ID        int64     `json:"id"`
	ListID    int64     `json:"-"`
	List      string    `json:"list"`
	Slug      string    `json:"slug"`
	Subject   string    `json:"subject"`
	Preheader string    `json:"preheader,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Markdown  string    `json:"markdown,omitempty"`
	Status    string    `json:"status"`
	Scheduled time.Time `json:"scheduled,omitzero"`
	Started   time.Time `json:"started,omitzero"`
	Finished  time.Time `json:"finished,omitzero"`
	Tested    time.Time `json:"tested,omitzero"` // when this content last went out as a test copy
	Welcome   bool      `json:"welcome,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
}

// Service stores and sends issues.
type Service struct {
	DB     *store.DB
	Lists  *lists.Service
	Events *events.Bus
	Queue  *outbound.Queue
	Blobs  *blob.Store // images of issues
	Now    func() time.Time
}

// Draft is the content of an issue; empty fields come from the Markdown.
type Draft struct {
	Markdown  string
	Subject   string
	Slug      string
	Preheader string
	Summary   string
}

// FrontMatter splits optional "---" key: value lines from the body.
func FrontMatter(src string) (map[string]string, string) { return emailmd.FrontMatter(src) }

// Slugify makes a URL path segment from a title.
func Slugify(s string) string {
	s = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "ae", "Ö", "oe", "Ü", "ue", "ß", "ss").Replace(s)
	s = norm.NFKD.String(s)
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case unicode.Is(unicode.Mn, r):
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= 60 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func (s *Service) resolve(d Draft) (Draft, error) {
	meta, body := FrontMatter(d.Markdown)
	pick := func(explicit, key string) string {
		if explicit != "" {
			return explicit
		}
		return meta[key]
	}
	out := Draft{Markdown: body, Subject: pick(d.Subject, "subject"), Slug: pick(d.Slug, "slug"), Preheader: pick(d.Preheader, "preheader"), Summary: pick(d.Summary, "summary")}
	if strings.TrimSpace(out.Markdown) == "" {
		return out, errors.New("the issue has no content")
	}
	doc, err := emailmd.Render(out.Markdown)
	if err != nil {
		return out, err
	}
	if out.Subject == "" {
		out.Subject = doc.Title
	}
	if out.Subject == "" {
		return out, errors.New("no subject: add a first heading, front matter \"subject:\" or --subject")
	}
	if out.Summary == "" {
		out.Summary = doc.Summary
	}
	if out.Slug == "" {
		out.Slug = Slugify(out.Subject)
	}
	out.Slug = strings.ToLower(out.Slug)
	if !slugRE.MatchString(out.Slug) {
		return out, fmt.Errorf("slug %q: use lowercase letters, digits and dashes", out.Slug)
	}
	return out, nil
}

// Create adds a draft issue to a list.
func (s *Service) Create(ctx context.Context, listSlug string, d Draft) (*Issue, error) {
	l, err := s.Lists.Get(ctx, listSlug)
	if err != nil {
		return nil, err
	}
	d, err = s.resolve(d)
	if err != nil {
		return nil, err
	}
	now := s.Now().Unix()
	var id int64
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		slug := d.Slug
		for i := 2; ; i++ {
			var n int
			tx.QueryRowContext(ctx, "SELECT count(*) FROM issues WHERE list_id = ? AND slug = ?", l.ID, slug).Scan(&n)
			if n == 0 {
				break
			}
			slug = fmt.Sprintf("%s-%d", d.Slug, i)
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO issues(list_id, slug, subject, preheader, summary, markdown, status, created, updated)
			VALUES(?, ?, ?, ?, ?, ?, 'draft', ?, ?)`, l.ID, slug, d.Subject, d.Preheader, d.Summary, d.Markdown, now, now)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Update replaces the content of a draft or scheduled issue.
func (s *Service) Update(ctx context.Context, id int64, d Draft) (*Issue, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != "draft" && w.Status != "scheduled" {
		return nil, fmt.Errorf("issue %d is %s; only drafts and scheduled issues can change", id, w.Status)
	}
	if d.Markdown == "" {
		d.Markdown = w.Markdown
	}
	d, err = s.resolve(d)
	if err != nil {
		return nil, err
	}
	var n int
	s.DB.R.QueryRowContext(ctx, "SELECT count(*) FROM issues WHERE list_id = ? AND slug = ? AND id != ?", w.ListID, d.Slug, id).Scan(&n)
	if n > 0 {
		return nil, fmt.Errorf("slug %s is taken on this list", d.Slug)
	}
	// A test copy proves the content it carried; changed content needs a new one.
	_, err = s.DB.W.ExecContext(ctx, `UPDATE issues SET tested = CASE WHEN subject = ? AND preheader = ? AND markdown = ? THEN tested ELSE 0 END,
		slug = ?, subject = ?, preheader = ?, summary = ?, markdown = ?, updated = ? WHERE id = ?`,
		d.Subject, d.Preheader, d.Markdown, d.Slug, d.Subject, d.Preheader, d.Summary, d.Markdown, s.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Put creates or replaces the issue with d's slug on a list, so a file on
// disk and its issue on the server stay one thing. An issue that went out
// cannot change. welcome makes it the list's welcome issue; there is one.
func (s *Service) Put(ctx context.Context, listSlug string, d Draft, welcome bool) (w *Issue, created bool, err error) {
	l, err := s.Lists.Get(ctx, listSlug)
	if err != nil {
		return nil, false, err
	}
	if d, err = s.resolve(d); err != nil {
		return nil, false, err
	}
	old, err := s.BySlug(ctx, l.ID, d.Slug)
	switch {
	case errors.Is(err, store.ErrNotFound):
		now := s.Now().Unix()
		var id int64
		err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
			if welcome {
				if _, err := tx.ExecContext(ctx, "UPDATE issues SET welcome = 0 WHERE list_id = ? AND welcome = 1", l.ID); err != nil {
					return err
				}
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO issues(list_id, slug, subject, preheader, summary, markdown, status, welcome, created, updated)
				VALUES(?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?)`, l.ID, d.Slug, d.Subject, d.Preheader, d.Summary, d.Markdown, welcome, now, now)
			if err == nil {
				id, _ = res.LastInsertId()
			}
			return err
		})
		if err != nil {
			return nil, false, err
		}
		w, err = s.Get(ctx, id)
		return w, true, err
	case err != nil:
		return nil, false, err
	}
	if old.Status != "draft" && old.Status != "scheduled" {
		return nil, false, fmt.Errorf("issue %s is %s and cannot change; give the file a new slug for a new issue", d.Slug, old.Status)
	}
	if old.Welcome != welcome {
		err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
			if welcome {
				if _, err := tx.ExecContext(ctx, "UPDATE issues SET welcome = 0 WHERE list_id = ? AND welcome = 1", l.ID); err != nil {
					return err
				}
			}
			_, err := tx.ExecContext(ctx, "UPDATE issues SET welcome = ?, status = 'draft', scheduled = 0 WHERE id = ?", welcome, old.ID)
			return err
		})
		if err != nil {
			return nil, false, err
		}
	}
	w, err = s.Update(ctx, old.ID, d)
	return w, false, err
}

// Welcome queues the list's welcome issue to a member who just confirmed.
// Each address gets it once.
func (s *Service) Welcome(ctx context.Context, l *lists.List, m *lists.Member) error {
	var id int64
	err := s.DB.R.QueryRowContext(ctx, "SELECT id FROM issues WHERE list_id = ? AND welcome = 1", l.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	// A paused list holds its queue; the welcome goes out on resume.
	if err != nil {
		return err
	}
	return s.Queue.Enqueue(ctx, outbound.Item{Kind: outbound.KindIssue, DomainID: l.DomainID, IssueID: id, ContactID: m.ContactID, MemberID: m.ID, Rcpt: m.Email})
}

// Crowding returns another issue of the list that starts within the list's
// minimum gap of at, or nil.
func (s *Service) Crowding(ctx context.Context, id int64, at time.Time) (*Issue, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	l, err := s.Lists.ByID(ctx, w.ListID)
	if err != nil || l.MinGap <= 0 {
		return nil, err
	}
	if at.IsZero() || at.Before(s.Now()) {
		at = s.Now()
	}
	gap := int64(l.MinGap)
	other, err := scanIssue(s.DB.R.QueryRowContext(ctx, selectIssue+` WHERE w.list_id = ? AND w.id != ? AND w.welcome = 0
		AND w.status IN ('scheduled', 'sending', 'sent') AND abs(coalesce(nullif(w.started, 0), w.scheduled) - ?) < ?
		ORDER BY abs(coalesce(nullif(w.started, 0), w.scheduled) - ?) LIMIT 1`, l.ID, id, at.Unix(), gap, at.Unix()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return other, err
}

const selectIssue = `SELECT w.id, w.list_id, l.slug, w.slug, w.subject, w.preheader, w.summary, w.markdown, w.status, w.scheduled, w.started, w.finished, w.tested, w.welcome, w.created, w.updated
	FROM issues w JOIN lists l ON l.id = w.list_id`

func scanIssue(row interface{ Scan(...any) error }) (*Issue, error) {
	var w Issue
	var sch, st, fin, tst, cr, up int64
	err := row.Scan(&w.ID, &w.ListID, &w.List, &w.Slug, &w.Subject, &w.Preheader, &w.Summary, &w.Markdown, &w.Status, &sch, &st, &fin, &tst, &w.Welcome, &cr, &up)
	unix := func(v int64) time.Time {
		if v == 0 {
			return time.Time{}
		}
		return time.Unix(v, 0).UTC()
	}
	w.Scheduled, w.Started, w.Finished, w.Tested, w.Created, w.Updated = unix(sch), unix(st), unix(fin), unix(tst), unix(cr), unix(up)
	return &w, err
}

// Get loads an issue.
func (s *Service) Get(ctx context.Context, id int64) (*Issue, error) {
	w, err := scanIssue(s.DB.R.QueryRowContext(ctx, selectIssue+" WHERE w.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("issue %d: %w; see lfr issue ls", id, store.ErrNotFound)
	}
	return w, err
}

// BySlug loads an issue of a list.
func (s *Service) BySlug(ctx context.Context, listID int64, slug string) (*Issue, error) {
	w, err := scanIssue(s.DB.R.QueryRowContext(ctx, selectIssue+" WHERE w.list_id = ? AND w.slug = ?", listID, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return w, err
}

// List returns the issues of a list, newest first; sentOnly limits to published ones.
func (s *Service) List(ctx context.Context, listID int64, sentOnly bool, limit int) ([]*Issue, error) {
	q := selectIssue + " WHERE w.list_id = ?"
	if sentOnly {
		q += " AND w.status IN ('sending', 'sent')"
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.R.QueryContext(ctx, q+" ORDER BY coalesce(nullif(w.started, 0), w.created) DESC, w.id DESC LIMIT ?", listID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Issue
	for rows.Next() {
		w, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Remove deletes an issue that is not being sent.
func (s *Service) Remove(ctx context.Context, id int64) error {
	w, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if w.Status == "sending" {
		return fmt.Errorf("issue %d is sending; cancel it first", id)
	}
	_, err = s.DB.W.ExecContext(ctx, "DELETE FROM issues WHERE id = ?", id)
	return err
}

// Schedule makes a draft go out at at (now or earlier: immediately).
func (s *Service) Schedule(ctx context.Context, id int64, at time.Time) (*Issue, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Status != "draft" && w.Status != "scheduled" {
		return nil, fmt.Errorf("issue %d is already %s", id, w.Status)
	}
	if w.Welcome {
		return nil, fmt.Errorf("issue %d is the welcome issue; every new member gets it when they confirm", id)
	}
	l, err := s.Lists.ByID(ctx, w.ListID)
	if err != nil {
		return nil, err
	}
	if l.Paused != "" {
		return nil, fmt.Errorf("list %s is paused: %s", l.Slug, l.Paused)
	}
	if at.IsZero() || at.Before(s.Now()) {
		at = s.Now()
	}
	if _, err := s.DB.W.ExecContext(ctx, "UPDATE issues SET status = 'scheduled', scheduled = ? WHERE id = ?", at.Unix(), id); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Cancel stops a scheduled or sending issue. A scheduled issue returns to
// draft; a sending issue keeps what went out and drops the rest.
func (s *Service) Cancel(ctx context.Context, id int64) (*Issue, error) {
	w, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	switch w.Status {
	case "scheduled":
		_, err = s.DB.W.ExecContext(ctx, "UPDATE issues SET status = 'draft', scheduled = 0 WHERE id = ?", id)
	case "sending":
		err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE outbound SET status = 'canceled', last_error = 'issue canceled' WHERE issue_id = ? AND status = 'queued'", id); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, "UPDATE issues SET status = 'canceled', finished = ? WHERE id = ?", s.Now().Unix(), id)
			return err
		})
	default:
		return nil, fmt.Errorf("issue %d is %s; nothing to cancel", id, w.Status)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// start queues the issue to every confirmed, unsuppressed member at once.
func (s *Service) start(ctx context.Context, w *Issue) (int, error) {
	l, err := s.Lists.ByID(ctx, w.ListID)
	if err != nil {
		return 0, err
	}
	if l.Paused != "" {
		return 0, nil // stays scheduled until the list resumes
	}
	now := s.Now().Unix()
	var n int64
	var ev events.Event
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO outbound(kind, domain_id, issue_id, contact_id, member_id, env_from, rcpt, status, next_at, created)
			SELECT 'issue', ?, ?, c.id, m.id, '', c.email, 'queued', ?, ?
			FROM members m JOIN contacts c ON c.id = m.contact_id
			WHERE m.list_id = ? AND m.status = 'confirmed' AND c.suppressed = 0
			ORDER BY m.id
			ON CONFLICT DO NOTHING`, l.DomainID, w.ID, now, now, l.ID)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		if _, err := tx.ExecContext(ctx, "UPDATE issues SET status = 'sending', started = ? WHERE id = ?", now, w.ID); err != nil {
			return err
		}
		ev = events.Event{Kind: events.IssueStarted, ListID: l.ID, IssueID: w.ID, Detail: fmt.Sprintf("%d recipients", n)}
		return s.Events.RecordTx(ctx, tx, &ev)
	})
	if err != nil {
		return 0, err
	}
	s.Events.Publish(ev)
	s.Queue.Wake()
	return int(n), nil
}

// Tick starts due issues and marks finished ones. The engine calls it every few seconds.
// MaxLate is how far past its slot a scheduled issue still starts by
// itself, after the server was down. Later, its text may no longer fit the
// day ("this weekend"), so it waits for the owner (Missed).
const MaxLate = 12 * time.Hour

// Missed returns scheduled issues held because their slot passed more than
// MaxLate ago.
func (s *Service) Missed(ctx context.Context) ([]*Issue, error) {
	rows, err := s.DB.R.QueryContext(ctx, selectIssue+" WHERE w.status = 'scheduled' AND w.scheduled <= ?", s.Now().Add(-MaxLate).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Issue
	for rows.Next() {
		if w, err := scanIssue(rows); err == nil {
			out = append(out, w)
		}
	}
	return out, rows.Err()
}

func (s *Service) Tick(ctx context.Context) error {
	now := s.Now()
	rows, err := s.DB.R.QueryContext(ctx, selectIssue+" WHERE w.status = 'scheduled' AND w.scheduled <= ? AND w.scheduled > ?", now.Unix(), now.Add(-MaxLate).Unix())
	if err != nil {
		return err
	}
	var due []*Issue
	for rows.Next() {
		w, err := scanIssue(rows)
		if err == nil {
			due = append(due, w)
		}
	}
	rows.Close()
	for _, w := range due {
		if _, err := s.start(ctx, w); err != nil {
			return err
		}
	}
	rows, err = s.DB.R.QueryContext(ctx, `SELECT w.id, w.list_id FROM issues w WHERE w.status = 'sending'
		AND NOT EXISTS (SELECT 1 FROM outbound o WHERE o.issue_id = w.id AND o.status = 'queued')`)
	if err != nil {
		return err
	}
	type done struct{ id, list int64 }
	var finished []done
	for rows.Next() {
		var d done
		rows.Scan(&d.id, &d.list)
		finished = append(finished, d)
	}
	rows.Close()
	for _, d := range finished {
		res, err := s.DB.W.ExecContext(ctx, "UPDATE issues SET status = 'sent', finished = ? WHERE id = ? AND status = 'sending'", s.Now().Unix(), d.id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // finished by an earlier tick
		}
		st, _ := s.Stats(ctx, d.id)
		s.Events.Record(ctx, events.Event{Kind: events.IssueFinished, ListID: d.list, IssueID: d.id,
			Detail: fmt.Sprintf("%d sent, %d bounced, %d failed", st.Sent, st.Bounced, st.Failed)})
	}
	return nil
}

// Archive freezes the numbers of issues finished before cutoff and deletes
// their per-recipient rows and events, which otherwise grow without bound.
func (s *Service) Archive(ctx context.Context, cutoff time.Time) (int, error) {
	rows, err := s.DB.R.QueryContext(ctx, "SELECT id FROM issues WHERE status IN ('sent', 'canceled') AND stats = '' AND finished > 0 AND finished < ?", cutoff.Unix())
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
		st, err := s.Stats(ctx, id)
		if err != nil {
			return 0, err
		}
		b, _ := json.Marshal(st)
		err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE issues SET stats = ? WHERE id = ?", string(b), id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM outbound WHERE issue_id = ? AND status != 'queued'", id); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, "DELETE FROM events WHERE issue_id = ? AND kind IN ('opened', 'clicked', 'sent', 'deferred', 'failed', 'bounced')", id)
			return err
		})
		if err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// Stats counts what happened to an issue.
type Stats struct {
	Recipients   int            `json:"recipients"`
	Queued       int            `json:"queued"`
	Sent         int            `json:"sent"`
	Bounced      int            `json:"bounced"`
	Failed       int            `json:"failed"`
	Canceled     int            `json:"canceled"`
	Opened       int            `json:"opened"`  // unique recipients, includes machine opens
	Clicked      int            `json:"clicked"` // unique recipients
	Unsubscribed int            `json:"unsubscribed"`
	Complained   int            `json:"complained"`
	Replies      int            `json:"replies"`               // distinct senders who answered in the list's mailbox
	Links        map[string]int `json:"link_clicks,omitempty"` // unique clicks per URL
	Tracking     bool           `json:"tracking"`              // opens and clicks were measured
	ReplyList    []Reply        `json:"reply_list,omitempty"`  // the replies themselves, newest first
}

// Reply is one reader's answer to an issue, in the list's mailbox.
type Reply struct {
	ID      int64  `json:"id"` // message ID for lfr mail read
	From    string `json:"from"`
	Snippet string `json:"snippet"`
}

// Stats reports an issue's numbers.
func (s *Service) Stats(ctx context.Context, id int64) (Stats, error) {
	var st Stats
	var snapshot string
	s.DB.R.QueryRowContext(ctx, "SELECT stats FROM issues WHERE id = ?", id).Scan(&snapshot)
	if snapshot != "" {
		return st, json.Unmarshal([]byte(snapshot), &st)
	}
	err := s.DB.R.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE status IN ('queued', 'sending')), count(*) FILTER (WHERE status = 'sent'), count(*) FILTER (WHERE status = 'bounced'),
		count(*) FILTER (WHERE status = 'failed'), count(*) FILTER (WHERE status = 'canceled')
		FROM outbound WHERE issue_id = ? AND kind = 'issue' AND contact_id IS NOT NULL`, id).Scan(&st.Recipients, &st.Queued, &st.Sent, &st.Bounced, &st.Failed, &st.Canceled)
	if err != nil {
		return st, err
	}
	if err := s.DB.R.QueryRowContext(ctx, "SELECT l.tracking FROM issues w JOIN lists l ON l.id = w.list_id WHERE w.id = ?", id).Scan(&st.Tracking); err != nil {
		return st, err
	}
	s.DB.R.QueryRowContext(ctx, `SELECT
		(SELECT count(DISTINCT contact_id) FROM events WHERE issue_id = ?1 AND kind = 'opened'),
		(SELECT count(DISTINCT contact_id) FROM events WHERE issue_id = ?1 AND kind = 'clicked'),
		(SELECT count(*) FROM events WHERE issue_id = ?1 AND kind = 'unsubscribed' AND detail NOT LIKE '% via complaint'), -- complaints count once, as complaints
		(SELECT count(*) FROM events WHERE issue_id = ?1 AND kind = 'complained')`, id).Scan(&st.Opened, &st.Clicked, &st.Unsubscribed, &st.Complained)
	st.ReplyList = s.replies(ctx, id)
	st.Replies = len(st.ReplyList)
	rows, err := s.DB.R.QueryContext(ctx, "SELECT detail, count(DISTINCT contact_id) FROM events WHERE issue_id = ? AND kind = 'clicked' GROUP BY detail ORDER BY 2 DESC LIMIT 50", id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var u string
			var n int
			rows.Scan(&u, &n)
			if st.Links == nil {
				st.Links = map[string]int{}
			}
			st.Links[u] = n
		}
	}
	return st, nil
}

var replyPrefix = regexp.MustCompile(`(?i)^\s*((re|aw|antw|sv|wg|fw|fwd)\s*(\[\d+\])?\s*:\s*)+`)

// replies counts the people who answered an issue: messages in the sender's
// mailbox, received after the issue started, whose subject is the issue's
// behind a reply prefix.
func (s *Service) replies(ctx context.Context, id int64) []Reply {
	rows, err := s.DB.R.QueryContext(ctx, `SELECT m.id, m.subject, m.from_addr, m.snippet, w.subject FROM messages m
		JOIN mailboxes mb ON mb.id = m.mailbox_id
		JOIN lists l ON l.domain_id = mb.domain_id AND l.from_local = mb.local
		JOIN issues w ON w.list_id = l.id
		WHERE w.id = ? AND w.started > 0 AND m.received >= w.started AND m.folder != 'sent' AND m.folder != 'junk'
		AND instr(lower(m.subject), lower(w.subject)) > 0 ORDER BY m.id DESC`, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []Reply
	for rows.Next() {
		var r Reply
		var subject, issue string
		if rows.Scan(&r.ID, &subject, &r.From, &r.Snippet, &issue) != nil {
			continue
		}
		// One per reader: the latest answer.
		if p := replyPrefix.FindString(subject); p != "" && strings.EqualFold(strings.TrimSpace(subject[len(p):]), strings.TrimSpace(issue)) && !seen[strings.ToLower(r.From)] {
			seen[strings.ToLower(r.From)] = true
			out = append(out, r)
		}
	}
	return out
}
