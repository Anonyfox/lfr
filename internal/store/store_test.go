package store

import (
	"context"
	"path/filepath"
	"testing"
)

// OpenTest opens a fresh database in a temporary directory.
func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "lfr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenMigratesAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lfr.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "a.b", "1"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.R.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v < 1 {
		t.Fatalf("user_version %d, %v", v, err)
	}
	if got, _ := db.Setting(ctx, "a.b", ""); got != "1" {
		t.Fatalf("setting = %q", got)
	}
}

func TestReadersCannotWrite(t *testing.T) {
	db := openTest(t)
	if _, err := db.R.Exec("INSERT INTO settings(key, value) VALUES('x', 'y')"); err == nil {
		t.Fatal("reader pool accepted a write")
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	if v, _ := db.Setting(ctx, "missing", "def"); v != "def" {
		t.Fatalf("default = %q", v)
	}
	for _, kv := range [][2]string{{"relay.url", "u"}, {"relay.user", "x"}, {"other", "o"}} {
		if err := db.SetSetting(ctx, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	db.SetSetting(ctx, "relay.url", "u2")
	m, err := db.Settings(ctx, "relay.")
	if err != nil || len(m) != 2 || m["relay.url"] != "u2" {
		t.Fatalf("settings = %v, %v", m, err)
	}
	db.DeleteSetting(ctx, "relay.url")
	if v, _ := db.Setting(ctx, "relay.url", "gone"); v != "gone" {
		t.Fatalf("after delete = %q", v)
	}
}

func TestFTSContentless(t *testing.T) {
	db := openTest(t)
	if _, err := db.W.Exec("INSERT INTO messages_fts(rowid, subject, people, body) VALUES(7, 'Grüße aus Köln', 'a@b.de', 'hallo welt')"); err != nil {
		t.Fatal(err)
	}
	var id int
	if err := db.R.QueryRow("SELECT rowid FROM messages_fts WHERE messages_fts MATCH 'koln'").Scan(&id); err != nil || id != 7 {
		t.Fatalf("match = %d, %v", id, err)
	}
	if _, err := db.W.Exec("DELETE FROM messages_fts WHERE rowid = 7"); err != nil {
		t.Fatal(err)
	}
}
