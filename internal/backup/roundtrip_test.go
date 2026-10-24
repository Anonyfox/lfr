package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/ctl"
)

// serve runs a fake server whose backup op sends frames.
func serve(t *testing.T, frames func(send func(any) error) error) ctl.Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	s.Stream(api.OpBackup, func(_ context.Context, _ json.RawMessage, send func(any) error) error { return frames(send) })
	l, err := net.Listen("unix", socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go s.Serve(ctx, l)
	return ctl.Client{Path: l.Addr().String()}
}

// socketPath is short: macOS caps unix socket paths at 104 bytes, and
// t.TempDir under its long TMPDIR exceeds that.
func socketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lfrb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func sendFile(send func(any) error, path string, data []byte, hash string) error {
	if err := send(api.BackupFrame{Path: path, Data: data}); err != nil {
		return err
	}
	return send(api.BackupFrame{Path: path, Offset: int64(len(data)), End: true, Size: int64(len(data)), SHA256: hash})
}

func TestReceiveWritesAVerifiedBackup(t *testing.T) {
	db, blob, page := []byte("sqlite"), []byte("Subject: hi\r\n\r\nhello\r\n"), []byte("<h1>hi</h1>")
	h := sum(blob)
	blobPath := "msg/" + h[:2] + "/" + h + ".eml"
	complete := fmt.Sprintf("lfr backup complete\nat: 2026-10-24T00:00:00Z\nmessages: 1\nbytes: %d\n", len(blob))
	client := serve(t, func(send func(any) error) error {
		for _, f := range []struct {
			path string
			data []byte
		}{{"lfr.db", db}, {blobPath, blob}, {"sites/news/index.html", page}} {
			if err := sendFile(send, f.path, f.data, sum(f.data)); err != nil {
				return err
			}
		}
		return send(api.BackupFrame{Summary: &api.BackupResult{Blobs: 1, Bytes: int64(len(blob))}, Complete: complete})
	})
	dir := filepath.Join(t.TempDir(), "backup")
	res, err := Receive(context.Background(), client, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Dir != dir || res.Blobs != 1 || res.Bytes != int64(len(blob)) {
		t.Fatalf("result %+v", res)
	}
	for path, want := range map[string]string{"lfr.db": string(db), blobPath: string(blob), "sites/news/index.html": string(page), "BACKUP-COMPLETE": complete} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q %v", path, got, err)
		}
	}
	if _, err := Receive(context.Background(), client, dir); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("second backup into the same directory: %v", err)
	}
}

func TestReceiveRejectsTamperedData(t *testing.T) {
	blob := []byte("hello")
	h := sum(blob)
	cases := map[string]func(send func(any) error) error{
		"wrong sha256": func(send func(any) error) error {
			return sendFile(send, "lfr.db", []byte("db"), sum([]byte("other")))
		},
		"blob name is not its hash": func(send func(any) error) error {
			sendFile(send, "lfr.db", []byte("db"), sum([]byte("db")))
			other := strings.Repeat("ab", 32)
			return sendFile(send, "msg/ab/"+other+".eml", blob, h)
		},
		"summary without database": func(send func(any) error) error {
			return send(api.BackupFrame{Summary: &api.BackupResult{}, Complete: "lfr backup complete\nat: x\nmessages: 0\nbytes: 0\n"})
		},
		"cut off": func(send func(any) error) error {
			return send(api.BackupFrame{Path: "lfr.db", Data: []byte("db")})
		},
	}
	for name, frames := range cases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "backup")
			if _, err := Receive(context.Background(), serve(t, frames), dir); err == nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("accepted: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "BACKUP-COMPLETE")); !os.IsNotExist(err) {
				t.Fatal("completion marker exists")
			}
		})
	}
}
