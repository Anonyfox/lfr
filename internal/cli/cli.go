// Package cli is the lfr command line: the only way to operate a server.
// Every command except init, serve, version and skills talks to the running
// server over its local control socket.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/Anonyfox/lfr/internal/ctl"
	"github.com/Anonyfox/lfr/internal/engine"
)

// Build identifies the binary.
type Build struct{ Version, Commit, Date string }

// Env is what a command touches outside the file system.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Build          Build
	Getenv         func(string) string
}

func (e Env) getenv(k string) string {
	if e.Getenv == nil {
		return os.Getenv(k)
	}
	return e.Getenv(k)
}

// Exit codes.
const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitUsage      = 2
	ExitNotRunning = 3
	ExitInterrupt  = 130
)

// DefaultData is where lfr keeps its data unless --data or LFR_DATA say otherwise.
const DefaultData = "/var/lib/lfr"

type flag struct {
	name  string
	value bool // takes a value
	multi bool // may repeat
	usage string
}

type command struct {
	path    string // "mail send"
	args    string // usage of positional arguments
	summary string
	details string
	flags   []flag
	local   bool // runs without a server
	hidden  bool // plumbing, left out of the overview
	run     func(c *call) error
}

// call is one invocation.
type call struct {
	ctx   context.Context
	env   Env
	cmd   *command
	args  []string
	flags map[string][]string
	data  string
	json  bool
}

func (c *call) has(name string) bool { _, ok := c.flags[name]; return ok }
func (c *call) str(name string) string {
	if v := c.flags[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}
func (c *call) all(name string) []string { return c.flags[name] }
func (c *call) opt(name string) *string {
	if !c.has(name) {
		return nil
	}
	v := c.str(name)
	return &v
}

func (c *call) client() ctl.Client {
	if target := c.remote(); target != "" {
		return ctl.Client{Dial: ctl.SSH(target, c.env.getenv(RemoteDataEnv))}
	}
	if p := c.env.getenv(engine.SocketEnv); p != "" {
		return ctl.Client{Path: p}
	}
	return ctl.Client{Path: engine.SocketPath(c.data)}
}

// Remote operation: the CLI on a laptop drives the server over ssh.
const (
	RemoteEnv     = "LFR_REMOTE"      // user@host (or an ssh alias) running the lfr server
	RemoteDataEnv = "LFR_REMOTE_DATA" // the server's data directory when not the default
)

// remote is the ssh target of --remote or LFR_REMOTE, or "".
func (c *call) remote() string {
	if v := c.str("remote"); v != "" {
		return v
	}
	return c.env.getenv(RemoteEnv)
}

// do calls the server and decodes into out.
func (c *call) do(op string, args, out any) error {
	return c.client().Call(c.ctx, op, args, out)
}

// result prints v as JSON with --json, or calls human otherwise.
func (c *call) result(v any, human func()) error {
	if c.json {
		enc := json.NewEncoder(c.env.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	human()
	return nil
}

func (c *call) printf(format string, a ...any) { fmt.Fprintf(c.env.Stdout, format, a...) }

// next prints the suggested next step, which agents follow.
func (c *call) next(format string, a ...any) {
	if !c.json {
		fmt.Fprintf(c.env.Stdout, "\nNext: "+format+"\n", a...)
	}
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usage(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

var globalFlags = []flag{
	{name: "json", usage: "print the result as JSON"},
	{name: "data", value: true, usage: "data directory (default " + DefaultData + ", or LFR_DATA)"},
	{name: "remote", value: true, usage: "drive the server at user@host over ssh (or LFR_REMOTE)"},
}

// Run executes args and returns the exit code.
func Run(ctx context.Context, env Env, args []string) int {
	if env.Stdin == nil {
		env.Stdin = os.Stdin
	}
	if len(args) == 0 {
		return start(ctx, env)
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return help(env, args[1:])
	}
	if args[0] == "--version" || args[0] == "-v" {
		args = []string{"version"}
	}
	// Global flags may come before the command: lfr --json status.
	var lead []string
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		name, _, hasValue := strings.Cut(args[0][2:], "=")
		f, ok := lookupFlag(globalFlags, name)
		if !ok {
			break
		}
		lead = append(lead, args[0])
		args = args[1:]
		if f.value && !hasValue && len(args) > 0 {
			lead = append(lead, args[0])
			args = args[1:]
		}
	}
	if len(args) == 0 {
		return start(ctx, env)
	}
	cmd, rest := find(args)
	rest = append(append([]string{}, rest...), lead...)
	if cmd == nil && len(args) == 1 && group(args[0]) != "" {
		return help(env, args)
	}
	if cmd == nil {
		fmt.Fprintf(env.Stderr, "lfr: unknown command %q.%s\nRun lfr help for every command.\n", strings.Join(args[:min(2, len(args))], " "), suggest(args))
		return ExitUsage
	}
	c := &call{ctx: ctx, env: env, cmd: cmd, flags: map[string][]string{}}
	helpWanted, err := c.parse(rest)
	if err != nil {
		fmt.Fprintf(env.Stderr, "lfr %s: %v\nUsage: %s\n", cmd.path, err, synopsis(cmd))
		return ExitUsage
	}
	if helpWanted {
		printHelp(env.Stdout, cmd)
		return ExitOK
	}
	c.json = c.has("json")
	c.data = c.str("data")
	if c.data == "" {
		c.data = env.getenv("LFR_DATA")
	}
	if c.data == "" {
		c.data = DefaultData
	}
	err = cmd.run(c)
	return c.exit(err)
}

func (c *call) exit(err error) int {
	if err == nil {
		return ExitOK
	}
	if c.ctx.Err() != nil {
		fmt.Fprintf(c.env.Stderr, "lfr %s: %v\n", c.cmd.path, err)
		if c.json {
			json.NewEncoder(c.env.Stdout).Encode(map[string]any{"error": err.Error(), "code": "interrupted", "exit": ExitInterrupt})
		}
		return ExitInterrupt
	}
	var ue usageError
	var ce *ctl.Error
	var se *silentErr
	code, msg, next, exit := "failed", "", "", ExitFailure
	switch {
	case errors.As(err, &se):
		return ExitFailure // already reported, in JSON too
	case errors.As(err, &ue):
		code, msg, exit = "usage", ue.msg, ExitUsage
		next = "lfr help " + c.cmd.path
		fmt.Fprintf(c.env.Stderr, "lfr %s: %s\nUsage: %s\n", c.cmd.path, ue.msg, synopsis(c.cmd))
	case errors.Is(err, ctl.ErrNotRunning) && c.remote() != "":
		code, msg, exit = "not_running", "no server is running on "+c.remote(), ExitNotRunning
		next = "start it there: systemctl start lfr"
		fmt.Fprintf(c.env.Stderr, "lfr: no server is running on %s.\nStart it there with: systemctl start lfr\n", c.remote())
	case errors.Is(err, ctl.ErrNotRunning):
		code, msg, exit = "not_running", "no server is running for "+c.data, ExitNotRunning
		next = "systemctl start lfr (or lfr serve --data " + c.data + "), or --remote user@host for a server elsewhere"
		fmt.Fprintf(c.env.Stderr, "lfr: no server is running for %s.\nStart it with: systemctl start lfr (or lfr serve --data %s)\n", c.data, c.data)
	case errors.As(err, &ce):
		code, msg = ce.Code, ce.Msg
		if ce.Code == ctl.CodeInvalid {
			exit = ExitUsage
		}
		fmt.Fprintf(c.env.Stderr, "lfr %s: %s\n", c.cmd.path, ce.Msg)
	default:
		msg = err.Error()
		fmt.Fprintf(c.env.Stderr, "lfr %s: %v\n", c.cmd.path, err)
	}
	// An agent reading --json output gets JSON in every case, failures
	// included, with the exit code and the step that helps.
	if c.json {
		out := map[string]any{"error": msg, "code": code, "exit": exit}
		if next != "" {
			out["next"] = next
		}
		json.NewEncoder(c.env.Stdout).Encode(out)
	}
	return exit
}

// parse reads flags anywhere among the arguments; "--" ends flags.
func (c *call) parse(args []string) (bool, error) {
	specs := append(append([]flag{}, c.cmd.flags...), globalFlags...)
	helpWanted := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			c.args = append(c.args, args[i+1:]...)
			break
		}
		if a == "-h" || a == "--help" {
			helpWanted = true
			continue
		}
		if a == "-o" {
			a = "--out"
		}
		if !strings.HasPrefix(a, "--") {
			if strings.HasPrefix(a, "-") && a != "-" && !isNumber(a) {
				return false, fmt.Errorf("unknown flag %s; flags are spelled --name", a)
			}
			c.args = append(c.args, a)
			continue
		}
		name, value, hasValue := strings.Cut(a[2:], "=")
		spec, ok := lookupFlag(specs, name)
		if !ok {
			var names []string
			for _, s := range specs {
				names = append(names, "--"+s.name)
			}
			return false, fmt.Errorf("unknown flag --%s%s", name, didYouMean("--"+name, names))
		}
		if _, seen := c.flags[name]; seen && !spec.multi {
			return false, fmt.Errorf("--%s given twice", name)
		}
		switch {
		case !spec.value && hasValue:
			if value != "true" && value != "false" {
				return false, fmt.Errorf("--%s takes no value (or =true / =false)", name)
			}
		case spec.value && !hasValue:
			if i+1 >= len(args) {
				return false, fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		c.flags[name] = append(c.flags[name], value)
	}
	return helpWanted, nil
}

func isNumber(s string) bool {
	for _, r := range strings.TrimPrefix(s, "-") {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 1
}

func lookupFlag(specs []flag, name string) (flag, bool) {
	for _, s := range specs {
		if s.name == name {
			return s, true
		}
	}
	return flag{}, false
}

// boolFlag reads a flag given as --x, --x=true or --x=false.
func (c *call) boolFlag(name string) *bool {
	if !c.has(name) {
		return nil
	}
	v := c.str(name) != "false"
	return &v
}

// find picks the longest command path matching the arguments.
func find(args []string) (*command, []string) {
	var best *command
	bestLen := 0
	for i := range commands {
		cmd := &commands[i]
		parts := strings.Fields(cmd.path)
		if len(parts) > len(args) || len(parts) <= bestLen {
			continue
		}
		match := true
		for j, p := range parts {
			if args[j] != p && !alias(p, args[j]) {
				match = false
				break
			}
		}
		if match {
			best, bestLen = cmd, len(parts)
		}
	}
	if best == nil {
		return nil, nil
	}
	return best, args[bestLen:]
}

// aliases accepted for verbs and nouns.
var aliases = map[string][]string{
	"ls": {"list"}, "rm": {"remove", "delete"}, "mv": {"move"}, "show": {"info", "get"},
	"new": {"create", "draft"}, "mailbox": {"mailboxes", "box"}, "domain": {"domains"}, "list": {"lists"},
	"reader": {"readers", "subscriber", "subscribers", "sub", "subs"}, "issue": {"issues"},
	"mail": {"inbox", "message", "messages"}, "read": {"open", "cat"},
}

func alias(canonical, given string) bool {
	for _, a := range aliases[canonical] {
		if a == given {
			return true
		}
	}
	return false
}

func synopsis(cmd *command) string {
	s := "lfr " + cmd.path
	if cmd.args != "" {
		s += " " + cmd.args
	}
	if len(cmd.flags) > 0 {
		s += " [flags]"
	}
	return s
}

func suggest(args []string) string {
	var names []string
	seen := map[string]bool{}
	for _, c := range commands {
		first := strings.Fields(c.path)[0]
		if !seen[first] {
			seen[first] = true
			names = append(names, first)
		}
	}
	return didYouMean(args[0], names)
}

func didYouMean(s string, options []string) string {
	best, dist := "", 3
	for _, o := range options {
		if d := levenshtein(s, o); d < dist {
			best, dist = o, d
		}
	}
	if best == "" {
		return ""
	}
	return " Did you mean " + best + "?"
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func help(env Env, args []string) int {
	if len(args) > 0 {
		if cmd, _ := find(args); cmd != nil {
			printHelp(env.Stdout, cmd)
			return ExitOK
		}
		var matches []string
		word := group(args[0])
		for _, c := range commands {
			if first := strings.Fields(c.path)[0]; first == word || first == args[0] {
				matches = append(matches, c.path)
			}
		}
		if len(matches) == 0 {
			fmt.Fprintf(env.Stderr, "lfr: no command %q.%s\n", strings.Join(args, " "), suggest(args))
			return ExitUsage
		}
		for _, m := range matches {
			cmd, _ := find(strings.Fields(m))
			fmt.Fprintf(env.Stdout, "  %-34s %s\n", synopsisShort(cmd), cmd.summary)
		}
		return ExitOK
	}
	fmt.Fprint(env.Stdout, overview())
	return ExitOK
}

func synopsisShort(cmd *command) string {
	s := cmd.path
	if cmd.args != "" {
		s += " " + cmd.args
	}
	return s
}

func printHelp(w io.Writer, cmd *command) {
	fmt.Fprintf(w, "%s\n\nUsage: %s\n", cmd.summary, synopsis(cmd))
	if cmd.details != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(cmd.details))
	}
	if len(cmd.flags) > 0 {
		fmt.Fprintln(w, "\nFlags:")
		for _, f := range cmd.flags {
			name := "--" + f.name
			if f.value {
				name += " <value>"
			}
			if f.multi {
				name += " (repeatable)"
			}
			fmt.Fprintf(w, "  %-28s %s\n", name, f.usage)
		}
	}
	// A command that is also a group word lists what lives under it.
	var subs []*command
	for i := range commands {
		if c := &commands[i]; strings.HasPrefix(c.path, cmd.path+" ") && !c.hidden {
			subs = append(subs, c)
		}
	}
	if len(subs) > 0 {
		fmt.Fprintln(w, "\nSubcommands:")
		for _, c := range subs {
			fmt.Fprintf(w, "  %-34s %s\n", synopsisShort(c), c.summary)
		}
	}
	fmt.Fprintln(w, "\nEvery command takes --json for machine-readable output and --data <dir>.")
}

func overview() string {
	var b strings.Builder
	b.WriteString(`lfr runs mail for your domains: mailboxes, and newsletters with their websites.
One binary on one small server, operated through this CLI.
`)
	for _, sec := range sections {
		fmt.Fprintf(&b, "\n%s\n", sec.name)
		for _, w := range sec.words {
			fmt.Fprintf(&b, "  %-10s %s\n", w, wordSummary(w))
		}
	}
	b.WriteString(`
lfr help <command> shows its subcommands and flags. --json works on every command,
--remote user@host runs it against a server over ssh.
Exit codes: 0 ok, 1 failed, 2 wrong usage or invalid input, 3 server not running.
`)
	return b.String()
}

// group returns the canonical top-level word for w, or "".
func group(w string) string {
	for _, sec := range sections {
		for _, g := range sec.words {
			if g == w || alias(g, w) {
				return g
			}
		}
	}
	return ""
}

// wordSummary describes a top-level word: its own command, or its group.
func wordSummary(w string) string {
	if s, ok := groupSummary[w]; ok {
		return s
	}
	for _, c := range commands {
		if c.path == w {
			return c.summary
		}
	}
	return ""
}

// start is bare `lfr`: what it is, the state of this machine and the next step.
func start(ctx context.Context, env Env) int {
	fmt.Fprint(env.Stdout, "lfr runs mail for your domains: mailboxes, and newsletters with their websites.\n\n")
	data := env.getenv("LFR_DATA")
	if data == "" {
		data = DefaultData
	}
	c := &call{ctx: ctx, env: env, data: data, flags: map[string][]string{}}
	var st struct {
		Hostname  string   `json:"hostname"`
		Domains   int      `json:"domains"`
		Mailboxes int      `json:"mailboxes"`
		Lists     int      `json:"lists"`
		Next      []string `json:"next"`
	}
	err := c.do("status", nil, &st)
	switch {
	case errors.Is(err, ctl.ErrNotRunning):
		if _, serr := os.Stat(data + "/" + engine.DBFile); serr != nil {
			if runtime.GOOS != "linux" {
				// A laptop: the server runs elsewhere.
				fmt.Fprintf(env.Stdout, "No lfr server on this machine.\n\nNext: LFR_REMOTE=user@your-server lfr status reaches a server over ssh; a new server needs Linux: install lfr there and run sudo lfr setup --hostname mail.example.com --systemd\n")
			} else {
				fmt.Fprintf(env.Stdout, "No lfr data in %s yet.\n\nNext: sudo lfr setup --hostname mail.example.com --systemd (or LFR_REMOTE=user@server to use a server elsewhere)\n", data)
			}
		} else {
			fmt.Fprintf(env.Stdout, "Data in %s, but the server is not running.\n\nNext: sudo systemctl start lfr (or lfr serve --data %s)\n", data, data)
		}
	case err != nil:
		fmt.Fprintf(env.Stdout, "Server for %s: %v\n", data, err)
	default:
		fmt.Fprintf(env.Stdout, "Running as %s with %d domains, %d mailboxes and %d lists.\n", st.Hostname, st.Domains, st.Mailboxes, st.Lists)
		for _, n := range st.Next {
			fmt.Fprintf(env.Stdout, "\nNext: %s\n", n)
		}
	}
	fmt.Fprint(env.Stdout, "\nlfr help lists every command; lfr skills install teaches your agent.\n")
	return ExitOK
}

func sortedKeys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
