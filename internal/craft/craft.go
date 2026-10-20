// Package craft is lfr's own research on newsletter craft, embedded in the
// binary: verbatim voices of readers and writers, peer newsletters, and
// evidence from standards, studies, benchmarks and guides. Projects cite it
// as lfr:V012, lfr:P003 or lfr:E031 next to their own research; lfr
// evidence prints it by dimension; rules and skills point into it.
package craft

import (
	"embed"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/research"
)

// Prefix marks citations of the built-in research.
const Prefix = "lfr:"

//go:embed research/*.md
var files embed.FS

var (
	once   sync.Once
	report research.Report
)

// Report is the parsed built-in research.
func Report() research.Report {
	once.Do(func() {
		m := map[string][]byte{}
		for _, n := range []string{research.VoicesFile, research.PeersFile, research.EvidenceFile} {
			if b, err := files.ReadFile("research/" + n); err == nil {
				m[n] = b
			}
		}
		report = research.Check(m, "research", time.Now())
	})
	return report
}

// IDs returns the citable built-in IDs, with the lfr: prefix.
func IDs() map[string]bool {
	ids := map[string]bool{}
	for id := range Report().IDs() {
		ids[Prefix+id] = true
	}
	return ids
}

// Evidence returns the evidence that applies to a dimension (all for "").
func Evidence(dimension string) []research.Evidence {
	r := Report()
	if r.Evidences == nil {
		return nil
	}
	var out []research.Evidence
	for _, e := range r.Evidences.Entries {
		if dimension == "" || slices.Contains(e.Applies, dimension) {
			out = append(out, e)
		}
	}
	return out
}

// Voice returns one built-in voice by number.
func Voice(n int) (research.Voice, bool) {
	r := Report()
	if r.Voices != nil {
		for _, v := range r.Voices.Entries {
			if v.ID == n {
				return v, true
			}
		}
	}
	return research.Voice{}, false
}

// Lookup renders one built-in item ("lfr:E031", "E031", "V012", "P003") as
// Markdown, verbatim.
func Lookup(id string) (string, bool) {
	id = strings.ToUpper(strings.TrimPrefix(strings.ToLower(id), Prefix))
	if len(id) < 2 {
		return "", false
	}
	var n int
	if _, err := fmt.Sscanf(id[1:], "%d", &n); err != nil {
		return "", false
	}
	r := Report()
	switch id[0] {
	case 'E':
		for _, e := range Evidence("") {
			if e.ID == n {
				return FormatEvidence(e), true
			}
		}
	case 'V':
		if v, ok := Voice(n); ok {
			return fmt.Sprintf("%sV%03d · %s (%s) · %s\n%s\n<%s>\n", Prefix, v.ID, v.Platform, v.Place, v.Author, research.Blockquote(v.Quote), v.URL), true
		}
	case 'P':
		if r.Peers != nil {
			for _, p := range r.Peers.Entries {
				if p.ID == n {
					var b strings.Builder
					fmt.Fprintf(&b, "%sP%03d · %s · %s\n", Prefix, p.ID, p.Name, p.URL)
					for _, k := range research.PeerFields {
						fmt.Fprintf(&b, "- %s: %s\n", k, p.Facts[k])
					}
					return b.String(), true
				}
			}
		}
	}
	return "", false
}

// FormatEvidence renders one evidence item.
func FormatEvidence(e research.Evidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%sE%03d · %s (%s) · %s\n%s\n<%s>\n", Prefix, e.ID, e.Publisher, e.Work, e.Kind, research.Blockquote(e.Quote), e.URL)
	if e.Observer != "" {
		fmt.Fprintf(&b, "Observer note: %s\n", e.Observer)
	}
	return b.String()
}
