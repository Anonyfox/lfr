package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"
)

// Skill folders, relative to a project or home directory. Codex, Cursor,
// OpenCode, Gemini CLI and GitHub Copilot read .agents/skills; Claude Code
// reads .claude/skills.
const (
	AgentsDir = ".agents/skills"
	ClaudeDir = ".claude/skills"
)

// Every printed or installed skill carries the version that rendered it and a
// digest of its own text, so an install can tell an untouched copy (safe to
// replace) from one somebody edited.
const (
	versionKey = "lfr-version"
	digestKey  = "lfr-sha256"
)

var (
	digestLine  = regexp.MustCompile(`(?m)^  ` + digestKey + `: "([0-9a-f]*)"$`)
	versionLine = regexp.MustCompile(`(?m)^  ` + versionKey + `: "([^"]*)"$`)
)

func stamp(text, version string) string {
	head, body, _ := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	unsigned := fmt.Sprintf("---\n%s\nmetadata:\n  %s: %q\n  %s: \"\"\n---\n%s", head, versionKey, version, digestKey, body)
	return strings.Replace(unsigned, digestKey+`: ""`, digestKey+`: "`+digest(unsigned)+`"`, 1)
}

func digest(unsigned string) string {
	sum := sha256.Sum256([]byte(unsigned))
	return hex.EncodeToString(sum[:])
}

// State says how an installed copy relates to this binary.
type State string

const (
	Current  State = "current"  // identical to what this binary prints
	Outdated State = "outdated" // untouched, printed by another lfr version
	Modified State = "modified" // changed after it was written
	Foreign  State = "foreign"  // not written by lfr
)

// Copy is one lfr skill found on disk.
type Copy struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	State   State  `json:"state"`
}

func inspect(text, want string) (version string, state State) {
	if m := versionLine.FindStringSubmatch(text); m != nil {
		version = m[1]
	}
	m := digestLine.FindStringSubmatch(text)
	switch {
	case m == nil:
		return version, Foreign
	case text == want:
		return version, Current
	case digest(strings.Replace(text, m[0], "  "+digestKey+`: ""`, 1)) != m[1]:
		return version, Modified
	}
	return version, Outdated
}

func byName(data Data) (map[string]Skill, error) {
	all, err := All(data)
	if err != nil {
		return nil, err
	}
	m := make(map[string]Skill, len(all))
	for _, s := range all {
		m[s.Name] = s
	}
	return m, nil
}

// Scan lists the lfr skills in one skills folder.
func Scan(dir string, data Data) ([]Copy, error) {
	current, err := byName(data)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var copies []Copy
	for _, entry := range entries {
		if entry.Name() != "lfr" && !strings.HasPrefix(entry.Name(), "lfr-") {
			continue
		}
		file := filepath.Join(dir, entry.Name(), "SKILL.md")
		raw, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c := Copy{Name: entry.Name(), Path: file}
		c.Version, c.State = inspect(string(raw), current[entry.Name()].Text)
		copies = append(copies, c)
	}
	return copies, nil
}

// Find lists the lfr skills an agent working in dir may load: the skill
// folders of dir and every directory above it, including the home directory.
func Find(dir string, data Data) []Copy {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var all []Copy
	for {
		for _, rel := range []string{AgentsDir, ClaudeDir} {
			folder := filepath.Join(abs, rel)
			real, err := filepath.EvalSymlinks(folder)
			if err != nil || seen[real] {
				continue
			}
			seen[real] = true
			copies, _ := Scan(folder, data)
			all = append(all, copies...)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return all
		}
		abs = parent
	}
}

// Change is what Install did with one skill.
type Change struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Action string `json:"action"` // created, updated, unchanged, kept, replaced, removed
	Reason string `json:"reason,omitempty"`
}

// Install writes every skill into dir as <name>/SKILL.md. Copies somebody
// edited, and SKILL.md files lfr did not write, are kept unless force is
// set; untouched copies of skills this binary no longer has are removed.
func Install(dir string, data Data, force bool, topics ...string) ([]Change, error) {
	current, err := byName(data)
	if err != nil {
		return nil, err
	}
	if len(topics) > 0 {
		for name, s := range current {
			if !slices.Contains(topics, s.Topic) {
				delete(current, name)
			}
		}
	}
	existing, err := Scan(dir, data)
	if err != nil {
		return nil, err
	}
	found := map[string]Copy{}
	for _, c := range existing {
		found[c.Name] = c
	}
	names := make([]string, 0, len(current))
	for n := range current {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return rank(current[names[i]].Topic) < rank(current[names[j]].Topic) })
	var changes []Change
	for _, name := range names {
		s := current[name]
		file := filepath.Join(dir, name, "SKILL.md")
		ch := Change{Name: name, Path: file}
		c, exists := found[name]
		switch {
		case !exists:
			ch.Action = "created"
		case c.State == Current:
			ch.Action = "unchanged"
		case c.State == Outdated:
			ch.Action = "updated"
			if c.Version != data.Version {
				ch.Reason = "was lfr " + c.Version
			}
		case force:
			ch.Action, ch.Reason = "replaced", describe(c.State)
		default:
			ch.Action, ch.Reason = "kept", describe(c.State)+"; --force replaces it"
		}
		if ch.Action == "created" || ch.Action == "updated" || ch.Action == "replaced" {
			if err := write(file, s.Text); err != nil {
				return changes, err
			}
		}
		changes = append(changes, ch)
	}
	for _, c := range existing {
		if _, ok := current[c.Name]; ok || c.State != Outdated {
			continue
		}
		if err := os.Remove(c.Path); err != nil {
			return changes, err
		}
		os.Remove(filepath.Dir(c.Path))
		changes = append(changes, Change{Name: c.Name, Path: c.Path, Action: "removed", Reason: "no longer part of lfr"})
	}
	return changes, nil
}

func describe(s State) string {
	if s == Foreign {
		return "not written by lfr"
	}
	return "changed by someone"
}

func write(file, text string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".SKILL.md.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// Folder is a skills folder chosen for installing, with the reason.
type Folder struct {
	Rel string
	Why string
}

// Readers names the agents that read a folder.
var Readers = map[string]string{
	AgentsDir: "Codex, Cursor, OpenCode, Gemini CLI and GitHub Copilot",
	ClaudeDir: "Claude Code",
}

// Choose picks where to install below base: folders that already hold lfr
// skills, else the running agent's folder (Claude Code sets CLAUDECODE), else
// existing agent folders, else .agents/skills.
func Choose(base string, data Data, getenv func(string) string) []Folder {
	var out []Folder
	for _, rel := range []string{AgentsDir, ClaudeDir} {
		if copies, _ := Scan(filepath.Join(base, rel), data); len(copies) > 0 {
			out = append(out, Folder{rel, "it already holds lfr skills"})
		}
	}
	if len(out) > 0 {
		return out
	}
	if getenv("CLAUDECODE") != "" {
		return []Folder{{ClaudeDir, "Claude Code is running"}}
	}
	for _, rel := range []string{ClaudeDir, AgentsDir} {
		if info, err := os.Stat(filepath.Join(base, rel)); err == nil && info.IsDir() {
			return []Folder{{rel, "it exists"}}
		}
	}
	return []Folder{{AgentsDir, "the shared default"}}
}
