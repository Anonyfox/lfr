package craft

import (
	"strings"
	"testing"

	"github.com/Anonyfox/lfr/internal/research"
)

// TestResearch holds lfr's own research to the format every project's
// research is held to.
func TestResearch(t *testing.T) {
	r := Report()
	for _, f := range r.Findings {
		if f.Rule == research.RuleBalance && r.Summary.Peers == 0 {
			continue // peers are merged separately
		}
		t.Errorf("%s:%d %s: %s", f.File, f.Line, f.Rule, f.Message)
	}
	if r.Summary.Voices < 100 || r.Summary.Evidence < 100 {
		t.Fatalf("%+v", r.Summary)
	}
	for _, d := range research.Dimensions {
		if d != "legal" && len(Evidence(d)) == 0 {
			t.Errorf("no evidence for %s", d)
		}
	}
}

func TestLookup(t *testing.T) {
	if !IDs()["lfr:E001"] || IDs()["E001"] {
		t.Fatal("ids")
	}
	e, ok := Lookup("lfr:E001")
	if !ok || !strings.HasPrefix(e, "lfr:E001 · ") || !strings.Contains(e, "\n> ") {
		t.Fatal(e)
	}
	if v, ok := Lookup("v001"); !ok || !strings.Contains(v, "lfr:V001") {
		t.Fatal(v)
	}
	if _, ok := Lookup("E9999"); ok {
		t.Fatal("found missing")
	}
}
