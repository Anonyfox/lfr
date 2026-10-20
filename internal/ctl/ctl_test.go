package ctl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallAndStream(t *testing.T) {
	dir, _ := os.MkdirTemp("", "lfr")
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "s")
	s := &Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Classify: func(error) string { return CodeConflict }}
	s.Handle("echo", func(_ context.Context, a json.RawMessage) (any, error) {
		var in struct{ X int }
		json.Unmarshal(a, &in)
		return map[string]int{"y": in.X * 2}, nil
	})
	s.Handle("fail", func(context.Context, json.RawMessage) (any, error) {
		return nil, &Error{Code: CodeNotFound, Msg: "nope"}
	})
	s.Handle("plain", func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("boom") })
	s.Handle("panic", func(context.Context, json.RawMessage) (any, error) { panic("oops") })
	s.Stream("count", func(ctx context.Context, _ json.RawMessage, send func(any) error) error {
		for i := 0; i < 3; i++ {
			if err := send(i); err != nil {
				return err
			}
		}
		return nil
	})
	l, err := Listen(path, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Serve(ctx, l)
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", fi.Mode())
	}
	if _, err := Listen(path, 0o600); err == nil {
		t.Fatal("second server on the same socket")
	}
	c := Client{Path: path}
	var out struct{ Y int }
	if err := c.Call(ctx, "echo", map[string]int{"x": 21}, &out); err != nil || out.Y != 42 {
		t.Fatal(out, err)
	}
	var e *Error
	if err := c.Call(ctx, "fail", nil, nil); !errors.As(err, &e) || e.Code != CodeNotFound {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "plain", nil, nil); !errors.As(err, &e) || e.Code != CodeConflict {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "panic", nil, nil); !errors.As(err, &e) || e.Code != CodeInternal {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "missing", nil, nil); err == nil {
		t.Fatal("unknown op")
	}
	var got []int
	err = c.Stream(ctx, "count", nil, func(m json.RawMessage) error {
		var i int
		json.Unmarshal(m, &i)
		got = append(got, i)
		return nil
	})
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}
	if err := (Client{Path: filepath.Join(dir, "absent")}).Call(ctx, "echo", nil, nil); !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
	_ = time.Second
}
