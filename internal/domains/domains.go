// Package domains manages the mail domains lfr serves: their DKIM keys, the
// postmaster mailbox every domain must have, and the exact DNS records an
// operator has to publish for mail and websites to work.
package domains

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/store"
)

// Domain is one served domain.
type Domain struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Selector string    `json:"selector"`
	Catchall string    `json:"catchall,omitempty"` // local part of the catch-all mailbox
	Created  time.Time `json:"created"`
	Next     *Domain   `json:"next,omitempty"` // keys of a rotation in progress: Selector and keys only

	rsaKey *rsa.PrivateKey
	edKey  ed25519.PrivateKey
}

// RSASelector and EdSelector are the DKIM selectors of the two keys.
func (d *Domain) RSASelector() string { return d.Selector + "r" }
func (d *Domain) EdSelector() string  { return d.Selector + "e" }

// Signers returns the DKIM keys, Ed25519 first.
func (d *Domain) Signers() []crypto.Signer { return []crypto.Signer{d.edKey, d.rsaKey} }

// RSAPublic is the p= value of the RSA DKIM record.
func (d *Domain) RSAPublic() string {
	der, _ := x509.MarshalPKIXPublicKey(&d.rsaKey.PublicKey)
	return base64.StdEncoding.EncodeToString(der)
}

// EdPublic is the p= value of the Ed25519 DKIM record (RFC 8463: the raw key).
func (d *Domain) EdPublic() string {
	return base64.StdEncoding.EncodeToString(d.edKey.Public().(ed25519.PublicKey))
}

// Service is the domain store.
type Service struct {
	DB  *store.DB
	Now func() time.Time
}

// ErrExists reports a duplicate.
var ErrExists = errors.New("already exists")

// Add creates a domain with fresh DKIM keys and a postmaster mailbox
// (with abuse@ as alias), as RFC 5321 and 2142 require.
func (s *Service) Add(ctx context.Context, name string) (*Domain, error) {
	name, err := addr.Domain(name)
	if err != nil {
		return nil, err
	}
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	_, ek, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	rder, _ := x509.MarshalPKCS8PrivateKey(rk)
	eder, _ := x509.MarshalPKCS8PrivateKey(ek)
	now := s.Now().Unix()
	selector := "lfr" + time.Unix(now, 0).UTC().Format("0601")
	var id int64
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		var n int
		tx.QueryRowContext(ctx, "SELECT count(*) FROM domains WHERE name = ?", name).Scan(&n)
		if n > 0 {
			return fmt.Errorf("domain %s %w", name, ErrExists)
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO domains(name, selector, dkim_rsa, dkim_ed, created) VALUES(?, ?, ?, ?, ?)",
			name, selector, rder, eder, now)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		res, err = tx.ExecContext(ctx, "INSERT INTO mailboxes(domain_id, local, created) VALUES(?, 'postmaster', ?)", id, now)
		if err != nil {
			return err
		}
		mb, _ := res.LastInsertId()
		_, err = tx.ExecContext(ctx, "INSERT INTO aliases(domain_id, local, mailbox_id) VALUES(?, 'abuse', ?)", id, mb)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

const selectDomain = `SELECT d.id, d.name, d.selector, d.dkim_rsa, d.dkim_ed, coalesce(m.local, ''), d.created, d.next_selector, d.next_rsa, d.next_ed
	FROM domains d LEFT JOIN mailboxes m ON m.id = d.catchall`

func scan(row interface{ Scan(...any) error }) (*Domain, error) {
	var d Domain
	var rder, eder, nrsa, ned []byte
	var created int64
	var next string
	if err := row.Scan(&d.ID, &d.Name, &d.Selector, &rder, &eder, &d.Catchall, &created, &next, &nrsa, &ned); err != nil {
		return nil, err
	}
	d.Created = time.Unix(created, 0).UTC()
	if err := d.parseKeys(rder, eder); err != nil {
		return nil, err
	}
	if next != "" {
		d.Next = &Domain{ID: d.ID, Name: d.Name, Selector: next}
		if err := d.Next.parseKeys(nrsa, ned); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

func (d *Domain) parseKeys(rder, eder []byte) error {
	rk, err := x509.ParsePKCS8PrivateKey(rder)
	if err != nil {
		return fmt.Errorf("domain %s: RSA key: %w", d.Name, err)
	}
	ek, err := x509.ParsePKCS8PrivateKey(eder)
	if err != nil {
		return fmt.Errorf("domain %s: Ed25519 key: %w", d.Name, err)
	}
	var ok bool
	if d.rsaKey, ok = rk.(*rsa.PrivateKey); !ok {
		return fmt.Errorf("domain %s: RSA key has the wrong type", d.Name)
	}
	if d.edKey, ok = ek.(ed25519.PrivateKey); !ok {
		return fmt.Errorf("domain %s: Ed25519 key has the wrong type", d.Name)
	}
	return nil
}

// Get loads a domain by name.
func (s *Service) Get(ctx context.Context, name string) (*Domain, error) {
	n, err := addr.Domain(name)
	if err != nil {
		return nil, err
	}
	d, err := scan(s.DB.R.QueryRowContext(ctx, selectDomain+" WHERE d.name = ?", n))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("domain %s: %w; add it with lfr domain add %s", n, store.ErrNotFound, n)
	}
	return d, err
}

// ByID loads a domain by ID.
func (s *Service) ByID(ctx context.Context, id int64) (*Domain, error) {
	d, err := scan(s.DB.R.QueryRowContext(ctx, selectDomain+" WHERE d.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("domain %d: %w", id, store.ErrNotFound)
	}
	return d, err
}

// List returns every domain by name.
func (s *Service) List(ctx context.Context) ([]*Domain, error) {
	rows, err := s.DB.R.QueryContext(ctx, selectDomain+" ORDER BY d.name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Domain
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Remove deletes a domain with its mailboxes, messages and lists. Blobs of the
// deleted messages are left for the next blob sweep.
func (s *Service) Remove(ctx context.Context, name string) error {
	d, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	return s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM messages_fts WHERE rowid IN (SELECT m.id FROM messages m JOIN mailboxes b ON b.id = m.mailbox_id WHERE b.domain_id = ?)", d.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM domains WHERE id = ?", d.ID)
		return err
	})
}

// PrepareRotation creates the next pair of DKIM keys under a new selector.
// They do not sign anything until Rotate switches to them, which the caller
// does once their records are visible in DNS (RFC 6376 §3.6.1: publish
// first, sign after). A rotation in progress is returned unchanged.
func (s *Service) PrepareRotation(ctx context.Context, name string) (*Domain, error) {
	d, err := s.Get(ctx, name)
	if err != nil || d.Next != nil {
		return d, err
	}
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	_, ek, _ := ed25519.GenerateKey(rand.Reader)
	rder, _ := x509.MarshalPKCS8PrivateKey(rk)
	eder, _ := x509.MarshalPKCS8PrivateKey(ek)
	selector := "lfr" + s.Now().UTC().Format("060102150405")
	if selector == d.Selector {
		return nil, errors.New("rotated a moment ago; wait a second")
	}
	if _, err := s.DB.W.ExecContext(ctx, "UPDATE domains SET next_selector = ?, next_rsa = ?, next_ed = ? WHERE id = ?", selector, rder, eder, d.ID); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

// Rotate makes the prepared keys the signing keys. The old records stay in
// DNS a few days, until mail signed with them has been delivered and read.
func (s *Service) Rotate(ctx context.Context, name string) (*Domain, error) {
	d, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if d.Next == nil {
		return nil, errors.New("no rotation prepared")
	}
	if _, err := s.DB.W.ExecContext(ctx, `UPDATE domains SET selector = next_selector, dkim_rsa = next_rsa, dkim_ed = next_ed,
		next_selector = '', next_rsa = NULL, next_ed = NULL WHERE id = ?`, d.ID); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}
