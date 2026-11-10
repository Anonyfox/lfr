package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectAndResearchCommands(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newsletter")
	for _, s := range []struct {
		name, stdin, want string
		args              []string
		exit              int
	}{
		{"scaffold", "", "Newsletter project", []string{"init", dir, "--list", "news", "--owner", "owner@example.com"}, 0},
		{"offline advice", "", "Next (research)", []string{"check", dir, "--offline"}, 0},
		{"machine advice", "", `"phase": "research"`, []string{"check", dir, "--offline", "--json"}, 0},
		{"unavailable server", "", "no lfr server runs here", []string{"check", dir, "--data", filepath.Join(dir, "absent")}, 0},
		{"install both", "", "created", []string{"skills", "install", dir, "--agents", "--claude"}, 0},
		{"idempotent install", "", "unchanged", []string{"skills", "install", dir}, 0},
		{"research dimensions", "", "lfr's research", []string{"evidence"}, 0},
		{"verbatim source", "", "E012", []string{"evidence", "lfr:E012"}, 0},
		{"dimension", "", "", []string{"evidence", "subject"}, 0},
		{"rules overview", "", "concept.md", []string{"rules"}, 0},
		{"one rule", "", "", []string{"rules", "list.cadence"}, 0},
		{"unknown rule", "", "no rule", []string{"rules", "missing"}, ExitUsage},
		{"unknown evidence", "", "neither a dimension", []string{"evidence", "missing"}, ExitUsage},
		{"unknown skill", "", "no skill", []string{"skills", "missing"}, ExitUsage},
		{"missing concept", "", "no concept.md", []string{"push", dir}, ExitUsage},
	} {
		t.Run(s.name, func(t *testing.T) {
			code, out, errs := run(t, nil, s.stdin, s.args...)
			if code != s.exit || !strings.Contains(out+errs, s.want) {
				t.Fatalf("%v: %d\n%s%s", s.args, code, out, errs)
			}
			if slicesJSON(s.args) && !json.Valid([]byte(out)) {
				t.Fatal(out)
			}
		})
	}
	// An edited installed skill must survive the CLI's default reinstall.
	file := filepath.Join(dir, ".agents", "skills", "lfr-issue", "SKILL.md")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, "\nLocal editorial instruction.\n"...)
	if err := os.WriteFile(file, b, 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, nil, "", "skills", "install", dir, "--agents")
	if code != 0 || !strings.Contains(out, "kept") {
		t.Fatalf("%d %s%s", code, out, errs)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != string(b) {
		t.Fatal("edited instructions overwritten")
	}
	code, out, errs = run(t, nil, "", "skills", "install", dir, "--agents", "--force")
	if code != 0 || !strings.Contains(out, "replaced") {
		t.Fatalf("%d %s%s", code, out, errs)
	}
}

func TestPushPublishesProjectPromiseWelcomeAndSite(t *testing.T) {
	_, env := serve(t)
	dir := filepath.Join(t.TempDir(), "bakers")
	must := func(stdin, want string, args ...string) string {
		t.Helper()
		code, out, errs := run(t, env, stdin, args...)
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("%v: %d %s%s", args, code, out, errs)
		}
		return out
	}
	must("", "", "domain", "add", "example.com")
	must("", "", "list", "add", "bakers", "--from", "bakers@example.com", "--name", "Bakers", "--address", "Weg 1, Berlin")
	must("", "", "init", dir, "--list", "bakers", "--owner", "owner@example.org")
	brief, err := os.ReadFile("testdata/concept.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "concept.md"), brief, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "welcome.md"), []byte("# Welcome\n\nHere is your first recipe."), 0600); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(dir, "site")
	must("", "", "site", "bakers", site, "--eject")
	must("", "welcome issue", "push", dir)
	out := must("", "Every Friday morning", "list", "show", "bakers", "--json")
	if !strings.Contains(out, "Better sourdough by Friday") {
		t.Fatal("signup promise not copied")
	}
	must("", "welcome", "issue", "ls", "bakers")
	must("", "", "check", dir)
	// The current project resolves bare issue names and gets its owner's test address.
	t.Chdir(dir)
	must("# Garden\n\nA recipe.", "bakers/garden", "issue", "put", "-", "--slug", "garden")
	must("", "Garden", "issue", "ls")
	must("", "Garden", "issue", "show", "garden", "--text")
	must("", "", "check", "--json")
	must("", "Test copies queued", "issue", "test", "garden", "owner@example.org")
}
