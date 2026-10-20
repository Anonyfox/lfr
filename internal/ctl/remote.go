package ctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SSH returns a Dial that reaches the control socket of the lfr server at
// target ("user@host" or an ssh config alias) by running `lfr proxy` there
// over ssh. Connections share one ssh session for a minute, so a command
// that makes many calls pays the handshake once.
func SSH(target, remoteData string) func(ctx context.Context) (net.Conn, error) {
	return func(ctx context.Context) (net.Conn, error) {
		args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
		if dir, err := os.UserHomeDir(); err == nil {
			args = append(args, "-o", "ControlMaster=auto", "-o", "ControlPersist=60",
				"-o", "ControlPath="+filepath.Join(dir, ".ssh", "lfr-%C"))
		}
		remote := "lfr proxy"
		if remoteData != "" {
			remote += " --data " + shellQuote(remoteData)
		}
		args = append(args, target, remote)
		cmd := exec.Command("ssh", args...)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, err
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("ssh %s: %w", target, err)
		}
		return &pipeConn{cmd: cmd, w: stdin, r: stdout, stderr: &stderr, target: target}, nil
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// pipeConn is a net.Conn over the pipes of an ssh process.
type pipeConn struct {
	cmd    *exec.Cmd
	w      io.WriteCloser
	r      io.ReadCloser
	stderr *bytes.Buffer
	target string
	once   sync.Once
}

// wait reaps the process once; afterwards stderr is complete.
func (p *pipeConn) wait() {
	p.once.Do(func() {
		done := make(chan struct{})
		go func() { p.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			p.cmd.Process.Kill()
			<-done
		}
	})
}

func (p *pipeConn) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if errors.Is(err, io.EOF) && n == 0 {
		// The remote side ended without answering: surface why.
		p.w.Close()
		p.wait()
		if msg := strings.TrimSpace(p.stderr.String()); msg != "" {
			if strings.Contains(msg, "not running") || strings.Contains(msg, "no server is running") {
				return 0, ErrNotRunning
			}
			return 0, fmt.Errorf("ssh %s: %s", p.target, msg)
		}
	}
	return n, err
}

func (p *pipeConn) Write(b []byte) (int, error) { return p.w.Write(b) }

func (p *pipeConn) Close() error {
	p.w.Close()
	p.wait()
	return nil
}

func (p *pipeConn) LocalAddr() net.Addr                { return pipeAddr(p.target) }
func (p *pipeConn) RemoteAddr() net.Addr               { return pipeAddr(p.target) }
func (p *pipeConn) SetDeadline(t time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(t time.Time) error { return nil }

type pipeAddr string

func (a pipeAddr) Network() string { return "ssh" }
func (a pipeAddr) String() string  { return string(a) }

// Proxy copies between in/out and the control socket at path until either
// side closes: the remote end of SSH.
func Proxy(ctx context.Context, path string, in io.Reader, out io.Writer) error {
	conn, err := Client{Path: path}.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, in)
		if uc, ok := conn.(*net.UnixConn); ok {
			uc.CloseWrite()
		}
		done <- err
	}()
	go func() { _, err := io.Copy(out, conn); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			err = <-done
		}
		return err
	case <-ctx.Done():
		return nil
	}
}
