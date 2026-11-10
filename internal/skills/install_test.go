package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/project"
	"github.com/Anonyfox/lfr/internal/skills"
)

func TestInstallPreservesEditsAndUpgradesUntouchedCopies(t *testing.T) {
	for _, tc := range []struct {
		name, edit string
		force      bool
		action     string
		state      skills.State
	}{
		{"unchanged", "", false, "unchanged", skills.Current},
		{"upgrade", "", false, "updated", skills.Current},
		{"edited", "\nPersonal instructions.\n", false, "kept", skills.Modified},
		{"force", "\nPersonal instructions.\n", true, "replaced", skills.Current},
		{"foreign", "foreign", false, "kept", skills.Foreign},
		{"replace foreign", "foreign", true, "replaced", skills.Current},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			data := skills.Data{Version: "old", Project: project.Facts()}
			data.Version = "next"
			changes, err := skills.Install(dir, data, false, "issue")
			if err != nil || len(changes) != 1 || changes[0].Action != "created" {
				t.Fatalf("install: %+v %v", changes, err)
			}
			file := changes[0].Path
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if tc.edit != "" {
				b := append(original, tc.edit...)
				if tc.edit == "foreign" {
					b = []byte("my own instructions")
				}
				if err := os.WriteFile(file, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.name != "unchanged" {
				data.Version = "new"
			}
			changes, err = skills.Install(dir, data, tc.force, "issue")
			if err != nil || len(changes) != 1 || changes[0].Action != tc.action {
				t.Fatalf("reinstall: %+v %v", changes, err)
			}
			copies, err := skills.Scan(dir, data)
			if err != nil || len(copies) != 1 || copies[0].State != tc.state {
				t.Fatalf("scan: %+v %v", copies, err)
			}
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if tc.action == "kept" && !strings.Contains(string(b), map[bool]string{true: "my own instructions", false: "Personal instructions"}[tc.edit == "foreign"]) {
				t.Fatalf("lost edits: %s", b)
			}
			if tc.state == skills.Current {
				s, err := skills.Get("lfr-issue", data)
				if err != nil || string(b) != s.Text {
					t.Fatalf("installed text differs: %v", err)
				}
			}
		})
	}
}

func TestSkillDiscoveryAndObsoleteCopies(t *testing.T) {
	data := skills.Data{Version: "test", Project: project.Facts()}
	base := t.TempDir()
	noenv := func(string) string { return "" }
	if got := skills.Choose(base, data, noenv); len(got) != 1 || got[0].Rel != skills.AgentsDir {
		t.Fatalf("default: %+v", got)
	}
	if got := skills.Choose(base, data, func(string) string { return "1" }); got[0].Rel != skills.ClaudeDir {
		t.Fatalf("Claude: %+v", got)
	}
	dir := filepath.Join(base, skills.ClaudeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if got := skills.Choose(base, data, noenv); got[0].Rel != skills.ClaudeDir {
		t.Fatalf("existing folder: %+v", got)
	}
	if _, err := skills.Install(dir, data, false, "issue", "learn"); err != nil {
		t.Fatal(err)
	}
	if got := skills.Choose(base, data, noenv); got[0].Why != "it already holds lfr skills" {
		t.Fatalf("existing skills: %+v", got)
	}
	child := filepath.Join(base, "nested", "project")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	copies := skills.Find(child, data)
	if len(copies) != 2 {
		t.Fatalf("ancestor discovery: %+v", copies)
	}
	data.Version = "next"
	changes, err := skills.Install(dir, data, false, "issue")
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	for _, c := range changes {
		if c.Name == "lfr-learn" && c.Action == "removed" {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("obsolete copy retained: %+v", changes)
	}
	for _, topic := range []string{"../issue", "unknown", "issue.md"} {
		if _, err := skills.Get(topic, data); err == nil {
			t.Errorf("accepted %q", topic)
		}
	}
	if _, err := skills.All(skills.Data{}); err == nil {
		t.Fatal("missing template facts accepted")
	}
}
