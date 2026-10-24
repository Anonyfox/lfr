package backup

import (
	"context"
	"encoding/json"
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

func TestRejectEscapingFrames(t *testing.T) {
	for _, p := range []string{"../escaped", "/escaped", "sites/../../escaped", "sites/../escaped", "sites/\\..\\escaped", "sites/C:/escaped"} {
		t.Run(p, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			s.Stream(api.OpBackup, func(_ context.Context, _ json.RawMessage, send func(any) error) error {
				return send(api.BackupFrame{Path: p, Data: []byte("bad")})
			})
			dir := t.TempDir()
			target := filepath.Join(dir, "backup")

			listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "s"))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go s.Serve(ctx, listener)
			client := ctl.Client{Path: listener.Addr().String()}
			if _, err := Receive(ctx, client, target); err == nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("escaping frame: %v", err)
			}
			if _, err := os.Stat(filepath.Join(target, "BACKUP-COMPLETE")); !os.IsNotExist(err) {
				t.Fatal("completion marker exists")
			}
		})
	}
}

func FuzzBackupPath(f *testing.F) {
	for _, s := range []string{"lfr.db", "sites/news/custom.html", "../escape", "/abs", "sites/../../bad", "msg/ab/" + strings.Repeat("ab", 32) + ".eml"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if ValidPath(s) {
			root := filepath.Join(string(filepath.Separator), "backup")
			target := filepath.Join(root, filepath.FromSlash(s))
			rel, err := filepath.Rel(root, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				t.Fatalf("escaped %q", s)
			}
		}
	})
}
