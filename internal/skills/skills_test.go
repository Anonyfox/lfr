package skills

import (
	"os"
	"testing"
)

// lfr.email serves SKILL.md and install.sh from site/assets/agents; they must
// be the repository's own files, byte for byte.
func TestSiteServesTheRepositoryAgentFiles(t *testing.T) {
	for _, name := range []string{"SKILL.md", "install.sh"} {
		want, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile("../../site/assets/agents/" + name)
		if err != nil || string(got) != string(want) {
			t.Errorf("site/assets/agents/%s differs from %s; copy it over", name, name)
		}
	}
}
