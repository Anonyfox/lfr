package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/concept"
	"github.com/Anonyfox/lfr/internal/craft"
	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/dnsx/dnsxtest"
	"github.com/Anonyfox/lfr/internal/engine"
	"github.com/Anonyfox/lfr/internal/project"
	"github.com/Anonyfox/lfr/internal/skills"
)

func run(t *testing.T, env map[string]string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(context.Background(), Env{Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb, Build: Build{Version: "test"},
		Getenv: func(k string) string { return env[k] }}, args)
	return code, out.String(), errb.String()
}

func TestEveryCommandHasHelpAndUniquePath(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands {
		if seen[c.path] {
			t.Errorf("duplicate command %q", c.path)
		}
		seen[c.path] = true
		if c.summary == "" || c.run == nil {
			t.Errorf("command %q lacks summary or run", c.path)
		}
		code, out, _ := run(t, nil, "", append(strings.Fields(c.path), "--help")...)
		if code != 0 || !strings.Contains(out, "Usage: lfr "+c.path) {
			t.Errorf("help for %q: %d %q", c.path, code, out)
		}
	}
}

func TestParsingAndErrors(t *testing.T) {
	code, _, errs := run(t, nil, "", "mial", "ls")
	if code != ExitUsage || !strings.Contains(errs, "Did you mean mail?") {
		t.Fatalf("%d %q", code, errs)
	}
	code, _, errs = run(t, nil, "", "mail", "ls", "--unred")
	if code != ExitUsage || !strings.Contains(errs, "Did you mean --unread?") {
		t.Fatalf("%d %q", code, errs)
	}
	code, _, errs = run(t, map[string]string{"LFR_DATA": "/nonexistent-lfr"}, "", "status")
	if code != ExitNotRunning || !strings.Contains(errs, "no server is running") {
		t.Fatalf("%d %q", code, errs)
	}
	code, out, _ := run(t, nil, "", "help")
	if code != 0 || !strings.Contains(out, "\nNewsletters\n") || !strings.Contains(out, "  reader ") || strings.Contains(out, "proxy") {
		t.Fatal(out)
	}
	code, out, _ = run(t, nil, "", "issues")
	if code != 0 || !strings.Contains(out, "issue show <issue>") {
		t.Fatal(out)
	}
	if code, out, _ := run(t, nil, "", "skills"); code != 0 || !strings.Contains(out, "concept") {
		t.Fatal(out)
	}
	if code, out, _ := run(t, nil, "", "skills", "mailbox"); code != 0 || !strings.Contains(out, "name: lfr-mailbox") {
		t.Fatal(out)
	}
	// Aliases and flags in any position.
	if c, _ := find([]string{"lists", "ls"}); c == nil || c.path != "list ls" {
		t.Fatal("alias")
	}
	if c, rest := find([]string{"mail", "read", "5"}); c.path != "mail read" || rest[0] != "5" {
		t.Fatal("find")
	}
}

// TestAgainstServer drives a real server through the CLI like an agent would.
// serve starts a server in a fresh data directory until the test ends.
func serve(t *testing.T) (dir string, env map[string]string) {
	t.Helper()
	dir, _ = os.MkdirTemp("/tmp", "lfrcli")
	t.Cleanup(func() { os.RemoveAll(dir) })
	env = map[string]string{"LFR_DATA": dir}
	if code, out, errs := run(t, env, "", "setup", "--hostname", "mail.example.com", "--ipv4", "203.0.113.7"); code != 0 {
		t.Fatalf("setup %d %s %s", code, out, errs)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	smtpAddr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	e, err := engine.Open(ctx, engine.Options{Data: dir, SMTPAddr: smtpAddr, DNS: dnsxtest.New(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done; e.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if code, _, _ := run(t, env, "", "status"); code == 0 {
			return dir, env
		}
		if time.Now().After(deadline) {
			t.Fatal("server not up")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestHelperLfr runs as `lfr` inside the fake ssh of TestRemote.
func TestHelperLfr(t *testing.T) {
	if os.Getenv("LFR_TEST_HELPER") == "" {
		t.Skip("helper process")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	os.Exit(Run(context.Background(), Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Build: Build{Version: "test"}}, args))
}

// TestRemote drives a server through --remote, with ssh replaced by a
// script that runs the remote command locally.
func TestRemote(t *testing.T) {
	dir, _ := serve(t)
	bin := t.TempDir()
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -o) shift 2;; -*) shift;; *) break;; esac; done\nshift\n" +
		"eval set -- \"$@\"\nshift\nLFR_TEST_HELPER=1 exec \"$LFR_TEST_BIN\" -test.run='^TestHelperLfr$' -- \"$@\"\n"
	if err := os.WriteFile(bin+"/ssh", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	t.Setenv("LFR_TEST_BIN", exe)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	env := map[string]string{"LFR_DATA": "/nonexistent", "LFR_REMOTE": "ana@vps", "LFR_REMOTE_DATA": dir}
	if code, out, errs := run(t, env, "", "domain", "add", "example.com"); code != 0 || !strings.Contains(out, "_dmarc.example.com") {
		t.Fatalf("remote domain add: %d %s %s", code, out, errs)
	}
	if code, out, _ := run(t, env, "", "domain", "ls", "--json"); code != 0 || !strings.Contains(out, `"example.com"`) {
		t.Fatalf("remote json: %d %s", code, out)
	}
	target := filepath.Join(t.TempDir(), "remote-backup")
	if code, out, errs := run(t, env, "", "backup", target, "--json"); code != 0 || !strings.Contains(out, target) {
		t.Fatalf("remote backup: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(target, "BACKUP-COMPLETE")); err != nil {
		t.Fatal(err)
	}
	env["LFR_REMOTE_DATA"] = "/nonexistent-lfr"
	if code, _, errs := run(t, env, "", "status"); code != ExitNotRunning || !strings.Contains(errs, "no server is running on ana@vps") {
		t.Fatalf("remote not running: %d %s", code, errs)
	}
}

func TestAgainstServer(t *testing.T) {
	_, env := serve(t)
	steps := []struct {
		stdin string
		args  []string
		want  string
		code  int
	}{
		{"", []string{"domain", "add", "example.com"}, "_dmarc.example.com", 0},
		{"", []string{"domain", "add", "example.com"}, "", 1},
		{"", []string{"mailbox", "add", "jo@example.com", "--name", "Jo"}, "Created jo@example.com", 0},
		{"", []string{"mailbox", "ls"}, "jo@example.com", 0},
		{"", []string{"list", "add", "news", "--name", "News", "--from", "news@example.com", "--address", "News, Weg 1, 10115 Berlin"}, "/lists/news/", 0},
		{"", []string{"list", "set", "news", "--language", "nl"}, "lfr list text news --import nl.json", 0},
		{"{\"confirm.subject\":\"Bevestig %s\",\"form.submit\":\"Aanmelden\"}", []string{"list", "text", "news", "--import", "-"}, "Aanmelden", 0},
		{"", []string{"list", "text", "news", "confirm.subject", "Bevestigen %s"}, "Bevestigen %s", 0},
		{"{\"form.submit\":\"SHOULD NOT APPLY\",\"confirm.subject\":\"missing placeholder\"}", []string{"list", "text", "news", "--import", "-"}, "", 2},
		{"", []string{"list", "text", "news", "--json"}, "\"Aanmelden\"", 0},
		{"", []string{"list", "text", "news", "confirm.subject", "--reset"}, "Please confirm", 0},
		{"", []string{"reader", "add", "news", "a@x.org", "--confirmed", "--consent", "Old form"}, "1 added", 0},
		{"email\nb@x.org\n", []string{"reader", "add", "news", "--csv", "-", "--confirmed", "--consent", "Old form"}, "1 added", 0},
		{"", []string{"reader", "ls", "news", "--csv"}, "b@x.org", 0},
		{"", []string{"mailbox", "alias", "hi@example.com", "jo@example.com"}, "hi@example.com", 0},
		{"", []string{"mailbox", "rm", "hi@example.com"}, "Removed alias", 0},
		{"", []string{"mailbox", "rm", "jo@example.com"}, "", 2}, // a mailbox needs --yes
		{"", []string{"domain", "set", "example.com", "--catchall", "jo@example.com"}, "go to jo@example.com", 0},
		{"# Hello\n\nFirst issue.\n", []string{"issue", "put", "news", "-"}, "Created issue news/hello", 0},
		{"", []string{"issue", "show", "news/hello"}, "list.cadence", 0},
		{"", []string{"list", "set", "news", "--pause", "testing"}, "Paused news", 0},
		{"", []string{"list", "set", "news", "--resume"}, "Resumed news", 0},
		{"", []string{"issue", "send", "1"}, "", 2}, // warnings need --force
		{"", []string{"issue", "send", "1", "--force"}, "status:", 0},
		{"Hi there", []string{"mail", "send", "--from", "jo@example.com", "--to", "b@y.org", "--subject", "Hi", "--text", "-"}, "Queued for 1", 0},
		{"", []string{"mail", "ls", "--folder", "sent"}, "Hi", 0},
		{"", []string{"mail", "read", "notanumber"}, "", 2},
		{"", []string{"config", "relay.rate"}, "10", 0},
		{"", []string{"config", "relay.rate", "zero"}, "", 2},
		{"", []string{"mail", "ls", "--search", "Hi", "--folder", "sent"}, "Hi", 0},
		{"", []string{"events", "--since", "1h"}, "", 0},
		{"", []string{"config", "nonsense"}, "", 2},
		{"", []string{"list", "rm", "news"}, "", 2},
		{"", []string{"queue"}, "queued", 0},
	}
	for _, s := range steps {
		code, out, errs := run(t, env, s.stdin, s.args...)
		if code != s.code || !strings.Contains(out+errs, s.want) {
			t.Errorf("lfr %s: exit %d (want %d)\n%s%s", strings.Join(s.args, " "), code, s.code, out, errs)
		}
	}
	backupDir := filepath.Join(t.TempDir(), "backup")
	if code, out, errs := run(t, env, "", "backup", backupDir, "--json"); code != 0 || !strings.Contains(out, backupDir) {
		t.Fatalf("backup: %d %s %s", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "BACKUP-COMPLETE")); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(t, env, "", "mailbox", "ls", "--json")
	var boxes []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &boxes) != nil || len(boxes) != 3 {
		t.Fatalf("json output: %d %s", code, out)
	}
	code, out, _ = run(t, env, "", "--json", "mail", "read", "999")
	if code != ExitFailure || !strings.Contains(out, `"code": "not_found"`) && !strings.Contains(out, `"code":"not_found"`) {
		t.Fatalf("json error: %d %s", code, out)
	}
}

// TestRuleCitations holds every rule's help to the research it cites.
func TestRuleCitations(t *testing.T) {
	ids := craft.IDs()
	seen := map[string]bool{}
	for _, g := range ruleGroups() {
		for _, r := range g.rules {
			if seen[r.ID] {
				t.Errorf("rule %s twice", r.ID)
			}
			seen[r.ID] = true
			for _, id := range concept.Citations(r.Help) {
				if !ids[id] {
					t.Errorf("rule %s cites %s, which lfr's research does not hold", r.ID, id)
				}
			}
		}
	}
}

// TestSkillsMatchTheBinary holds every skill to the lfr that prints it: the
// commands it names exist, the flags it uses exist, and the research it
// cites is lfr's.
func TestSkillsMatchTheBinary(t *testing.T) {
	all, err := skills.All(skills.Data{Version: "test", Project: project.Facts()})
	if err != nil {
		t.Fatal(err)
	}
	ids := craft.IDs()
	cmdRE := regexp.MustCompile("`lfr ([a-z][a-z ]*[a-z])((?: [^`]*)?)`")
	flagRE := regexp.MustCompile(` --([a-z][a-z-]+)`)
	checked := 0
	defer func() {
		if checked < 40 {
			t.Errorf("only %d commands found in the skills; the pattern is broken", checked)
		}
	}()
	for _, s := range all {
		if !strings.HasPrefix(s.Name, "lfr-") || len(s.Description) > 1024 || strings.Contains(s.Text, "[[") {
			t.Errorf("%s: name, description or unrendered template", s.Name)
		}
		for _, id := range concept.Citations(s.Text) {
			if strings.HasPrefix(id, craft.Prefix) && !ids[id] {
				t.Errorf("%s cites %s, which lfr's research does not hold", s.Name, id)
			}
		}
		for _, m := range cmdRE.FindAllStringSubmatch(s.Text, -1) {
			words := strings.Fields(m[1] + m[2])
			cmd, _ := find(words)
			if cmd == nil {
				t.Errorf("%s names `lfr %s`, which does not exist", s.Name, m[1]+m[2])
				continue
			}
			checked++
			known := map[string]bool{"json": true, "data": true}
			for _, f := range cmd.flags {
				known[f.name] = true
			}
			for _, f := range flagRE.FindAllStringSubmatch(m[2], -1) {
				if !known[f[1]] {
					t.Errorf("%s uses `lfr %s --%s`, which has no such flag", s.Name, cmd.path, f[1])
				}
			}
		}
	}
}

// With --json an agent gets JSON on stdout for failures too, with the exit
// code and the next step.
func TestJSONErrors(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		exit int
		code string
	}{
		{[]string{"status", "--json", "--data", dir}, ExitNotRunning, "not_running"},
		{[]string{"issue", "show", "--json"}, ExitUsage, "usage"},
	} {
		code, out, _ := run(t, nil, "", c.args...)
		var got struct {
			Error, Code, Next string
			Exit              int
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil || code != c.exit || got.Exit != c.exit || got.Code != c.code || got.Error == "" || got.Next == "" {
			t.Errorf("%v: exit %d, stdout %q", c.args, code, out)
		}
	}
}

func TestBackupRejectsEscapingFrame(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir, err := os.MkdirTemp("/tmp", "lfr-bad-backup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	listener, err := net.Listen("unix", filepath.Join(dir, "lfr.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server.Stream(api.OpBackup, func(_ context.Context, _ json.RawMessage, send func(any) error) error {
		return send(api.BackupFrame{Path: "../escape", Data: []byte("bad")})
	})
	go server.Serve(ctx, listener)
	target := filepath.Join(t.TempDir(), "backup")
	code, _, errs := run(t, map[string]string{"LFR_DATA": dir}, "", "backup", target)
	if code == 0 || !strings.Contains(errs, "incomplete") {
		t.Fatalf("%d %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(target, "BACKUP-COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("completion marker exists")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), "escape")); !os.IsNotExist(err) {
		t.Fatal("escaped file exists")
	}
}

func TestBackupCancellationReportsIncomplete(t *testing.T) {
	serverCtx, stop := context.WithCancel(context.Background())
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir, err := os.MkdirTemp("/tmp", "lfr-cancel-backup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	listener, err := net.Listen("unix", filepath.Join(dir, "lfr.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &ctl.Server{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	server.Stream(api.OpBackup, func(_ context.Context, _ json.RawMessage, send func(any) error) error {
		err := send(api.BackupFrame{Path: "lfr.db", Data: []byte("partial")})
		cancel()
		return err
	})
	go server.Serve(serverCtx, listener)
	target := filepath.Join(t.TempDir(), "backup")
	var out, errs bytes.Buffer
	code := Run(ctx, Env{Stdout: &out, Stderr: &errs, Getenv: func(k string) string {
		if k == "LFR_DATA" {
			return dir
		}
		return ""
	}}, []string{"backup", target, "--json"})
	if code != ExitInterrupt || !strings.Contains(errs.String(), "incomplete") || !strings.Contains(out.String(), "\"interrupted\"") {
		t.Fatalf("%d %s %s", code, &out, &errs)
	}
	if _, err := os.Stat(filepath.Join(target, "BACKUP-COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("completion marker exists")
	}
}
