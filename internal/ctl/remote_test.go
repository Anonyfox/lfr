package ctl

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHTransportReportsRemoteFailures(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		notRunning         bool
	}{
		{"round trip", "cat", `hello`, false},
		{"server absent", "echo 'lfr: no server is running' >&2; exit 1", "", true},
		{"ssh failure", "echo 'Permission denied' >&2; exit 255", "Permission denied", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			script := "#!/bin/sh\n" + tc.script + "\n"
			if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			conn, err := SSH("user@server", "/data/owner's mail")(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if tc.name == "round trip" {
				if _, err := io.WriteString(conn, "hello"); err != nil {
					t.Fatal(err)
				}
				b := make([]byte, 5)
				if _, err := io.ReadFull(conn, b); err != nil || string(b) != "hello" {
					t.Fatalf("round trip: %q %v", b, err)
				}
				return
			}
			_, err = io.ReadAll(conn)
			if tc.notRunning {
				if !errors.Is(err, ErrNotRunning) {
					t.Fatalf("remote missing server: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q: %v", tc.want, err)
			}
		})
	}
}

func TestProxyCopiesBothDirectionsAndHonorsHalfClose(t *testing.T) {
	// macOS caps unix socket paths at 104 bytes; its t.TempDir paths run past that.
	dir, err := os.MkdirTemp("/tmp", "lfrc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "socket")
	listener, err := Listen(path, 0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// EOF on stdin must still allow the server response to reach stdout.
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b, _ := io.ReadAll(conn)
		io.WriteString(conn, strings.ToUpper(string(b)))
	}()
	defer listener.Close()
	var out strings.Builder
	if err := Proxy(ctx, path, strings.NewReader("hello"), &out); err != nil || out.String() != "HELLO" {
		t.Fatalf("proxy: %q %v", out.String(), err)
	}
	if err := Proxy(ctx, filepath.Join(dir, "absent"), strings.NewReader(""), io.Discard); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("missing socket: %v", err)
	}
}
