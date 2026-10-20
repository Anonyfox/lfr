// Package research reads and checks the research files of a newsletter
// project: verbatim voices of readers and writers, the peer newsletters of
// the niche, and evidence from standards and studies. The formats follow
// lfm's research files, so the same discipline applies: every item is
// verbatim, dated, retrievable and cited by ID.
//
// Structure lives only in lines that start with a heading marker, a field
// marker or a table pipe. Quotes are blockquotes: every line starts with ">",
// so verbatim text may contain anything.
package research

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Files of a newsletter project's research directory.
const (
	Dir          = "research"
	VoicesFile   = "voices.md"
	PeersFile    = "peers.md"
	EvidenceFile = "evidence.md"
)

// Minimums for research started from zero.
const (
	MinVoices    = 40
	MinPlatforms = 4
	MinPeers     = 8
)

// Fixed vocabularies.
var (
	Authors       = []string{"reader", "writer", "adjacent", "expert", "brand"}
	Methods       = []string{"connector", "fetch", "browser", "user"}
	EvidenceKinds = []string{"standard", "study", "benchmark", "guide", "survey"}
	// PeerFields are the facts recorded for every peer newsletter, in order.
	PeerFields = []string{"Cadence", "Length", "Structure", "Subject", "Opening", "Voice", "Visual", "Links", "Signup", "Takeaway"}
	// Dimensions are the aspects evidence can apply to.
	Dimensions = []string{"deliverability", "consent", "signup", "welcome", "frequency", "timing", "length", "subject", "preheader",
		"layout", "accessibility", "images", "links", "tone", "structure", "personalization", "engagement", "growth", "retention",
		"unsubscribe", "hygiene", "measurement", "legal", "plaintext", "darkmode", "mobile"}
)

const sep = " · "

// Problem is a format defect at a line.
type Problem struct {
	Line    int
	Message string
	Hint    string
}

// Batch is one dated research session.
type Batch struct {
	Line        int
	Date        time.Time
	FromZero    bool
	Trigger     string
	Queries     string
	Unreachable string
	Shortfall   string
}

// Precision is how exactly a source dates a post.
type Precision int

const (
	Unknown Precision = iota
	Year
	Month
	Day
)

// Voice is one verbatim quote.
type Voice struct {
	Line      int
	ID        int
	Platform  string
	Place     string
	Author    string
	Posted    time.Time
	Precision Precision
	Fetched   time.Time
	Via       string
	Quote     string
	URL       string
	Observer  string
	Batch     int
}

// Voices is voices.md.
type Voices struct {
	Title   string
	Batches []Batch
	Entries []Voice
}

// Peer is one peer newsletter.
type Peer struct {
	Line    int
	ID      int
	Name    string
	URL     string
	Fetched time.Time
	Via     string
	Facts   map[string]string
	Batch   int
}

// GridRow is one value of one fact and the number of peers showing it.
type GridRow struct {
	Line   int
	Column string
	Value  string
	Peers  int
}

// Peers is peers.md. Grid, Baseline and Gaps are those of the latest batch that has them.
type Peers struct {
	Title    string
	Batches  []Batch
	Entries  []Peer
	Grid     []GridRow
	GridLine int
	Baseline string
	Gaps     []string
}

// Evidence is one fact from a standard, study, benchmark or guide.
type Evidence struct {
	Line      int
	ID        int
	Publisher string
	Work      string
	Kind      string
	Published time.Time
	Precision Precision
	Fetched   time.Time
	Via       string
	Quote     string
	URL       string
	Applies   []string
	Observer  string
	Batch     int
}

// Evidences is evidence.md.
type Evidences struct {
	Title   string
	Batches []Batch
	Entries []Evidence
}

func lines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func date(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

// periodDate reads YYYY-MM-DD, YYYY-MM or YYYY.
func periodDate(s string) (time.Time, Precision, bool) {
	for _, f := range []struct {
		layout    string
		precision Precision
	}{{time.DateOnly, Day}, {"2006-01", Month}, {"2006", Year}} {
		if len(s) == len(f.layout) {
			if t, err := time.Parse(f.layout, s); err == nil {
				return t, f.precision, true
			}
		}
	}
	return time.Time{}, Unknown, false
}

func oneOf(s string, list []string) bool {
	for _, item := range list {
		if s == item {
			return true
		}
	}
	return false
}

// mailRef points at a reply in the newsletter's mailbox (lfr mail read ID)
// without naming the reader.
var mailRef = regexp.MustCompile(`^lfr:mail/[0-9]+$`)

func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " <>")
}

func quote(s string) string { return "\"" + preview(s) + "\"" }

func preview(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// parseBatch reads "## Batch <date> · <mode>[ · queries: … · unreachable: …[ · shortfall: …]]".
func parseBatch(line string, n int, withSources bool) (Batch, *Problem) {
	format := "## Batch YYYY-MM-DD · from zero | ongoing: <trigger>[ · shortfall: <what and why>]"
	if withSources {
		format += ` · queries: "…" · unreachable: <platforms or none>`
	}
	bad := &Problem{Line: n, Message: "Batch heading does not match the format.", Hint: "Write it as: " + format}
	rest, ok := strings.CutPrefix(line, "## Batch ")
	if !ok {
		return Batch{}, bad
	}
	fields := strings.Split(rest, sep)
	if len(fields) < 2 {
		return Batch{}, bad
	}
	b := Batch{Line: n}
	if b.Date, ok = date(fields[0]); !ok {
		return Batch{}, &Problem{Line: n, Message: "Batch date " + quote(fields[0]) + " is not YYYY-MM-DD.", Hint: "Use the date of the research session."}
	}
	switch mode := fields[1]; {
	case mode == "from zero":
		b.FromZero = true
	case strings.HasPrefix(mode, "ongoing: ") && strings.TrimSpace(mode[len("ongoing: "):]) != "":
		b.Trigger = strings.TrimSpace(mode[len("ongoing: "):])
	default:
		return Batch{}, &Problem{Line: n, Message: "Batch mode " + quote(mode) + " is neither \"from zero\" nor \"ongoing: <trigger>\".", Hint: "Name the mode; an ongoing batch says what triggered it."}
	}
	if !withSources && (len(fields) == 2 || !strings.HasPrefix(fields[2], "queries: ")) {
		// Peer batches need not name queries; they may still name a shortfall.
		for _, f := range fields[2:] {
			v, ok := strings.CutPrefix(f, "shortfall: ")
			if !ok || strings.TrimSpace(v) == "" {
				return Batch{}, bad
			}
			b.Shortfall = strings.TrimSpace(v)
		}
		return b, nil
	}
	tail := strings.Join(fields[2:], sep)
	queries, unreachable, found := strings.Cut(tail, sep+"unreachable: ")
	unreachable, shortfall, hasShortfall := strings.Cut(unreachable, sep+"shortfall: ")
	if hasShortfall {
		if b.Shortfall = strings.TrimSpace(shortfall); b.Shortfall == "" {
			return Batch{}, &Problem{Line: n, Message: "Empty shortfall in the batch heading.", Hint: "Say what is missing and why, or drop the field."}
		}
	}
	queries, hasQueries := strings.CutPrefix(queries, "queries: ")
	if !found || !hasQueries || strings.TrimSpace(queries) == "" || strings.TrimSpace(unreachable) == "" {
		return Batch{}, &Problem{Line: n, Message: "Batch heading lacks its queries or unreachable list.", Hint: "List the searches and the sources that could not be reached (or none): " + format}
	}
	b.Queries, b.Unreachable = strings.TrimSpace(queries), strings.TrimSpace(unreachable)
	return b, nil
}

// entryID reads "V012", "P003" or "E101".
func entryID(field, prefix string) (int, bool) {
	digits, ok := strings.CutPrefix(field, prefix)
	if !ok || len(digits) < 3 || len(digits) > 6 {
		return 0, false
	}
	id := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		id = id*10 + int(c-'0')
	}
	return id, id > 0
}

// quoteBlock reads the ">" lines, the <url> line and an optional observer note after a heading.
func quoteBlock(ls []string, i int) (text, link, observer string, next int, linkLine int) {
	j := i + 1
	var qs []string
	for ; j < len(ls) && strings.HasPrefix(ls[j], ">"); j++ {
		qs = append(qs, strings.TrimPrefix(strings.TrimPrefix(ls[j], ">"), " "))
	}
	text = strings.Join(qs, "\n")
	if j < len(ls) {
		raw := strings.TrimSpace(ls[j])
		if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
			link, linkLine = raw[1:len(raw)-1], j+1
			j++
		}
	}
	return text, link, observer, j, linkLine
}

func observerNote(ls []string, j int) (string, int) {
	if j < len(ls) {
		if note, ok := strings.CutPrefix(ls[j], "Observer note: "); ok {
			return strings.TrimSpace(note), j + 1
		}
	}
	return "", j
}

// platform splits "Hacker News (Ask HN: …)" into its platform and place.
func platform(field string) (name, place string) {
	field = strings.TrimSpace(field)
	if open := strings.Index(field, " ("); open > 0 && strings.HasSuffix(field, ")") {
		return field[:open], field[open+2 : len(field)-1]
	}
	return field, ""
}

// ParseVoices reads voices.md, reporting every defect.
func ParseVoices(text string) (Voices, []Problem) {
	var v Voices
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	ls := lines(text)
	if title, ok := strings.CutPrefix(ls[0], "# Voices: "); ok && strings.TrimSpace(title) != "" {
		v.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Voices: <newsletter>\".", "Restore the title line.")
	}
	format := "### V001 · Platform (place) · " + strings.Join(Authors, "|") + " · posted YYYY-MM-DD|YYYY-MM|YYYY|unknown · fetched YYYY-MM-DD · via " + strings.Join(Methods, "|")
	for i := 1; i < len(ls); i++ {
		line, n := ls[i], i+1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "## "):
			if b, p := parseBatch(line, n, true); p != nil {
				problems = append(problems, *p)
			} else {
				v.Batches = append(v.Batches, b)
			}
		case strings.HasPrefix(line, "|"):
			if len(v.Batches) == 0 {
				add(n, "Table outside a batch.", "Put behaviour numbers in a table directly under their batch heading.")
			}
		case strings.HasPrefix(line, "### "):
			e, next, ps := parseVoice(ls, i, format)
			problems = append(problems, ps...)
			if e.ID > 0 {
				if len(v.Batches) == 0 {
					add(n, "Voice before the first batch heading.", "Every voice belongs to a dated batch.")
				}
				e.Batch = len(v.Batches) - 1
				v.Entries = append(v.Entries, e)
			}
			i = next - 1
		default:
			add(n, "Unexpected line: "+preview(line), "Only batch headings, tables, voice headings, quote lines (\">\"), the source URL and an observer note belong here.")
		}
	}
	return v, problems
}

func parseVoice(ls []string, i int, format string) (Voice, int, []Problem) {
	n := i + 1
	var problems []Problem
	add := func(line int, message, hint string) { problems = append(problems, Problem{line, message, hint}) }
	v := Voice{Line: n}
	fields := strings.Split(strings.TrimPrefix(ls[i], "### "), sep)
	bad := func() { add(n, "Voice heading does not match the format.", "Write it as: "+format) }
	if len(fields) != 6 {
		bad()
	} else {
		var ok bool
		if v.ID, ok = entryID(fields[0], "V"); !ok {
			add(n, "Voice ID "+quote(fields[0])+" is not V followed by at least three digits.", "Continue the sequence: V001, V002, …")
		}
		v.Platform, v.Place = platform(fields[1])
		switch {
		case v.Platform == "":
			add(n, "Voice has no platform.", "Name the service it was posted on, e.g. Hacker News (Ask HN: …).")
		case strings.ContainsAny(v.Platform, "\"“”()") || len([]rune(v.Platform)) > 32:
			add(n, "Platform "+quote(v.Platform)+" reads like a place.", "Name the service only and put the thread or page in parentheses.")
		}
		if v.Author = fields[2]; !oneOf(v.Author, Authors) {
			add(n, "Author type "+quote(v.Author)+" is not one of "+strings.Join(Authors, ", ")+".", "Pick the closest type.")
		}
		if posted, ok := strings.CutPrefix(fields[3], "posted "); !ok {
			bad()
		} else if posted != "unknown" {
			if v.Posted, v.Precision, ok = periodDate(posted); !ok {
				add(n, "Posted date "+quote(posted)+" is not YYYY-MM-DD, YYYY-MM, YYYY or unknown.", "Give the date as precisely as the source shows it.")
			}
		}
		if fetched, ok := strings.CutPrefix(fields[4], "fetched "); !ok {
			bad()
		} else if v.Fetched, ok = date(fetched); !ok {
			add(n, "Fetched date "+quote(fetched)+" is not YYYY-MM-DD.", "Use the day you opened the source.")
		}
		if via, ok := strings.CutPrefix(fields[5], "via "); !ok || !oneOf(via, Methods) {
			add(n, "Fetch method "+quote(fields[5])+" is not via "+strings.Join(Methods, ", ")+".", "Say how the source was obtained.")
		} else {
			v.Via = via
		}
	}
	var linkLine int
	var j int
	v.Quote, v.URL, _, j, linkLine = quoteBlock(ls, i)
	if strings.Trim(strings.TrimSpace(v.Quote), "…. ") == "" {
		add(n, "Voice has no quote.", "Copy the passage verbatim into lines that start with \">\".")
	}
	switch {
	case v.URL == "":
		add(n, "Voice has no source URL after its quote.", "Add the link on its own line, in angle brackets: <https://…>.")
	case !webURL(v.URL) && !mailRef.MatchString(v.URL):
		add(linkLine, "Source "+quote(v.URL)+" is not an absolute web URL.", "Link the page where the quote can be reopened; a reply to the newsletter is <lfr:mail/ID>.")
	}
	v.Observer, j = observerNote(ls, j)
	return v, j, problems
}

// ParsePeers reads peers.md.
func ParsePeers(text string) (Peers, []Problem) {
	var c Peers
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	ls := lines(text)
	if title, ok := strings.CutPrefix(ls[0], "# Peers: "); ok && strings.TrimSpace(title) != "" {
		c.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Peers: <newsletter>\".", "Restore the title line.")
	}
	section := ""
	var grid []GridRow
	var gridLine, tableRows int
	var baseline, gaps []string
	flush := func() {
		if gridLine > 0 {
			c.Grid, c.GridLine = grid, gridLine
		}
		if len(baseline) > 0 {
			c.Baseline = strings.Join(baseline, " ")
		}
		if len(gaps) > 0 {
			c.Gaps = gaps
		}
		grid, gridLine, baseline, gaps = nil, 0, nil, nil
	}
	for i := 1; i < len(ls); i++ {
		line, n := ls[i], i+1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "## "):
			flush()
			section = ""
			if b, p := parseBatch(line, n, false); p != nil {
				problems = append(problems, *p)
			} else {
				c.Batches = append(c.Batches, b)
			}
		case line == "### Grid after this batch":
			section, gridLine, tableRows = "grid", n, 0
		case line == "### Baseline":
			section = "baseline"
		case line == "### Gaps":
			section = "gaps"
		case strings.HasPrefix(line, "### "):
			section = "peer"
			p, ps := parsePeerHeading(line, n)
			problems = append(problems, ps...)
			if p.ID > 0 {
				if len(c.Batches) == 0 {
					add(n, "Peer before the first batch heading.", "Every peer belongs to a dated batch.")
				}
				p.Batch = len(c.Batches) - 1
				c.Entries = append(c.Entries, p)
			}
		case section == "peer" && strings.HasPrefix(line, "- "):
			key, value, ok := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
			if len(c.Entries) == 0 {
				continue
			}
			peer := &c.Entries[len(c.Entries)-1]
			switch {
			case !ok || !oneOf(key, PeerFields):
				add(n, "Unknown peer fact "+quote(key)+".", "Use one line per fact: "+strings.Join(PeerFields, ", ")+".")
			case peer.Facts[key] != "":
				add(n, "Peer fact "+key+" appears twice.", "Keep one line per fact.")
			case strings.TrimSpace(value) == "":
				add(n, "Peer fact "+key+" is empty.", "Record what the newsletter shows, or none.")
			default:
				peer.Facts[key] = strings.TrimSpace(value)
			}
		case section == "grid" && strings.HasPrefix(line, "|"):
			cells := strings.Split(strings.Trim(line, "|"), "|")
			for k := range cells {
				cells[k] = strings.TrimSpace(cells[k])
			}
			if len(cells) != 3 {
				add(n, "Grid rows have three cells: column, value, peers.", "Write: | Cadence | weekly | 7 |")
				continue
			}
			// The first row names the columns, whatever words it uses.
			if tableRows++; tableRows == 1 || strings.Trim(cells[0], "-: ") == "" {
				continue
			}
			count, ok := number(cells[2])
			if !oneOf(cells[0], PeerFields) || cells[1] == "" || !ok {
				add(n, "Grid row "+preview(line)+" is not | <fact> | <value> | <count> |.", "Use a peer fact as the column and a whole number of peers.")
				continue
			}
			grid = append(grid, GridRow{Line: n, Column: cells[0], Value: cells[1], Peers: count})
		case section == "baseline":
			baseline = append(baseline, strings.TrimSpace(line))
		case section == "gaps" && strings.HasPrefix(line, "- "):
			gaps = append(gaps, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		default:
			add(n, "Unexpected line: "+preview(line), "Peers are a heading plus one \"- Fact: value\" line per fact; then the grid table, the baseline paragraph and the gaps list.")
		}
	}
	flush()
	return c, problems
}

func parsePeerHeading(line string, n int) (Peer, []Problem) {
	format := "### P001 · <name> · https://… · fetched YYYY-MM-DD · via " + strings.Join(Methods, "|")
	fields := strings.Split(strings.TrimPrefix(line, "### "), sep)
	if len(fields) != 5 {
		return Peer{}, []Problem{{n, "Peer heading does not match the format.", "Write it as: " + format}}
	}
	var problems []Problem
	p := Peer{Line: n, Name: strings.TrimSpace(fields[1]), URL: strings.Trim(fields[2], "<>"), Facts: map[string]string{}}
	var ok bool
	if p.ID, ok = entryID(fields[0], "P"); !ok {
		problems = append(problems, Problem{n, "Peer ID " + quote(fields[0]) + " is not P followed by at least three digits.", "Continue the sequence: P001, P002, …"})
	}
	if p.Name == "" {
		problems = append(problems, Problem{n, "Peer has no name.", "Name the newsletter."})
	}
	if !webURL(p.URL) {
		problems = append(problems, Problem{n, "Peer URL " + quote(p.URL) + " is not an absolute web URL.", "Link the archive or issue you analyzed."})
	}
	if fetched, found := strings.CutPrefix(fields[3], "fetched "); !found {
		problems = append(problems, Problem{n, "Peer heading lacks its fetched date.", "Write it as: " + format})
	} else if p.Fetched, ok = date(fetched); !ok {
		problems = append(problems, Problem{n, "Fetched date " + quote(fetched) + " is not YYYY-MM-DD.", "Use the day you opened the page."})
	}
	if via, found := strings.CutPrefix(fields[4], "via "); !found || !oneOf(via, Methods) {
		problems = append(problems, Problem{n, "Fetch method " + quote(fields[4]) + " is not via " + strings.Join(Methods, ", ") + ".", "Say how the page was obtained."})
	} else {
		p.Via = via
	}
	return p, problems
}

func number(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 1_000_000 {
			return 0, false
		}
	}
	return n, true
}

// ParseEvidence reads evidence.md.
func ParseEvidence(text string) (Evidences, []Problem) {
	var v Evidences
	var problems []Problem
	add := func(n int, message, hint string) { problems = append(problems, Problem{n, message, hint}) }
	ls := lines(text)
	if title, ok := strings.CutPrefix(ls[0], "# Evidence: "); ok && strings.TrimSpace(title) != "" {
		v.Title = strings.TrimSpace(title)
	} else {
		add(1, "The file must start with \"# Evidence: <topic>\".", "Restore the title line.")
	}
	format := "### E001 · Publisher (work) · " + strings.Join(EvidenceKinds, "|") + " · published YYYY-MM-DD|YYYY-MM|YYYY|unknown · fetched YYYY-MM-DD · via " + strings.Join(Methods, "|")
	for i := 1; i < len(ls); i++ {
		line, n := ls[i], i+1
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "## "):
			if b, p := parseBatch(line, n, true); p != nil {
				problems = append(problems, *p)
			} else {
				v.Batches = append(v.Batches, b)
			}
		case strings.HasPrefix(line, "### "):
			e, next, ps := parseEvidence(ls, i, format)
			problems = append(problems, ps...)
			if e.ID > 0 {
				if len(v.Batches) == 0 {
					add(n, "Evidence before the first batch heading.", "Every item belongs to a dated batch.")
				}
				e.Batch = len(v.Batches) - 1
				v.Entries = append(v.Entries, e)
			}
			i = next - 1
		default:
			add(n, "Unexpected line: "+preview(line), "Only batch headings, evidence headings, quote lines (\">\"), the source URL, an Applies line and an observer note belong here.")
		}
	}
	return v, problems
}

func parseEvidence(ls []string, i int, format string) (Evidence, int, []Problem) {
	n := i + 1
	var problems []Problem
	add := func(line int, message, hint string) { problems = append(problems, Problem{line, message, hint}) }
	e := Evidence{Line: n}
	fields := strings.Split(strings.TrimPrefix(ls[i], "### "), sep)
	bad := func() { add(n, "Evidence heading does not match the format.", "Write it as: "+format) }
	if len(fields) != 6 {
		bad()
	} else {
		var ok bool
		if e.ID, ok = entryID(fields[0], "E"); !ok {
			add(n, "Evidence ID "+quote(fields[0])+" is not E followed by at least three digits.", "Continue the sequence: E001, E002, …")
		}
		if e.Publisher, e.Work = platform(fields[1]); e.Publisher == "" {
			add(n, "Evidence has no publisher.", "Name who published it, with the work in parentheses.")
		}
		if e.Kind = fields[2]; !oneOf(e.Kind, EvidenceKinds) {
			add(n, "Evidence kind "+quote(e.Kind)+" is not one of "+strings.Join(EvidenceKinds, ", ")+".", "Pick the closest kind.")
		}
		if published, ok := strings.CutPrefix(fields[3], "published "); !ok {
			bad()
		} else if published != "unknown" {
			if e.Published, e.Precision, ok = periodDate(published); !ok {
				add(n, "Published date "+quote(published)+" is not YYYY-MM-DD, YYYY-MM, YYYY or unknown.", "Give the date as the source shows it.")
			}
		}
		if fetched, ok := strings.CutPrefix(fields[4], "fetched "); !ok {
			bad()
		} else if e.Fetched, ok = date(fetched); !ok {
			add(n, "Fetched date "+quote(fetched)+" is not YYYY-MM-DD.", "Use the day you opened the source.")
		}
		if via, ok := strings.CutPrefix(fields[5], "via "); !ok || !oneOf(via, Methods) {
			add(n, "Fetch method "+quote(fields[5])+" is not via "+strings.Join(Methods, ", ")+".", "Say how the source was obtained.")
		} else {
			e.Via = via
		}
	}
	var linkLine, j int
	e.Quote, e.URL, _, j, linkLine = quoteBlock(ls, i)
	if strings.Trim(strings.TrimSpace(e.Quote), "…. ") == "" {
		add(n, "Evidence has no quote.", "Copy the sentence verbatim into lines that start with \">\".")
	}
	switch {
	case e.URL == "":
		add(n, "Evidence has no source URL after its quote.", "Add the link on its own line, in angle brackets.")
	case !webURL(e.URL):
		add(linkLine, "Source "+quote(e.URL)+" is not an absolute web URL.", "Link the page where the quote can be reopened.")
	}
	if j < len(ls) {
		if applies, ok := strings.CutPrefix(ls[j], "Applies: "); ok {
			for _, d := range strings.Split(applies, ",") {
				d = strings.TrimSpace(d)
				if !oneOf(d, Dimensions) {
					add(j+1, "Unknown dimension "+quote(d)+".", "Use only: "+strings.Join(Dimensions, ", ")+".")
					continue
				}
				e.Applies = append(e.Applies, d)
			}
			j++
		}
	}
	if len(e.Applies) == 0 {
		add(n, "Evidence lacks an Applies line.", "Add \"Applies: <dimension>, …\" after the URL.")
	}
	e.Observer, j = observerNote(ls, j)
	return e, j, problems
}

// Blockquote formats verbatim text as quote lines that parse back to the same text.
func Blockquote(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	quoted := strings.Split(text, "\n")
	for i, line := range quoted {
		if line == "" {
			quoted[i] = ">"
		} else {
			quoted[i] = "> " + line
		}
	}
	return strings.Join(quoted, "\n")
}
