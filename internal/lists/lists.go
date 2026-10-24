// Package lists manages newsletter lists and their audience: contacts,
// memberships with double opt-in and stored consent proof, unsubscribes,
// suppression after bounces and complaints, and import and export.
package lists

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/events"
	"github.com/Anonyfox/lfr/internal/store"
)

// List is one newsletter.
type List struct {
	ID           int64     `json:"id"`
	DomainID     int64     `json:"-"`
	Domain       string    `json:"domain"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	From         string    `json:"from"` // sender address
	FromName     string    `json:"from_name"`
	ReplyTo      string    `json:"reply_to,omitempty"`
	Host         string    `json:"host,omitempty"`
	Language     string    `json:"language"`
	Tracking     bool      `json:"tracking"`
	ImprintURL   string    `json:"imprint_url,omitempty"`
	PrivacyURL   string    `json:"privacy_url,omitempty"`
	Address      string    `json:"address,omitempty"`
	Consent      string    `json:"consent,omitempty"`
	Paused       string    `json:"paused,omitempty"`
	Cadence      string    `json:"cadence,omitempty"`  // how often mail comes, shown at signup
	Headline     string    `json:"headline,omitempty"` // the website's first line; the name when empty
	Pitch        string    `json:"pitch,omitempty"`    // under the headline; the description when empty
	oldHosts     string    // former hosts, still served
	MinGap       Seconds   `json:"min_gap"` // time that must pass between two issues
	Created      time.Time `json:"created"`
	Confirmed    int       `json:"confirmed"`
	Pending      int       `json:"pending"`
	Unsubscribed int       `json:"unsubscribed"`

	// Texts is loaded with the list in the same query.
	Texts map[string]string `json:"texts,omitempty"`
}

// Seconds is a duration stored and shown in whole seconds.
type Seconds int64

// Duration converts to time.Duration.
func (s Seconds) Duration() time.Duration { return time.Duration(s) * time.Second }

// DefaultMinGap keeps two issues of a list from landing on the same day by
// accident; a list that sends daily sets it lower.
const DefaultMinGap = 20 * time.Hour

// ConsentText is the statement a signup agrees to.
func (l *List) ConsentText() string {
	if l.Consent != "" {
		return l.Consent
	}
	if l.Tracking {
		return fmt.Sprintf(l.Text("form.consent.tracked"), l.Name)
	}
	return fmt.Sprintf(l.Text("form.consent"), l.Name)
}

// ConsentsToTracking reports whether a consent text tells readers about open
// and click measurement. Only readers who agreed to such a text are
// measured (TDDDG §25, GDPR Art. 6).
func ConsentsToTracking(consent string) bool {
	c := strings.ToLower(consent)
	for _, w := range []string{"track", "statisti", "opened", "open rate", "click", "klick", "geöffnet", "öffnung", "messung", "auswert", "analy"} {
		if strings.Contains(c, w) {
			return true
		}
	}
	return false
}

// Service is the list store.
type Service struct {
	DB     *store.DB
	Events *events.Bus
	Now    func() time.Time
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Input sets list fields; nil pointers keep the current value.
type Input struct {
	Name, Description, From, FromName, ReplyTo, Host, Language *string
	Tracking                                                   *bool
	ImprintURL, PrivacyURL, Address, Consent, Cadence          *string
	Headline, Pitch                                            *string
	MinGap                                                     *time.Duration
}

const selectList = `SELECT l.id, l.domain_id, d.name, l.slug, l.name, l.description, l.from_local || '@' || d.name, l.from_name, l.reply_to, l.host,
	l.language, l.tracking, l.imprint_url, l.privacy_url, l.address, l.consent, l.paused, l.cadence, l.headline, l.pitch, l.old_hosts, l.min_gap, l.created,
	(SELECT count(*) FROM members WHERE list_id = l.id AND status = 'confirmed'),
	(SELECT count(*) FROM members WHERE list_id = l.id AND status = 'pending'),
	(SELECT count(*) FROM members WHERE list_id = l.id AND status = 'unsubscribed'),
	(SELECT json_group_object(key, value) FROM list_texts WHERE list_id = l.id)
	FROM lists l JOIN domains d ON d.id = l.domain_id`

// selectListLight skips the member counts, for lookups on hot paths
// (every web request, every recipient of an issue).
const selectListLight = `SELECT l.id, l.domain_id, d.name, l.slug, l.name, l.description, l.from_local || '@' || d.name, l.from_name, l.reply_to, l.host,
	l.language, l.tracking, l.imprint_url, l.privacy_url, l.address, l.consent, l.paused, l.cadence, l.headline, l.pitch, l.old_hosts, l.min_gap, l.created, 0, 0, 0,
	(SELECT json_group_object(key, value) FROM list_texts WHERE list_id = l.id)
	FROM lists l JOIN domains d ON d.id = l.domain_id`

func scanList(row interface{ Scan(...any) error }) (*List, error) {
	var l List
	var created int64
	var texts string
	err := row.Scan(&l.ID, &l.DomainID, &l.Domain, &l.Slug, &l.Name, &l.Description, &l.From, &l.FromName, &l.ReplyTo, &l.Host,
		&l.Language, &l.Tracking, &l.ImprintURL, &l.PrivacyURL, &l.Address, &l.Consent, &l.Paused, &l.Cadence, &l.Headline, &l.Pitch, &l.oldHosts, &l.MinGap, &created,
		&l.Confirmed, &l.Pending, &l.Unsubscribed, &texts)
	if err == nil {
		err = json.Unmarshal([]byte(texts), &l.Texts)
	}
	l.Created = time.Unix(created, 0).UTC()
	return &l, err
}

// Create adds a list sending from an address of a served domain.
func (s *Service) Create(ctx context.Context, slug string, in Input) (*List, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRE.MatchString(slug) {
		return nil, fmt.Errorf("list name %q: use lowercase letters, digits and dashes", slug)
	}
	if in.From == nil || *in.From == "" {
		return nil, errors.New("a list needs a sender address (--from news@example.com)")
	}
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
		return nil, errors.New("a list needs a display name (--name \"My Newsletter\")")
	}
	from, err := addr.Parse(*in.From)
	if err != nil {
		return nil, err
	}
	local, err := addr.Local(from.Local)
	if err != nil {
		return nil, err
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		var did int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM domains WHERE name = ?", from.Domain).Scan(&did); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("domain %s: %w; add it with lfr domain add %s", from.Domain, store.ErrNotFound, from.Domain)
			}
			return err
		}
		var n int
		tx.QueryRowContext(ctx, "SELECT count(*) FROM lists WHERE slug = ?", slug).Scan(&n)
		if n > 0 {
			return fmt.Errorf("list %s already exists", slug)
		}
		fromName := strings.TrimSpace(*in.Name)
		if in.FromName != nil && strings.TrimSpace(*in.FromName) != "" {
			fromName = strings.TrimSpace(*in.FromName)
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO lists(domain_id, slug, name, from_local, from_name, created) VALUES(?, ?, ?, ?, ?, ?)",
			did, slug, strings.TrimSpace(*in.Name), local, fromName, s.Now().Unix())
		return err
	})
	if err != nil {
		return nil, err
	}
	in.From = nil
	in.Name = nil
	return s.Update(ctx, slug, in)
}

// Update changes list fields.
func (s *Service) Update(ctx context.Context, slug string, in Input) (*List, error) {
	l, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	sets := map[string]any{}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return nil, errors.New("name cannot be empty")
		}
		sets["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		sets["description"] = strings.TrimSpace(*in.Description)
	}
	if in.From != nil {
		a, err := addr.Parse(*in.From)
		if err != nil {
			return nil, err
		}
		if a.Domain != l.Domain {
			return nil, fmt.Errorf("sender must stay on %s; create a new list for another domain", l.Domain)
		}
		local, err := addr.Local(a.Local)
		if err != nil {
			return nil, err
		}
		sets["from_local"] = local
	}
	if in.FromName != nil {
		sets["from_name"] = strings.TrimSpace(*in.FromName)
	}
	if in.ReplyTo != nil {
		v := strings.TrimSpace(*in.ReplyTo)
		if v != "" {
			if v, err = addr.Email(v); err != nil {
				return nil, err
			}
		}
		sets["reply_to"] = v
	}
	if in.Host != nil {
		v := strings.TrimSpace(*in.Host)
		if v != "" {
			if v, err = addr.Domain(v); err != nil {
				return nil, err
			}
			var other string
			if s.DB.R.QueryRowContext(ctx, "SELECT slug FROM lists WHERE host = ? AND id != ?", v, l.ID).Scan(&other) == nil {
				return nil, fmt.Errorf("host %s already serves list %s", v, other)
			}
		}
		sets["host"] = v
		// Mail already sent links to the old host (unsubscribe above all):
		// it stays served.
		if l.Host != "" && l.Host != v {
			sets["old_hosts"] = strings.Trim(strings.ReplaceAll(","+l.oldHosts+","+l.Host+",", ","+v+",", ","), ",")
		}
	}
	if in.Language != nil {
		v := strings.ToLower(strings.TrimSpace(*in.Language))
		if len(v) < 2 || len(v) > 8 {
			return nil, fmt.Errorf("language %q: use a code like en or de", v)
		}
		sets["language"] = v
	}
	if in.Tracking != nil {
		sets["tracking"] = *in.Tracking
	}
	for col, p := range map[string]*string{"imprint_url": in.ImprintURL, "privacy_url": in.PrivacyURL} {
		if p != nil {
			v := strings.TrimSpace(*p)
			if v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				return nil, fmt.Errorf("%s must be an http(s) URL", strings.ReplaceAll(col, "_", " "))
			}
			sets[col] = v
		}
	}
	if in.Address != nil {
		sets["address"] = strings.TrimSpace(*in.Address)
	}
	if in.Consent != nil {
		sets["consent"] = strings.TrimSpace(*in.Consent)
	}
	if in.Cadence != nil {
		v := strings.TrimSpace(*in.Cadence)
		if len(v) > 80 {
			return nil, errors.New("cadence is shown next to the signup form; keep it under 80 characters, like \"Every Tuesday morning\"")
		}
		sets["cadence"] = v
	}
	if in.Headline != nil {
		sets["headline"] = strings.TrimSpace(*in.Headline)
	}
	if in.Pitch != nil {
		sets["pitch"] = strings.TrimSpace(*in.Pitch)
	}
	if in.MinGap != nil {
		if *in.MinGap < 0 || *in.MinGap > 60*24*time.Hour {
			return nil, errors.New("min gap must be between 0 and 60 days")
		}
		sets["min_gap"] = int64(in.MinGap.Seconds())
	}
	if len(sets) > 0 {
		var cols []string
		var args []any
		for c, v := range sets {
			cols = append(cols, c+" = ?")
			args = append(args, v)
		}
		args = append(args, l.ID)
		if _, err := s.DB.W.ExecContext(ctx, "UPDATE lists SET "+strings.Join(cols, ", ")+" WHERE id = ?", args...); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, slug)
}

// Get loads a list by slug.
func (s *Service) Get(ctx context.Context, slug string) (*List, error) {
	l, err := scanList(s.DB.R.QueryRowContext(ctx, selectList+" WHERE l.slug = ?", strings.ToLower(strings.TrimSpace(slug))))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("list %s: %w; see lfr list ls", slug, store.ErrNotFound)
	}
	return l, err
}

// ByID loads a list without member counts.
func (s *Service) ByID(ctx context.Context, id int64) (*List, error) {
	l, err := scanList(s.DB.R.QueryRowContext(ctx, selectListLight+" WHERE l.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("list %d: %w", id, store.ErrNotFound)
	}
	return l, err
}

// ByHost finds the list whose website is host, without member counts.
func (s *Service) ByHost(ctx context.Context, host string) (*List, error) {
	host = strings.ToLower(host)
	l, err := scanList(s.DB.R.QueryRowContext(ctx, selectListLight+" WHERE l.host = ?1 OR instr(',' || l.old_hosts || ',', ',' || ?1 || ',') > 0 ORDER BY l.host = ?1 DESC LIMIT 1", host))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return l, err
}

// All returns every list.
func (s *Service) All(ctx context.Context) ([]*List, error) {
	rows, err := s.DB.R.QueryContext(ctx, selectList+" ORDER BY l.slug")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*List
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Hosts returns the web hosts of lists on a domain (or all when domainID is 0).
func (s *Service) Hosts(ctx context.Context, domainID int64) ([]string, error) {
	rows, err := s.DB.R.QueryContext(ctx, "SELECT host, old_hosts FROM lists WHERE host != '' AND (? = 0 OR domain_id = ?) ORDER BY host", domainID, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h, old string
		rows.Scan(&h, &old)
		out = append(out, h)
		for _, o := range strings.Split(old, ",") {
			if o != "" {
				out = append(out, o)
			}
		}
	}
	return out, rows.Err()
}

// Remove deletes a list with its memberships and issues. Contacts stay.
func (s *Service) Remove(ctx context.Context, slug string) error {
	l, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	_, err = s.DB.W.ExecContext(ctx, "DELETE FROM lists WHERE id = ?", l.ID)
	return err
}

// Pause stops sending for a list; an empty reason resumes it.
func (s *Service) Pause(ctx context.Context, slug, reason string) (*List, error) {
	l, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	// A resume starts list health afresh: the bounces and complaints that
	// caused the pause would otherwise pause it again at the next one.
	resumed := int64(0)
	if reason == "" && l.Paused != "" {
		resumed = s.Now().Unix()
	}
	if _, err := s.DB.W.ExecContext(ctx, "UPDATE lists SET paused = ?, resumed = max(resumed, ?) WHERE id = ?", reason, resumed, l.ID); err != nil {
		return nil, err
	}
	if reason != "" {
		s.Events.Record(ctx, events.Event{Kind: events.Paused, ListID: l.ID, Detail: reason})
	}
	return s.Get(ctx, slug)
}

// Health limits: past these rates a list pauses itself before mailbox
// providers start treating every domain on the server as a spammer.
const (
	healthWindow     = 30 * 24 * time.Hour
	minSentForHealth = 200
	maxComplaintRate = 0.003 // Gmail's published ceiling
	maxBounceRate    = 0.05
)

// CheckHealth pauses a list whose complaint or bounce rate over the last 30
// days is too high. It returns the reason when it paused the list.
func (s *Service) CheckHealth(ctx context.Context, listID int64) (string, error) {
	l, err := s.ByID(ctx, listID)
	if err != nil || l.Paused != "" {
		return "", err
	}
	since := s.Now().Add(-healthWindow).Unix()
	var resumed int64
	s.DB.R.QueryRowContext(ctx, "SELECT resumed FROM lists WHERE id = ?", listID).Scan(&resumed)
	since = max(since, resumed)
	var sent, bounced, complaints int
	s.DB.R.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM outbound o JOIN issues w ON w.id = o.issue_id WHERE w.list_id = ?1 AND o.contact_id IS NOT NULL AND o.status IN ('sent', 'bounced') AND o.created >= ?2),
		(SELECT count(*) FROM outbound o JOIN issues w ON w.id = o.issue_id WHERE w.list_id = ?1 AND o.contact_id IS NOT NULL AND o.status = 'bounced' AND o.created >= ?2),
		(SELECT count(*) FROM events WHERE list_id = ?1 AND kind = 'complained' AND at >= ?2)`, listID, since).Scan(&sent, &bounced, &complaints)
	if sent < minSentForHealth {
		return "", nil
	}
	var reason string
	switch {
	// One complaint is noise at any size; two above the rate are a pattern.
	case complaints >= 2 && float64(complaints)/float64(sent) > maxComplaintRate:
		reason = fmt.Sprintf("complaint rate %.2f%% over 30 days (%d of %d) exceeds 0.3%%; review content and signup sources, then lfr list set %s --resume", 100*float64(complaints)/float64(sent), complaints, sent, l.Slug)
	case float64(bounced)/float64(sent) > maxBounceRate:
		reason = fmt.Sprintf("bounce rate %.1f%% over 30 days (%d of %d) exceeds 5%%: the list holds stale or invented addresses. lfr issue cancel <issue> drops what has not gone out; ask the old readers to confirm again (lfr reader add %s --csv <file> --reconfirm); then lfr list set %s --resume", 100*float64(bounced)/float64(sent), bounced, sent, l.Slug, l.Slug)
	}
	if reason == "" {
		return "", nil
	}
	_, err = s.Pause(ctx, l.Slug, reason)
	return reason, err
}
