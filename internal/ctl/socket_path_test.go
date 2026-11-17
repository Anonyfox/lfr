package ctl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSocketPath(t *testing.T) {
	limit := socketPathLimit()
	if limit == 0 {
		t.Skip("Windows has no Unix path check")
	}
	for _, tc := range []struct {
		name      string
		path      string
		wantError bool
	}{
		{"under", strings.Repeat("x", limit-1), false},
		{"at", strings.Repeat("x", limit), false},
		{"over", strings.Repeat("x", limit+1), true},
		{"multibyte", strings.Repeat("é", limit/2+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSocketPath(tc.path)
			if (err != nil) != tc.wantError {
				t.Fatalf("checkSocketPath(%q) = %v", tc.path, err)
			}
			if tc.wantError {
				want := fmt.Sprintf("control socket path %q is %d bytes, longer than the %d this system allows: set LFR_SOCKET to a shorter path (such as /tmp/lfr-<name>.sock) or use a shorter --data/LFR_DATA", tc.path, len(tc.path), limit)
				if err.Error() != want {
					t.Fatalf("got %q, want %q", err, want)
				}
			}
		})
	}
}

func TestListenLongSocketPath(t *testing.T) {
	if socketPathLimit() == 0 {
		t.Skip("Windows has no Unix path check")
	}
	dir, err := os.MkdirTemp("/tmp", "lfrc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, strings.Repeat("x", socketPathLimit()))
	listener, err := Listen(path, 0600)
	if listener != nil {
		listener.Close()
		t.Fatal("unexpected listener")
	}
	want := checkSocketPath(path)
	if err == nil || err.Error() != want.Error() {
		t.Fatalf("Listen = %v, want %v", err, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Listen created files: %v, %v", entries, err)
	}
	// Validation must also precede stale-socket removal.
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path, 0600); err == nil {
		t.Fatal("expected path error")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "keep" {
		t.Fatalf("existing file changed: %q, %v", b, err)
	}
}

func TestClientDialLongSocketPath(t *testing.T) {
	if socketPathLimit() == 0 {
		t.Skip("Windows has no Unix path check")
	}
	path := strings.Repeat("x", socketPathLimit()+1)
	c := Client{Path: path}
	conn, err := c.dial(context.Background())
	if conn != nil {
		conn.Close()
		t.Fatal("unexpected connection")
	}
	if err == nil || err.Error() != checkSocketPath(path).Error() || errors.Is(err, ErrNotRunning) {
		t.Fatalf("dial = %v", err)
	}
	sentinel := errors.New("custom dial")
	called := false
	c.Dial = func(context.Context) (net.Conn, error) {
		called = true
		return nil, sentinel
	}
	if _, err := c.dial(context.Background()); !called || err != sentinel {
		t.Fatalf("custom dial: called=%v, error=%v", called, err)
	}
}
