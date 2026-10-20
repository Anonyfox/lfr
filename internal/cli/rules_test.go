package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Anonyfox/lfr/internal/project"
	"github.com/Anonyfox/lfr/internal/skills"
)

// Every rule ID the code can emit is in the catalog lfr rules prints, so an
// agent can always look up why a finding exists (ported from lfm).
func TestCatalogCoversEmittedRuleIDs(t *testing.T) {
	known := map[string]bool{}
	for _, g := range ruleGroups() {
		for _, r := range g.rules {
			known[r.ID] = true
		}
	}
	ruleID := regexp.MustCompile(`^[a-z0-9]+\.[a-z0-9-]+$`)
	// Constants per package directory, so identifiers resolve.
	consts := map[string]map[string]string{}
	files := map[string]*ast.File{}
	err := filepath.WalkDir("..", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		files[path] = f
		dir := filepath.Dir(path)
		if consts[dir] == nil {
			consts[dir] = map[string]string{}
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							consts[dir][name.Name], _ = strconv.Unquote(lit.Value)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, f := range files {
		value := func(e ast.Expr) string {
			switch v := e.(type) {
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					s, _ := strconv.Unquote(v.Value)
					return s
				}
			case *ast.Ident:
				return consts[filepath.Dir(path)][v.Name]
			}
			return ""
		}
		check := func(e ast.Expr) {
			if id := value(e); ruleID.MatchString(id) && !known[id] {
				t.Errorf("%s emits rule %q, which lfr rules does not know", path, id)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.KeyValueExpr:
				if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Rule" {
					check(n.Value)
				}
			case *ast.CallExpr:
				name := ""
				switch fn := n.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				switch name {
				case "add", "note", "warn":
					for _, a := range n.Args[:min(2, len(n.Args))] {
						check(a)
					}
				}
			}
			return true
		})
	}
}

// Commands named in Go strings (hints, fixes, Next: lines) exist: a hint
// like "lfr list pause" sends an agent into a usage error.
func TestCommandsInStringsExist(t *testing.T) {
	paths := map[string]bool{}
	firsts := map[string]bool{}
	for _, c := range commands {
		paths[c.path] = true
		firsts[strings.Fields(c.path)[0]] = true
	}
	mention := regexp.MustCompile(`\blfr ([a-z]+) ([a-z][a-z-]*)`)
	err := filepath.WalkDir("..", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, _ := strconv.Unquote(lit.Value)
			for _, m := range mention.FindAllStringSubmatch(s, -1) {
				word, sub := m[1], m[2]
				// Only group words: "lfr check dir" passes an argument. Prose
				// ("lfr mail and newsletter server") is not a command.
				prose := map[string]bool{"and": true, "or": true, "is": true, "for": true, "to": true, "with": true, "on": true, "in": true}
				if firsts[word] && !paths[word] && !paths[word+" "+sub] && !prose[sub] {
					t.Errorf("%s names %q, which is no command", path, "lfr "+word+" "+sub)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Every skill loads in any Agent Skills client and stays small enough to be
// read in full (https://agentskills.io/specification; ported from lfm).
func TestSkillsFollowTheSpec(t *testing.T) {
	all, err := skills.All(skills.Data{Version: "1.2.3", Project: project.Facts()})
	if err != nil || len(all) == 0 {
		t.Fatal(err, len(all))
	}
	name := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, s := range all {
		t.Run(s.Topic, func(t *testing.T) {
			if !name.MatchString(s.Name) || len(s.Name) > 64 || s.Name != "lfr-"+s.Topic {
				t.Errorf("name %q must be lfr-<topic>, lowercase and hyphenated", s.Name)
			}
			if n := utf8.RuneCountInString(s.Description); n > 1024 || n < 200 {
				t.Errorf("description has %d characters; keep it between 200 and 1024", n)
			}
			if !strings.Contains(s.Description, "Use ") || !strings.Contains(s.Description, "Not for") {
				t.Error("description must say when to use the skill and when not")
			}
			header, body, _ := strings.Cut(strings.TrimPrefix(s.Text, "---\n"), "\n---\n")
			for _, line := range strings.Split(header, "\n") {
				key, value, _ := strings.Cut(line, ": ")
				if strings.Contains(value, ": ") || strings.ContainsAny(value, "<>") {
					t.Errorf("frontmatter %s holds YAML-unsafe text: %q", key, value)
				}
			}
			if lines := strings.Count(body, "\n"); lines > 250 {
				t.Errorf("body has %d lines; keep skills under 250 so they are read in full", lines)
			}
			if tokens := len(body) / 4; tokens > 5000 {
				t.Errorf("body is about %d tokens; the spec budget is 5000", tokens)
			}
			for _, banned := range []string{"[[", "]]", "—", "TODO", "MUST", "NEVER", "mcp__", "WebSearch", "WebFetch"} {
				if strings.Contains(s.Text, banned) {
					t.Errorf("skill contains %q", banned)
				}
			}
			if !strings.Contains(s.Text, "lfr-version: \"1.2.3\"") || !strings.Contains(s.Text, "lfr-sha256: \"") {
				t.Error("skill does not name the version that printed it")
			}
		})
	}
}

// A fresh project checks clean: the first thing an agent sees is the next
// step, not a warning about something it has not reached yet (from lfm).
func TestInitThenCheckIsClean(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "news")
	if code, _, errOut := run(t, nil, "", "init", dir, "--list", "news"); code != ExitOK {
		t.Fatalf("init: %d %s", code, errOut)
	}
	code, out, _ := run(t, nil, "", "check", dir, "--offline")
	if code != ExitOK || !strings.Contains(out, "0 errors, 0 warnings") {
		t.Fatalf("check after init: %d\n%s", code, out)
	}
}

func TestSavedName(t *testing.T) {
	for in, want := range map[string]string{".zshrc": "zshrc", "../../evil.txt": "evil.txt", "/tmp/x/abs.txt": "abs.txt", `..\..\win.bat`: "win.bat", "": "attachment-7-0", "...": "attachment-7-0", "Überweisung.pdf": "Überweisung.pdf"} {
		if got := savedName(in, 7, 0); got != want {
			t.Errorf("savedName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The root SKILL.md, the README and the operations guide name only commands
// and flags that exist: they are the first thing a new agent reads.
func TestDocsNameRealCommands(t *testing.T) {
	cmdRE := regexp.MustCompile("`(?:sudo )?lfr ([a-z][a-z ]*[a-z])((?: [^`]*)?)`|\\n(?:sudo )?lfr ([a-z][a-z ]*[a-z])([^\\n#]*)")
	flagRE := regexp.MustCompile(` --([a-z][a-z-]+)`)
	for _, file := range []string{"../../SKILL.md", "../../README.md", "../../docs/operations.md"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		checked := 0
		for _, m := range cmdRE.FindAllStringSubmatch(string(b), -1) {
			name, rest := m[1]+m[3], m[2]+m[4]
			words := strings.Fields(name + rest)
			cmd, _ := find(words)
			if cmd == nil {
				// A sentence ("lfr runs on one server") is prose, not a command.
				if word := strings.Fields(name)[0]; group(word) == "" && wordSummary(word) == "" {
					continue
				}
				t.Errorf("%s names `lfr %s`, which does not exist", file, strings.TrimSpace(name+rest))
				continue
			}
			checked++
			known := map[string]bool{"json": true, "data": true, "remote": true}
			for _, f := range cmd.flags {
				known[f.name] = true
			}
			for _, f := range flagRE.FindAllStringSubmatch(rest, -1) {
				if !known[f[1]] {
					t.Errorf("%s uses `lfr %s --%s`, which has no such flag", file, cmd.path, f[1])
				}
			}
		}
		if checked < 5 {
			t.Errorf("%s: only %d commands found; the pattern is broken", file, checked)
		}
	}
}
