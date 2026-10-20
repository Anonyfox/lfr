package research

import (
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

const voices = `# Voices: Bakers Weekly

## Batch 2026-10-20 · from zero · queries: "sourdough newsletter" · unreachable: Reddit (blocked)

### V001 · Hacker News (Ask HN: newsletters) · reader · posted 2024-05 · fetched 2026-10-20 · via fetch
> I only keep the ones that are short.
>
> And plain.
<https://news.ycombinator.com/item?id=1>
Observer note: 41 points

### V002 · Lemmy (c/baking) · writer · posted unknown · fetched 2026-10-20 · via fetch
> Weekly is all I can sustain.
<https://lemmy.world/comment/2>
`

func TestParseVoices(t *testing.T) {
	v, ps := ParseVoices(voices)
	if len(ps) != 0 || len(v.Entries) != 2 || v.Entries[0].Quote != "I only keep the ones that are short.\n\nAnd plain." || v.Entries[0].Observer != "41 points" || v.Entries[1].Author != "writer" {
		t.Fatalf("%+v %+v", v, ps)
	}
	bad := strings.Replace(voices, "· reader ·", "· buyer ·", 1)
	if _, ps := ParseVoices(bad); len(ps) != 1 {
		t.Fatalf("bad author: %+v", ps)
	}
}

const peers = `# Peers: Bakers Weekly

## Batch 2026-10-20 · from zero

### P001 · The Bread Letter · https://bread.example/archive · fetched 2026-10-20 · via fetch
- Cadence: weekly, Fridays
- Length: 600 / 800 / 700
- Structure: note, one recipe, three links
- Subject: "Crumb shots"
- Opening: "Hi bakers,"
- Voice: first person, warm
- Visual: light HTML, one photo
- Links: 3 to 5
- Signup: promise and sample issue, email only
- Takeaway: a recipe each week

### Grid after this batch

| Column | Value | Peers |
| --- | --- | --- |
| Cadence | weekly | 1 |
| Length | under 1,000 words | 1 |
| Structure | note plus feature plus links | 1 |
| Subject | short phrase | 1 |
| Opening | greeting | 1 |
| Voice | personal | 1 |
| Visual | light HTML | 1 |
| Links | a few | 1 |
| Signup | promise and sample | 1 |
| Takeaway | recipe | 1 |

### Baseline

Weekly, short, personal.

### Gaps

- No video.
`

func TestCheckPeersAndBalance(t *testing.T) {
	r := Check(map[string][]byte{VoicesFile: []byte(voices), PeersFile: []byte(peers)}, "research", today)
	var rules []string
	for _, f := range r.Findings {
		rules = append(rules, f.Rule+": "+f.Message)
	}
	if len(r.Findings) != 2 || r.Complete { // too few voices and peers
		t.Fatalf("%v", rules)
	}
	if r.Summary.Voices != 2 || r.Summary.Peers != 1 || r.Summary.Platforms != 2 || !r.IDs()["P001"] {
		t.Fatalf("%+v", r.Summary)
	}
	broken := strings.Replace(peers, "| Cadence | weekly | 1 |", "| Cadence | weekly | 2 |", 1)
	r = Check(map[string][]byte{PeersFile: []byte(broken)}, "research", today)
	found := false
	for _, f := range r.Findings {
		if strings.Contains(f.Message, "Grid counts 2 peers for Cadence") {
			found = true
		}
	}
	if !found {
		t.Fatal("grid sum not checked")
	}
}

func TestEvidence(t *testing.T) {
	src := `# Evidence: newsletter craft

## Batch 2026-10-20 · from zero · queries: "sender guidelines" · unreachable: none

### E001 · Google (Email sender guidelines) · standard · published 2024-02 · fetched 2026-10-20 · via fetch
> Keep spam rates reported in Postmaster Tools below 0.3%.
<https://support.google.com/a/answer/81126>
Applies: deliverability, hygiene
`
	e, ps := ParseEvidence(src)
	if len(ps) != 0 || len(e.Entries) != 1 || e.Entries[0].Publisher != "Google" || e.Entries[0].Work != "Email sender guidelines" || len(e.Entries[0].Applies) != 2 {
		t.Fatalf("%+v %+v", e, ps)
	}
	if _, ps := ParseEvidence(strings.Replace(src, "hygiene", "vibes", 1)); len(ps) != 1 {
		t.Fatal("unknown dimension accepted")
	}
}

func FuzzParse(f *testing.F) {
	f.Add(voices)
	f.Add(peers)
	f.Fuzz(func(t *testing.T, s string) {
		ParseVoices(s)
		ParsePeers(s)
		ParseEvidence(s)
		Check(map[string][]byte{VoicesFile: []byte(s), PeersFile: []byte(s), EvidenceFile: []byte(s)}, "r", today)
	})
}

func TestPeerBatchHeadings(t *testing.T) {
	for _, h := range []string{
		"## Batch 2026-10-20 · from zero",
		"## Batch 2026-10-20 · from zero · shortfall: a small niche, 3 of 8 peers",
		`## Batch 2026-10-20 · from zero · queries: "archives" · unreachable: none`,
		`## Batch 2026-10-20 · from zero · queries: "archives" · unreachable: none · shortfall: small niche`,
	} {
		if _, p := parseBatch(h, 1, false); p != nil {
			t.Errorf("%q: %s", h, p.Message)
		}
	}
	if _, p := parseBatch("## Batch 2026-10-20 · from zero · whatever", 1, false); p == nil {
		t.Error("an unknown field passed")
	}
}
