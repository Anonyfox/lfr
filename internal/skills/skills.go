// Package skills holds the Agent Skills (agentskills.io) that teach an agent
// to run lfr: setting up the server, researching and conceiving a
// newsletter, launching it, writing and sending each issue, learning from
// the numbers, and handling mailboxes. Skills are embedded in the binary and
// rendered with its facts, so a printed skill always matches the lfr that
// printed it.
package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"text/template"
)

//go:embed files
var files embed.FS

// Data is what a skill template refers to, with [[ ]] delimiters.
type Data struct {
	Version string
	// Newsletter project layout and minimums, filled by the project package.
	Project map[string]any
}

// Skill is one rendered skill.
type Skill struct {
	Topic       string `json:"topic"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Text        string `json:"-"`
}

// Newsletter are the skills a newsletter project needs: its own steps, and
// the mailbox skill for reading readers' replies. Server setup belongs to
// the operator's own skills (lfr skills install).
var Newsletter = []string{"mailbox", "research", "concept", "launch", "issue", "learn"}

// lifecycle orders the skills as a newsletter uses them.
var lifecycle = []string{"setup", "mailbox", "research", "concept", "launch", "issue", "learn"}

func rank(topic string) int {
	if i := slices.Index(lifecycle, topic); i >= 0 {
		return i
	}
	return len(lifecycle)
}

// Topics lists every skill's topic in lifecycle order.
func Topics() []string {
	found, _ := fs.Glob(files, "files/*/SKILL.md")
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, path.Base(path.Dir(f)))
	}
	slices.SortStableFunc(out, func(a, b string) int {
		if d := rank(a) - rank(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	return out
}

// All renders every skill in lifecycle order.
func All(data Data) ([]Skill, error) {
	var all []Skill
	for _, t := range Topics() {
		s, err := render(t, data)
		if err != nil {
			return nil, err
		}
		all = append(all, s)
	}
	sort.SliceStable(all, func(i, j int) bool { return rank(all[i].Topic) < rank(all[j].Topic) })
	return all, nil
}

// Get renders the skill of a topic (or full name, "lfr-issue").
func Get(topic string, data Data) (Skill, error) {
	topic = strings.TrimPrefix(topic, "lfr-")
	if strings.ContainsAny(topic, "/.") || !slices.Contains(Topics(), topic) {
		return Skill{}, fmt.Errorf("no skill %q; skills are %s", topic, strings.Join(Topics(), ", "))
	}
	return render(topic, data)
}

func render(topic string, data Data) (Skill, error) {
	source, err := files.ReadFile("files/" + topic + "/SKILL.md")
	if err != nil {
		return Skill{}, err
	}
	t, err := template.New(topic).Delims("[[", "]]").Option("missingkey=error").Parse(string(source))
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	s := Skill{Topic: topic}
	s.Name, s.Description, err = frontmatter(out.String())
	if err != nil {
		return Skill{}, fmt.Errorf("skill %s: %w", topic, err)
	}
	s.Text = stamp(out.String(), data.Version)
	return s, nil
}

// frontmatter reads name and description from the YAML header (single lines).
func frontmatter(text string) (name, description string, err error) {
	rest, ok := strings.CutPrefix(text, "---\n")
	header, _, closed := strings.Cut(rest, "\n---\n")
	if !ok || !closed {
		return "", "", fmt.Errorf("missing --- frontmatter")
	}
	for _, line := range strings.Split(header, "\n") {
		if v, ok := strings.CutPrefix(line, "name: "); ok {
			name = v
		}
		if v, ok := strings.CutPrefix(line, "description: "); ok {
			description = v
		}
	}
	if name == "" || description == "" {
		return "", "", fmt.Errorf("frontmatter needs name and description")
	}
	return name, description, nil
}
