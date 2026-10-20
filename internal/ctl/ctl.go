// Package ctl is the local control protocol between the lfr CLI and the
// running server: one JSON request per connection over a unix socket that
// only the server's user (and group) can open. There is no network API.
package ctl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

// Request is one call.
type Request struct {
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Response is the answer; streams send several Data lines, then a final one with Done.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
	Code  string          `json:"code,omitempty"` // not_found, invalid, conflict, internal
	Done  bool            `json:"done,omitempty"`
}

// Error carries a machine-readable code.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Codes.
const (
	CodeNotFound = "not_found"
	CodeInvalid  = "invalid"
	CodeConflict = "conflict"
	CodeInternal = "internal"
)

// Handler answers a call.
type Handler func(ctx context.Context, args json.RawMessage) (any, error)

// StreamHandler sends values until ctx ends or it returns.
type StreamHandler func(ctx context.Context, args json.RawMessage, send func(any) error) error

// Server dispatches calls.
type Server struct {
	Log      *slog.Logger
	Classify func(error) string // maps errors to codes

	mu      sync.RWMutex
	ops     map[string]Handler
	streams map[string]StreamHandler
}

// Handle registers an operation.
func (s *Server) Handle(op string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ops == nil {
		s.ops = map[string]Handler{}
	}
	s.ops[op] = h
}

// Stream registers a streaming operation.
func (s *Server) Stream(op string, h StreamHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streams == nil {
		s.streams = map[string]StreamHandler{}
	}
	s.streams[op] = h
}

// Ops lists every registered operation.
func (s *Server) Ops() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for k := range s.ops {
		out = append(out, k)
	}
	for k := range s.streams {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Listen opens the socket at path with the given permissions, replacing a stale one.
func Listen(path string, mode os.FileMode) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, fmt.Errorf("another lfr server is running on %s", path)
	}
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, mode); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Serve answers connections until ctx ends.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.serveConn(ctx, c)
	}
}

const maxRequest = 64 << 20 // imports and attachments travel inside requests

func (s *Server) serveConn(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(time.Minute))
	var req Request
	if err := json.NewDecoder(io.LimitReader(c, maxRequest)).Decode(&req); err != nil {
		writeResp(c, Response{Error: "unreadable request: " + err.Error(), Code: CodeInvalid})
		return
	}
	c.SetReadDeadline(time.Time{})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.RLock()
	h, ok := s.ops[req.Op]
	st, isStream := s.streams[req.Op]
	s.mu.RUnlock()
	start := time.Now()
	defer func() {
		if v := recover(); v != nil {
			s.Log.Error("ctl panic", "op", req.Op, "panic", v)
			writeResp(c, Response{Error: fmt.Sprintf("internal error: %v", v), Code: CodeInternal})
		}
	}()
	switch {
	case isStream:
		// The client ends a stream by closing its side.
		go func() {
			io.Copy(io.Discard, c)
			cancel()
		}()
		w := bufio.NewWriter(c)
		send := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(w).Encode(Response{OK: true, Data: b}); err != nil {
				return err
			}
			return w.Flush()
		}
		err := st(ctx, req.Args, send)
		if ctx.Err() == nil {
			if err != nil {
				writeResp(w, s.errResp(err))
			} else {
				writeResp(w, Response{OK: true, Done: true})
			}
			w.Flush()
		}
	case ok:
		v, err := h(ctx, req.Args)
		if err != nil {
			s.Log.Debug("ctl", "op", req.Op, "err", err, "took", time.Since(start))
			writeResp(c, s.errResp(err))
			return
		}
		b, err := json.Marshal(v)
		if err != nil {
			writeResp(c, Response{Error: err.Error(), Code: CodeInternal})
			return
		}
		writeResp(c, Response{OK: true, Data: b, Done: true})
	default:
		writeResp(c, Response{Error: fmt.Sprintf("unknown operation %q; the CLI and server versions may differ", req.Op), Code: CodeInvalid})
	}
}

func (s *Server) errResp(err error) Response {
	var e *Error
	if errors.As(err, &e) {
		return Response{Error: e.Msg, Code: e.Code}
	}
	code := CodeInternal
	if s.Classify != nil {
		code = s.Classify(err)
	}
	return Response{Error: err.Error(), Code: code}
}

func writeResp(w io.Writer, r Response) {
	json.NewEncoder(w).Encode(r)
}

// Client calls a server.
type Client struct {
	Path    string
	Timeout time.Duration
	// Dial replaces the unix socket, e.g. with a pipe through ssh.
	Dial func(ctx context.Context) (net.Conn, error)
}

// ErrNotRunning means nothing listens on the socket.
var ErrNotRunning = errors.New("lfr server is not running")

func (c Client) dial(ctx context.Context) (net.Conn, error) {
	if c.Dial != nil {
		return c.Dial(ctx)
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "unix", c.Path)
	if err != nil {
		var ne *net.OpError
		if errors.As(err, &ne) || errors.Is(err, os.ErrNotExist) {
			if errors.Is(err, os.ErrPermission) {
				return nil, fmt.Errorf("no permission to open %s: run as the lfr user or a member of its group", c.Path)
			}
			return nil, ErrNotRunning
		}
		return nil, err
	}
	return conn, nil
}

func send(conn net.Conn, op string, args any) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return json.NewEncoder(conn).Encode(Request{Op: op, Args: b})
}

// Call runs op and decodes its result into out (which may be nil).
func (c Client) Call(ctx context.Context, op string, args, out any) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	conn.SetDeadline(time.Now().Add(timeout))
	if err := send(conn, op, args); err != nil {
		return err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if !resp.OK {
		return &Error{Code: resp.Code, Msg: resp.Error}
	}
	if out != nil && len(resp.Data) > 0 {
		return json.Unmarshal(resp.Data, out)
	}
	return nil
}

// Stream runs a streaming op, calling fn for each value until ctx ends.
func (c Client) Stream(ctx context.Context, op string, args any, fn func(json.RawMessage) error) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() { <-ctx.Done(); conn.Close() }()
	if err := send(conn, op, args); err != nil {
		return err
	}
	dec := json.NewDecoder(conn)
	for {
		var resp Response
		if err := dec.Decode(&resp); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if !resp.OK {
			return &Error{Code: resp.Code, Msg: resp.Error}
		}
		if resp.Done {
			return nil
		}
		if err := fn(resp.Data); err != nil {
			return err
		}
	}
}
