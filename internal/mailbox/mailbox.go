// Package mailbox is the plain mailbox side of lfr: addresses that receive
// and send like any mail account, with aliases, catch-alls, plus-addressing,
// forwarding, folders, flags and full-text search.
package mailbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/blob"
	"github.com/Anonyfox/lfr/internal/store"
)

// Folders a message can be in.
var Folders = []string{"inbox", "junk", "archive", "sent", "trash"}

// ValidFolder reports whether f is a folder name, accepting "spam" for junk.
func ValidFolder(f string) (string, bool) {
	f = strings.ToLower(strings.TrimSpace(f))
	if f == "spam" {
		f = "junk"
	}
	for _, x := range Folders {
		if x == f {
			return f, true
		}
	}
	return f, false
}

// Mailbox is one address that receives mail.
type Mailbox struct {
	ID       int64     `json:"id"`
	DomainID int64     `json:"-"`
	Address  string    `json:"address"`
	Name     string    `json:"name,omitempty"`
	Forward  string    `json:"forward,omitempty"`
	Aliases  []string  `json:"aliases,omitempty"`
	Catchall bool      `json:"catchall,omitempty"`
	Unread   int       `json:"unread"`
	Total    int       `json:"total"`
	Created  time.Time `json:"created"`
}

// Service is the mailbox store.
type Service struct {
	DB    *store.DB
	Blobs *blob.Store
	Now   func() time.Time
}

// Reserved local parts are handled by lfr itself and cannot be mailboxes.
// required are the addresses every domain must answer (RFC 5321 §4.5.1,
// RFC 2142): postmaster as a mailbox, abuse as an alias of one.
func required(local string) string {
	switch local {
	case "postmaster":
		return "postmaster@ must reach someone on every domain (RFC 5321 §4.5.1); forward it instead: lfr mailbox set postmaster@<domain> --forward you@example.org"
	case "abuse":
		return "abuse@ must reach someone on every domain (RFC 2142); point it at another mailbox instead: lfr mailbox alias abuse@<domain> you@<domain>"
	}
	return ""
}

func reserved(local string) bool {
	base := addr.Base(local)
	switch base {
	case "dmarc-reports", "tls-reports", "bounce", "unsub":
		return true
	}
	return false
}

func (s *Service) domainID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, domain string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM domains WHERE name = ?", domain).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("domain %s: %w; add it with lfr domain add %s", domain, store.ErrNotFound, domain)
	}
	return id, err
}

func splitLocal(email string) (local, domain string, err error) {
	a, err := addr.Parse(email)
	if err != nil {
		return "", "", err
	}
	local, err = addr.Local(a.Local)
	if err != nil {
		return "", "", err
	}
	return local, a.Domain, nil
}

// ErrExists reports a duplicate address.
var ErrExists = errors.New("already exists")

func (s *Service) taken(ctx context.Context, tx *sql.Tx, domainID int64, local string) error {
	var n int
	tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM mailboxes WHERE domain_id = ?1 AND local = ?2) +
		(SELECT count(*) FROM aliases WHERE domain_id = ?1 AND local = ?2)`, domainID, local).Scan(&n)
	if n > 0 {
		return ErrExists
	}
	return nil
}

// Add creates a mailbox.
func (s *Service) Add(ctx context.Context, email, name string) (*Mailbox, error) {
	local, domain, err := splitLocal(email)
	if err != nil {
		return nil, err
	}
	if reserved(local) {
		return nil, fmt.Errorf("%s is reserved for lfr itself", local)
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		did, err := s.domainID(ctx, tx, domain)
		if err != nil {
			return err
		}
		if err := s.taken(ctx, tx, did, local); err != nil {
			return fmt.Errorf("%s@%s %w", local, domain, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO mailboxes(domain_id, local, name, created) VALUES(?, ?, ?, ?)",
			did, local, strings.TrimSpace(name), s.Now().Unix())
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, local+"@"+domain)
}

const selectMailbox = `SELECT b.id, b.domain_id, b.local || '@' || d.name, b.name, b.forward, b.created, d.catchall = b.id,
	(SELECT count(*) FROM messages WHERE mailbox_id = b.id AND folder = 'inbox' AND seen = 0),
	(SELECT count(*) FROM messages WHERE mailbox_id = b.id AND folder != 'trash'),
	coalesce((SELECT group_concat(a.local || '@' || d.name, ',') FROM aliases a WHERE a.mailbox_id = b.id), '')
	FROM mailboxes b JOIN domains d ON d.id = b.domain_id`

func scanMailbox(row interface{ Scan(...any) error }) (*Mailbox, error) {
	var m Mailbox
	var created int64
	var catchall sql.NullBool
	var aliases string
	if err := row.Scan(&m.ID, &m.DomainID, &m.Address, &m.Name, &m.Forward, &created, &catchall, &m.Unread, &m.Total, &aliases); err != nil {
		return nil, err
	}
	m.Created = time.Unix(created, 0).UTC()
	m.Catchall = catchall.Bool
	if aliases != "" {
		m.Aliases = strings.Split(aliases, ",")
	}
	return &m, nil
}

// Get loads a mailbox by its address.
func (s *Service) Get(ctx context.Context, email string) (*Mailbox, error) {
	local, domain, err := splitLocal(email)
	if err != nil {
		return nil, err
	}
	m, err := scanMailbox(s.DB.R.QueryRowContext(ctx, selectMailbox+" WHERE d.name = ? AND b.local = ?", domain, local))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("mailbox %s@%s: %w", local, domain, store.ErrNotFound)
	}
	return m, err
}

// ByID loads a mailbox.
func (s *Service) ByID(ctx context.Context, id int64) (*Mailbox, error) {
	m, err := scanMailbox(s.DB.R.QueryRowContext(ctx, selectMailbox+" WHERE b.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("mailbox %d: %w", id, store.ErrNotFound)
	}
	return m, err
}

// List returns mailboxes, of one domain when domain is set.
func (s *Service) List(ctx context.Context, domain string) ([]*Mailbox, error) {
	q, args := selectMailbox, []any{}
	if domain != "" {
		d, err := addr.Domain(domain)
		if err != nil {
			return nil, err
		}
		q += " WHERE d.name = ?"
		args = append(args, d)
	}
	rows, err := s.DB.R.QueryContext(ctx, q+" ORDER BY d.name, b.local", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Mailbox
	for rows.Next() {
		m, err := scanMailbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Remove deletes a mailbox with all its messages.
func (s *Service) Remove(ctx context.Context, email string) error {
	m, err := s.Get(ctx, email)
	if err != nil {
		return err
	}
	if why := required(addr.Base(strings.SplitN(m.Address, "@", 2)[0])); why != "" {
		return errors.New(why)
	}
	var blobs []string
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT DISTINCT blob FROM messages WHERE mailbox_id = ?", m.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var b string
			rows.Scan(&b)
			blobs = append(blobs, b)
		}
		rows.Close()
		if _, err := tx.ExecContext(ctx, "DELETE FROM messages_fts WHERE rowid IN (SELECT id FROM messages WHERE mailbox_id = ?)", m.ID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM mailboxes WHERE id = ?", m.ID)
		return err
	})
	if err != nil {
		return err
	}
	s.dropUnreferenced(ctx, blobs)
	return nil
}

// Rename moves a mailbox to a new address, keeping its messages.
func (s *Service) Rename(ctx context.Context, from, to string) (*Mailbox, error) {
	m, err := s.Get(ctx, from)
	if err != nil {
		return nil, err
	}
	if why := required(addr.Base(strings.SplitN(m.Address, "@", 2)[0])); why != "" {
		return nil, errors.New(why)
	}
	local, domain, err := splitLocal(to)
	if err != nil {
		return nil, err
	}
	if reserved(local) {
		return nil, fmt.Errorf("%s is reserved for lfr itself", local)
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		did, err := s.domainID(ctx, tx, domain)
		if err != nil {
			return err
		}
		if err := s.taken(ctx, tx, did, local); err != nil {
			return fmt.Errorf("%s@%s %w", local, domain, err)
		}
		if did != m.DomainID {
			// Aliases and catch-all belong to the old domain.
			if _, err := tx.ExecContext(ctx, "DELETE FROM aliases WHERE mailbox_id = ?", m.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE domains SET catchall = NULL WHERE catchall = ?", m.ID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE mailboxes SET domain_id = ?, local = ? WHERE id = ?", did, local, m.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.ByID(ctx, m.ID)
}

// Update changes the display name and forwarding address; nil leaves a value as is.
func (s *Service) Update(ctx context.Context, email string, name, forward *string) (*Mailbox, error) {
	m, err := s.Get(ctx, email)
	if err != nil {
		return nil, err
	}
	if name != nil {
		if _, err := s.DB.W.ExecContext(ctx, "UPDATE mailboxes SET name = ? WHERE id = ?", strings.TrimSpace(*name), m.ID); err != nil {
			return nil, err
		}
	}
	if forward != nil {
		f := strings.TrimSpace(*forward)
		if f != "" {
			if f, err = addr.Email(f); err != nil {
				return nil, err
			}
			if strings.EqualFold(f, m.Address) {
				return nil, errors.New("a mailbox cannot forward to itself")
			}
		}
		if _, err := s.DB.W.ExecContext(ctx, "UPDATE mailboxes SET forward = ? WHERE id = ?", f, m.ID); err != nil {
			return nil, err
		}
	}
	return s.ByID(ctx, m.ID)
}

// AddAlias makes alias deliver into the mailbox target; both share a domain.
func (s *Service) AddAlias(ctx context.Context, alias, target string) error {
	local, domain, err := splitLocal(alias)
	if err != nil {
		return err
	}
	if reserved(local) {
		return fmt.Errorf("%s is reserved for lfr itself", local)
	}
	m, err := s.Get(ctx, target)
	if err != nil {
		return err
	}
	return s.DB.Tx(ctx, func(tx *sql.Tx) error {
		did, err := s.domainID(ctx, tx, domain)
		if err != nil {
			return err
		}
		if did != m.DomainID {
			return fmt.Errorf("alias %s and mailbox %s must share a domain; use forwarding across domains", alias, m.Address)
		}
		if local == "abuse" {
			// abuse@ always exists; aliasing it again points it elsewhere.
			if res, err := tx.ExecContext(ctx, "UPDATE aliases SET mailbox_id = ? WHERE domain_id = ? AND local = 'abuse'", m.ID, did); err != nil {
				return err
			} else if n, _ := res.RowsAffected(); n > 0 {
				return nil
			}
		}
		if err := s.taken(ctx, tx, did, local); err != nil {
			return fmt.Errorf("%s@%s %w", local, domain, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO aliases(domain_id, local, mailbox_id) VALUES(?, ?, ?)", did, local, m.ID)
		return err
	})
}

// RemoveAlias deletes an alias.
func (s *Service) RemoveAlias(ctx context.Context, alias string) error {
	local, domain, err := splitLocal(alias)
	if err != nil {
		return err
	}
	if why := required(local); why != "" {
		return errors.New(why)
	}
	res, err := s.DB.W.ExecContext(ctx, "DELETE FROM aliases WHERE local = ? AND domain_id = (SELECT id FROM domains WHERE name = ?)", local, domain)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("alias %s@%s: %w", local, domain, store.ErrNotFound)
	}
	return nil
}

// SetCatchall routes unknown addresses of domain to target; empty target turns it off.
func (s *Service) SetCatchall(ctx context.Context, domain, target string) error {
	d, err := addr.Domain(domain)
	if err != nil {
		return err
	}
	did, err := s.domainID(ctx, s.DB.R, d)
	if err != nil {
		return err
	}
	if target == "" || target == "off" {
		_, err = s.DB.W.ExecContext(ctx, "UPDATE domains SET catchall = NULL WHERE id = ?", did)
		return err
	}
	m, err := s.Get(ctx, target)
	if err != nil {
		return err
	}
	if m.DomainID != did {
		return fmt.Errorf("catch-all mailbox %s must belong to %s", m.Address, d)
	}
	_, err = s.DB.W.ExecContext(ctx, "UPDATE domains SET catchall = ? WHERE id = ?", m.ID, did)
	return err
}

// Resolve finds the mailbox mail to rcpt belongs in: the exact address, an
// alias, the address without its +tag, then the domain's catch-all.
func (s *Service) Resolve(ctx context.Context, rcpt string) (int64, bool, error) {
	a, err := addr.Parse(rcpt)
	if err != nil {
		return 0, false, nil
	}
	var id int64
	for _, local := range uniq(a.Local, addr.Base(a.Local)) {
		err = s.DB.R.QueryRowContext(ctx, `SELECT coalesce(
			(SELECT b.id FROM mailboxes b JOIN domains d ON d.id = b.domain_id WHERE d.name = ?1 AND b.local = ?2),
			(SELECT a.mailbox_id FROM aliases a JOIN domains d ON d.id = a.domain_id WHERE d.name = ?1 AND a.local = ?2), 0)`,
			a.Domain, local).Scan(&id)
		if err != nil {
			return 0, false, err
		}
		if id != 0 {
			return id, true, nil
		}
	}
	var catchall sql.NullInt64
	err = s.DB.R.QueryRowContext(ctx, "SELECT catchall FROM domains WHERE name = ?", a.Domain).Scan(&catchall)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return catchall.Int64, catchall.Valid, nil
}

func uniq(a, b string) []string {
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

// BlobRefs counts what still points at a stored blob: messages, sends not
// done yet, issue images. A blob is removed only when this says 0 without
// an error; a failed query must never read as "unused".
func BlobRefs(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, hash string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM messages WHERE blob = ?1)
		+ (SELECT count(*) FROM outbound WHERE blob = ?1 AND status IN ('queued', 'sending'))
		+ (SELECT count(*) FROM media WHERE hash = ?1)`, hash).Scan(&n)
	return n, err
}

// dropUnreferenced removes blobs nothing points at anymore.
func (s *Service) dropUnreferenced(ctx context.Context, hashes []string) {
	for _, h := range hashes {
		if n, err := BlobRefs(ctx, s.DB.W, h); err == nil && n == 0 {
			s.Blobs.Remove(h)
		}
	}
}
