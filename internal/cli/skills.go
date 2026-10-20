package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfr/internal/project"
	"github.com/Anonyfox/lfr/internal/skills"
)

func skillData(c *call) skills.Data {
	return skills.Data{Version: c.env.Build.Version, Project: project.Facts()}
}

func runSkills(c *call) error {
	data := skillData(c)
	if len(c.args) == 0 {
		all, err := skills.All(data)
		if err != nil {
			return err
		}
		return c.result(all, func() {
			for _, s := range all {
				c.printf("%-10s %s\n", s.Topic, gist(s.Description))
			}
			c.next("lfr skills <topic> prints one; lfr skills install puts all of them where your agent loads skills")
		})
	}
	if len(c.args) > 1 {
		return usage("one topic at a time")
	}
	s, err := skills.Get(c.args[0], data)
	if err != nil {
		return usage("%v", err)
	}
	return c.result(map[string]string{"name": s.Name, "text": s.Text}, func() { c.printf("%s", s.Text) })
}

func runSkillsInstall(c *call) error {
	data := skillData(c)
	base := "."
	if len(c.args) == 1 {
		base = c.args[0]
	} else if len(c.args) > 1 {
		return usage("one directory at most")
	}
	if c.has("global") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		base = home
	}
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return fmt.Errorf("%s: no such directory", base)
	}
	var folders []skills.Folder
	switch {
	case c.has("agents") && c.has("claude"):
		folders = []skills.Folder{{Rel: skills.AgentsDir, Why: "as asked"}, {Rel: skills.ClaudeDir, Why: "as asked"}}
	case c.has("agents"):
		folders = []skills.Folder{{Rel: skills.AgentsDir, Why: "as asked"}}
	case c.has("claude"):
		folders = []skills.Folder{{Rel: skills.ClaudeDir, Why: "as asked"}}
	default:
		folders = skills.Choose(base, data, c.env.getenv)
	}
	type result struct {
		Folder  string          `json:"folder"`
		Changes []skills.Change `json:"changes"`
	}
	var results []result
	for _, f := range folders {
		changes, err := skills.Install(filepath.Join(base, f.Rel), data, c.has("force"))
		if err != nil {
			return err
		}
		results = append(results, result{filepath.Join(base, f.Rel), changes})
	}
	return c.result(results, func() {
		for i, r := range results {
			c.printf("%s (for %s, because %s)\n", r.Folder, skills.Readers[folders[i].Rel], folders[i].Why)
			for _, ch := range r.Changes {
				line := fmt.Sprintf("  %-9s %s", ch.Action, ch.Name)
				if ch.Reason != "" {
					line += "  (" + ch.Reason + ")"
				}
				c.printf("%s\n", line)
			}
		}
		c.next("lfr check in a newsletter project names the skill for each step")
	})
}

// gist is what a skill does, without the when and when-not: the
// description up to its first " - " or sentence end.
func gist(description string) string {
	d := description
	if i := strings.Index(d, " - "); i > 0 {
		d = d[:i]
	} else if i := strings.Index(d, ". "); i > 0 {
		d = d[:i]
	}
	return strings.TrimSuffix(d, ".")
}
