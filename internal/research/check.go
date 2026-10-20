package research

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Anonyfox/lfr/internal/finding"
)

// Rule IDs.
const (
	RuleFormat    = "research.format"
	RuleSubstance = "research.substance"
	RuleBalance   = "research.balance"
)

// Rules describes the research checks.
func Rules() []finding.Rule {
	return []finding.Rule{
		{ID: RuleFormat, Summary: "Research files follow the exact format of the lfr-research skill",
			Help: "Every heading, field and ID in research/voices.md, research/peers.md and research/evidence.md is parsed. A malformed entry cannot be cited reliably and usually means text was paraphrased or merged. Fix the named line to match the format in lfr skills research; quote lines always start with \">\"."},
		{ID: RuleSubstance, Summary: "Research entries are plausible: real dates, no duplicates, no placeholders",
			Help: "Flags dates in the future or out of order, sources fetched outside their batch, the same quote or peer twice, leftover template placeholders and a peer grid that no longer adds up. These are the traces of copied, invented or stale entries."},
		{ID: RuleBalance, Summary: "Research from zero meets its minimums and no single source dominates",
			Help: fmt.Sprintf("A newsletter concept needs %d voices from %d platforms and %d peer newsletters, no platform above 40%% of the voices and no single day holding most of them. A narrow sample makes every editorial decision rest on one corner of the audience. Collect more, or name the shortfall in the batch heading when the niche is truly that small.", MinVoices, MinPlatforms, MinPeers)},
	}
}

// Summary describes the research a project stands on.
type Summary struct {
	Voices    int `json:"voices"`
	Platforms int `json:"platforms"`
	Peers     int `json:"peers"`
	Evidence  int `json:"evidence"`
}

// Report is the result of checking the research directory.
type Report struct {
	Findings  []finding.Finding
	Summary   Summary
	Voices    *Voices
	Peers     *Peers
	Evidences *Evidences
	// Complete is true when the minimums hold (or a shortfall is named) and nothing is malformed.
	Complete bool
}

// IDs returns every citable ID ("V001", "P003", "E101") in the report.
func (r Report) IDs() map[string]bool {
	ids := map[string]bool{}
	if r.Voices != nil {
		for _, e := range r.Voices.Entries {
			ids[fmt.Sprintf("V%03d", e.ID)] = true
		}
	}
	if r.Peers != nil {
		for _, e := range r.Peers.Entries {
			ids[fmt.Sprintf("P%03d", e.ID)] = true
		}
	}
	if r.Evidences != nil {
		for _, e := range r.Evidences.Entries {
			ids[fmt.Sprintf("E%03d", e.ID)] = true
		}
	}
	return ids
}

// Check parses and checks the research files present in files (keyed by
// file name; absent files are skipped). dir prefixes names in findings.
func Check(files map[string][]byte, dir string, today time.Time) Report {
	var r Report
	report := func(file, rule string, line int, message, hint string) {
		r.Findings = append(r.Findings, finding.Finding{Rule: rule, Severity: finding.Warning, File: dir + "/" + file, Line: line, Message: message, Fix: hint})
	}
	if raw, ok := files[VoicesFile]; ok {
		v, problems := ParseVoices(string(raw))
		for _, p := range problems {
			report(VoicesFile, RuleFormat, p.Line, p.Message, p.Hint)
		}
		checkVoices(v, today, func(rule string, line int, m, h string) { report(VoicesFile, rule, line, m, h) })
		r.Voices = &v
	}
	if raw, ok := files[PeersFile]; ok {
		c, problems := ParsePeers(string(raw))
		for _, p := range problems {
			report(PeersFile, RuleFormat, p.Line, p.Message, p.Hint)
		}
		checkPeers(c, today, func(rule string, line int, m, h string) { report(PeersFile, rule, line, m, h) })
		r.Peers = &c
	}
	if raw, ok := files[EvidenceFile]; ok {
		e, problems := ParseEvidence(string(raw))
		for _, p := range problems {
			report(EvidenceFile, RuleFormat, p.Line, p.Message, p.Hint)
		}
		checkEvidence(e, today, func(rule string, line int, m, h string) { report(EvidenceFile, rule, line, m, h) })
		r.Evidences = &e
	}
	hasVoices := r.Voices != nil && len(r.Voices.Entries) > 0
	hasPeers := r.Peers != nil && len(r.Peers.Entries) > 0
	switch {
	case hasVoices && !hasPeers:
		report(PeersFile, RuleBalance, 1, "Voices are collected but no peer newsletter is analyzed yet.", "Analyze the newsletters your readers already get; the concept needs both files.")
	case hasPeers && !hasVoices:
		report(VoicesFile, RuleBalance, 1, "Peers are analyzed but no voice is collected yet.", "Collect verbatim voices of readers; the concept needs both files.")
	}
	if r.Voices != nil {
		r.Summary.Voices = len(r.Voices.Entries)
		platforms := map[string]bool{}
		for _, e := range r.Voices.Entries {
			platforms[strings.ToLower(e.Platform)] = true
		}
		r.Summary.Platforms = len(platforms)
	}
	if r.Peers != nil {
		r.Summary.Peers = len(r.Peers.Entries)
	}
	if r.Evidences != nil {
		r.Summary.Evidence = len(r.Evidences.Entries)
	}
	enough := r.Summary.Voices >= MinVoices && r.Summary.Platforms >= MinPlatforms && r.Summary.Peers >= MinPeers
	if !enough && hasVoices && hasPeers && (shortfall(r.Voices.Batches) || (r.Peers != nil && shortfall(r.Peers.Batches))) {
		enough = true
	}
	formatOK := true
	for _, f := range r.Findings {
		if f.Rule == RuleFormat {
			formatOK = false
		}
	}
	r.Complete = enough && formatOK
	return r
}

type reporter func(rule string, line int, message, hint string)

func checkSequence(ids, lines []int, prefix string, add reporter) {
	seen := map[int]bool{}
	for k, id := range ids {
		switch {
		case seen[id]:
			add(RuleFormat, lines[k], fmt.Sprintf("%s%03d is used twice.", prefix, id), "IDs are never reused; give the later entry the next free number.")
		case k > 0 && id != ids[k-1]+1 && !seen[ids[k-1]+1]:
			add(RuleFormat, lines[k], fmt.Sprintf("Expected %s%03d here, found %s%03d.", prefix, ids[k-1]+1, prefix, id), "IDs continue without gaps across batches and are never renumbered once cited.")
		}
		seen[id] = true
	}
}

func checkDates(posted, fetched, batch, today time.Time, line int, add reporter) {
	y, m, d := today.Date()
	limit := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	if !fetched.IsZero() && fetched.After(limit) {
		add(RuleSubstance, line, "Fetched date "+fetched.Format(time.DateOnly)+" is in the future.", "Use the day the source was opened.")
	}
	future := !posted.IsZero() && posted.After(limit)
	if future {
		add(RuleSubstance, line, "Posted date is in the future.", "Copy the date the source shows, or write unknown.")
	}
	if !future && !posted.IsZero() && !fetched.IsZero() && posted.After(fetched) {
		add(RuleSubstance, line, "Posted after it was fetched.", "One of the two dates is wrong; check the source.")
	}
	if !batch.IsZero() && !fetched.IsZero() && (fetched.Before(batch.AddDate(0, 0, -1)) || fetched.After(batch.AddDate(0, 0, 7))) {
		add(RuleSubstance, line, "Fetched "+fetched.Format(time.DateOnly)+", outside its batch of "+batch.Format(time.DateOnly)+".", "Entries are fetched during their batch; a stale date usually means the entry was copied.")
	}
}

var placeholders = []string{"<exact text>", "<platform>", "<place>", "<newsletter>", "<name>", "https://source.url", "https://peer.url", "<query>", "<trigger>", "<topic>"}

func placeholder(s string) string {
	for _, p := range placeholders {
		if strings.Contains(s, p) {
			return p
		}
	}
	return ""
}

func batchDate(batches []Batch, i int) time.Time {
	if i >= 0 && i < len(batches) {
		return batches[i].Date
	}
	return time.Time{}
}

func checkVoices(v Voices, today time.Time, add reporter) {
	ids, lines := make([]int, len(v.Entries)), make([]int, len(v.Entries))
	quotes := map[string]int{}
	for k, e := range v.Entries {
		ids[k], lines[k] = e.ID, e.Line
		checkDates(e.Posted, e.Fetched, batchDate(v.Batches, e.Batch), today, e.Line, add)
		if p := placeholder(e.Quote + " " + e.Platform + " " + e.Place + " " + e.URL); p != "" {
			add(RuleSubstance, e.Line, "Template placeholder "+p+" left in an entry.", "Replace it with the real text or source, or remove the entry.")
		}
		key := strings.Join(strings.Fields(strings.ToLower(e.Quote)), " ")
		if first, seen := quotes[key]; seen && key != "" {
			add(RuleSubstance, e.Line, fmt.Sprintf("Same quote as V%03d.", first), "Keep the original source once; reposts are not new voices.")
		} else {
			quotes[key] = e.ID
		}
	}
	checkSequence(ids, lines, "V", add)
	for _, b := range v.Batches {
		if p := placeholder(b.Queries + " " + b.Unreachable + " " + b.Trigger); p != "" {
			add(RuleSubstance, b.Line, "Template placeholder "+p+" left in a batch heading.", "Write the actual queries and unreachable sources.")
		}
	}
	if !fromZero(v.Batches) {
		return
	}
	platforms := map[string]int{}
	days := map[time.Time]int{}
	dated := 0
	for _, e := range v.Entries {
		platforms[strings.ToLower(e.Platform)]++
		if e.Precision == Day {
			days[e.Posted]++
			dated++
		}
	}
	total := len(v.Entries)
	if (total < MinVoices || len(platforms) < MinPlatforms) && !shortfall(v.Batches) {
		add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d voices from %d %s; research from zero needs %d from %d.", total, len(platforms), map[bool]string{true: "platform", false: "platforms"}[len(platforms) == 1], MinVoices, MinPlatforms), "Collect more, preferring platforms not yet covered, or add \"· shortfall: <what and why>\" to the batch heading.")
	}
	if total >= 10 {
		for _, name := range slices.Sorted(maps.Keys(platforms)) {
			if n := platforms[name]; n*5 > total*2 {
				add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d of %d voices (%d%%) come from %s.", n, total, 100*n/total, name), "Keep any one platform at or below 40% so one corner of the audience cannot dominate.")
			}
		}
	}
	if dated >= 10 {
		order := slices.Collect(maps.Keys(days))
		sort.Slice(order, func(i, j int) bool { return order[i].Before(order[j]) })
		for _, day := range order {
			if n := days[day]; n*10 > dated*3 {
				add(RuleBalance, v.Batches[0].Line, fmt.Sprintf("%d of %d dated voices were posted on %s.", n, dated, day.Format(time.DateOnly)), "One thread dominates; spread across sources and time.")
			}
		}
	}
}

func checkPeers(c Peers, today time.Time, add reporter) {
	ids, lines := make([]int, len(c.Entries)), make([]int, len(c.Entries))
	urls := map[string]int{}
	for k, p := range c.Entries {
		ids[k], lines[k] = p.ID, p.Line
		checkDates(time.Time{}, p.Fetched, batchDate(c.Batches, p.Batch), today, p.Line, add)
		var missing []string
		for _, f := range PeerFields {
			if p.Facts[f] == "" {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			add(RuleFormat, p.Line, fmt.Sprintf("P%03d lacks %s.", p.ID, strings.Join(missing, ", ")), "Record every fact on its own \"- Fact: value\" line; write none when the newsletter shows nothing.")
		}
		facts := ""
		for _, v := range p.Facts {
			facts += " " + v
		}
		if ph := placeholder(p.Name + " " + p.URL + facts); ph != "" {
			add(RuleSubstance, p.Line, "Template placeholder "+ph+" left in an entry.", "Replace it with what the newsletter shows.")
		}
		key := strings.TrimSuffix(strings.ToLower(p.URL), "/")
		if first, seen := urls[key]; seen {
			add(RuleSubstance, p.Line, fmt.Sprintf("Same page as P%03d.", first), "Analyze each newsletter once.")
		} else {
			urls[key] = p.ID
		}
	}
	checkSequence(ids, lines, "P", add)
	if fromZero(c.Batches) && len(c.Entries) < MinPeers && !shortfallPeers(c) {
		add(RuleBalance, c.Batches[0].Line, fmt.Sprintf("%d peers; research from zero needs %d.", len(c.Entries), MinPeers), "Add newsletters your readers already read or compare against.")
	}
	if len(c.Entries) == 0 {
		return
	}
	if len(c.Grid) == 0 {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without a grid.", "Add \"### Grid after this batch\": a | Column | Value | Peers | table with one row per value.")
	} else {
		sums := map[string]int{}
		for _, row := range c.Grid {
			sums[row.Column] += row.Peers
		}
		for _, f := range PeerFields {
			if sums[f] != len(c.Entries) {
				add(RuleSubstance, c.GridLine, fmt.Sprintf("Grid counts %d peers for %s; the file has %d.", sums[f], f, len(c.Entries)), "Recount the grid across all peers after every batch.")
			}
		}
	}
	if strings.TrimSpace(c.Baseline) == "" {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without a baseline.", "Add \"### Baseline\" with one paragraph naming the most frequent value per fact.")
	}
	if len(c.Gaps) == 0 {
		add(RuleFormat, c.Entries[len(c.Entries)-1].Line, "Peers without gaps.", "Add \"### Gaps\" listing values no peer uses.")
	}
}

func checkEvidence(v Evidences, today time.Time, add reporter) {
	ids, lines := make([]int, len(v.Entries)), make([]int, len(v.Entries))
	quotes := map[string]int{}
	for k, e := range v.Entries {
		ids[k], lines[k] = e.ID, e.Line
		checkDates(e.Published, e.Fetched, batchDate(v.Batches, e.Batch), today, e.Line, add)
		if p := placeholder(e.Quote + " " + e.Publisher + " " + e.URL); p != "" {
			add(RuleSubstance, e.Line, "Template placeholder "+p+" left in an entry.", "Replace it with the real text or source.")
		}
		key := strings.Join(strings.Fields(strings.ToLower(e.Quote)), " ")
		if first, seen := quotes[key]; seen && key != "" {
			add(RuleSubstance, e.Line, fmt.Sprintf("Same quote as E%03d.", first), "Record each statement once.")
		} else {
			quotes[key] = e.ID
		}
	}
	checkSequence(ids, lines, "E", add)
}

func shortfall(batches []Batch) bool {
	for _, b := range batches {
		if b.Shortfall != "" {
			return true
		}
	}
	return false
}

func shortfallPeers(c Peers) bool { return shortfall(c.Batches) }

// Short reports whether the voices and the peers each meet their minimums or
// name a shortfall.
func (r Report) Short() (voices, peers bool) {
	s := r.Summary
	voices = (s.Voices < MinVoices || s.Platforms < MinPlatforms) && !(r.Voices != nil && shortfall(r.Voices.Batches))
	peers = s.Peers < MinPeers && !(r.Peers != nil && shortfall(r.Peers.Batches))
	return voices, peers
}

func fromZero(batches []Batch) bool {
	for _, b := range batches {
		if b.FromZero {
			return true
		}
	}
	return false
}
