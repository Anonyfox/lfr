// Package store owns the SQLite database: opening it with the pragmas a
// long-running server needs, schema migrations, transactions and the
// key-value settings table. Every other package runs its own queries
// through a *DB; none opens the file itself.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the database, split into one writer connection and a small pool of
// readers. SQLite allows one writer at a time anyway; giving it a dedicated
// connection keeps writes from queueing behind busy timeouts, while WAL lets
// readers proceed concurrently.
type DB struct {
	W *sql.DB // the only connection that writes
	R *sql.DB // read-only connections
}

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("not found")

// Open opens or creates the database at path and migrates it to the
// current schema.
func Open(ctx context.Context, path string) (*DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	pragmas := []string{
		"busy_timeout(10000)",
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"foreign_keys(ON)",
		"temp_store(MEMORY)",
		"cache_size(-4000)", // 4 MB page cache per connection
	}
	dsn := func(extra string) string {
		q := url.Values{}
		for _, p := range pragmas {
			q.Add("_pragma", p)
		}
		if extra != "" {
			k, v, _ := strings.Cut(extra, "=")
			q.Set(k, v)
		}
		return "file:" + abs + "?" + q.Encode()
	}
	// The writer syncs every commit: a 250 to an SMTP client, a recorded
	// unsubscribe or a sent mark must survive a power loss (RFC 5321 §6.1).
	pragmas[2] = "synchronous(FULL)"
	w, err := sql.Open("sqlite", dsn("_txlock=immediate"))
	pragmas[2] = "synchronous(NORMAL)"
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)
	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db := &DB{W: w}
	if err := db.migrate(ctx); err != nil {
		w.Close()
		return nil, err
	}
	pragmas = append(pragmas, "query_only(ON)")
	r, err := sql.Open("sqlite", dsn(""))
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(4)
	r.SetMaxIdleConns(2)
	db.R = r
	return db, nil
}

// Close closes both pools.
func (db *DB) Close() error {
	return errors.Join(db.R.Close(), db.W.Close())
}

// Tx runs fn in a write transaction, committing when it returns nil.
func (db *DB) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.W.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (db *DB) migrate(ctx context.Context) error {
	var version int
	if err := db.W.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	type step struct {
		n    int
		name string
	}
	var steps []step
	for _, e := range entries {
		num, _, ok := strings.Cut(e.Name(), "_")
		n, err := strconv.Atoi(num)
		if !ok || err != nil {
			return fmt.Errorf("migration %s: name must start with a number", e.Name())
		}
		steps = append(steps, step{n, e.Name()})
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].n < steps[j].n })
	if len(steps) > 0 && version > steps[len(steps)-1].n {
		return fmt.Errorf("data was written by a newer lfr (schema %d); install that version", version)
	}
	for _, s := range steps {
		if s.n <= version {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + s.name)
		if err != nil {
			return err
		}
		err = db.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", s.name, err)
			}
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", s.n))
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Setting reads one settings value; missing keys return def.
func (db *DB) Setting(ctx context.Context, key, def string) (string, error) {
	var v string
	err := db.R.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	return v, err
}

// SetSetting writes one settings value.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.W.ExecContext(ctx,
		"INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

// Settings returns every setting whose key starts with prefix.
func (db *DB) Settings(ctx context.Context, prefix string) (map[string]string, error) {
	rows, err := db.R.QueryContext(ctx, "SELECT key, value FROM settings WHERE key >= ? AND key < ? ORDER BY key", prefix, prefix+"￿")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// DeleteSetting removes one settings value.
func (db *DB) DeleteSetting(ctx context.Context, key string) error {
	_, err := db.W.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", key)
	return err
}

// Backup writes a consistent copy of the database to path.
func (db *DB) Backup(ctx context.Context, path string) error {
	_, err := db.W.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}
