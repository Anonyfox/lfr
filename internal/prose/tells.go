package prose

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Anonyfox/lfr/internal/aitext"
)

// ---------- copy.tells ----------

// A tell is a stock phrase of generated copy. Each pattern is narrow enough that
// a match is the phrase itself, not an ordinary use of one of its words: a
// knitting page may say "seamless", it will not say "seamless experience".
// Repeated constructions only count from their threshold on, because one use is
// ordinary language and the habit is the tell. Every pattern names literal
// keywords one of which any match contains, so the regular expression only
// runs on the few text blocks that contain one.
type tell struct {
	min  int      // occurrences on the page before the construction counts
	keys []string // lowercase literals, one of which every match contains
	re   *regexp.Regexp
}

type tellSource struct {
	min     int
	keys    []string
	pattern string
}

var copyTells = func() []tell {
	sources := []tellSource{
		{1, []string{"today"}, `in today'?s (?:fast[- ]paced|digital|modern|ever[- ](?:changing|evolving)|competitive|busy) (?:world|age|landscape|era|economy)`},
		{1, []string{"ever"}, `in (?:an|the|this) ever[- ](?:evolving|changing) (?:world|landscape|field|market)`},
		{1, []string{"unlock"}, `unlock (?:the (?:full )?(?:power|potential)|your (?:full |true )?potential)`},
		{1, []string{"secret"}, `unlock(?:ing)? (?:the |its |their |your )?secrets?`},
		{1, []string{"guide"}, `(?:the|a|your) (?:ultimate|complete|definitive|comprehensive|essential) guide`},
		{1, []string{"everything you need"}, `everything you need to know`},
		{1, []string{"art of"}, `master(?:ing)? the art of`},
		{1, []string{"geheimnis"}, `(?:entdecke|entdecken sie|lüfte) (?:das|die) geheimnis(?:se)?`},
		{1, []string{"wissen musst"}, `alles,? was (?:du|ihr) (?:über \pL+ )?wissen m(?:u|ü)sst`},
		{1, []string{"leitfaden"}, `(?:der|dein|ihr) (?:ultimative|komplette|vollständige|umfassende) (?:leitfaden|guide|ratgeber)`},
		{1, []string{"reise"}, `(?:deine|ihre|die) reise zu (?:einem|einer|mehr) `},
		{1, []string{"unleash"}, `unleash (?:the|your) (?:full )?(?:power|potential|creativity|inner)`},
		{1, []string{"elevate"}, `elevate your (?:\w+ )?(?:game|experience|brand|business|skills|workflow|routine|style)`},
		{1, []string{"next level"}, `take (?:your|it) (?:\w+ ){0,3}to the next level`},
		{1, []string{"game"}, `game[- ]chang(?:er|ing)`},
		{1, []string{"look no further"}, `look no further`},
		{1, []string{"goodbye"}, `say goodbye to`},
		{1, []string{"delv"}, `delv(?:e|es|ing) (?:into|deeper)`},
		{1, []string{"testament"}, `(?:is|stands as|serves as) a testament to`},
		{1, []string{"seamless"}, `seamless(?:ly)? (?:experience|integration|integrat\w+|workflow|journey|solution|blend)`},
		{1, []string{"harness"}, `harness(?:es|ing)? the (?:full )?(?:power|potential)`},
		{1, []string{"embark"}, `embark (?:on|upon) (?:a|an|your) (?:\w+ )?(?:journey|adventure|quest)`},
		{1, []string{"navigat"}, `navigat(?:e|ing) the (?:complexities|intricacies|ins and outs)`},
		{1, []string{"realm"}, `in the realm of`},
		{1, []string{"note", "remember", "mention"}, `it'?s (?:important|worth|crucial|essential) to (?:note|remember|mention)`},
		{1, []string{"whether"}, `whether you'?re an? [^.,;:!?]{1,30} or an?`},
		{1, []string{"cutting"}, `cutting[- ]edge (?:technology|solutions?|tools?|platform|features?|ai)`},
		{1, []string{"state"}, `state[- ]of[- ]the[- ]art (?:technology|solutions?|platform)`},
		{1, []string{"the way you"}, `(?:revolutioni[sz]e|transform) the way you`},
		{1, []string{"supercharge"}, `supercharge your`},
		{1, []string{"empower"}, `empower(?:s|ing)? (?:you|your|businesses|teams|individuals) to`},
		{1, []string{"we understand"}, `we understand (?:how|that) [^.!?]{0,40}(?:overwhelming|frustrating|stressful|daunting|challenging)`},
		{1, []string{"nestled"}, `nestled (?:in|among|between) the`},
		{1, []string{"tapestry"}, `(?:rich|vibrant|intricate) tapestry`},
		{1, []string{"dive"}, `(?:let'?s|ready to) dive (?:in|into|deeper)`},
		{1, []string{". no "}, `no (?:\w+)\. no (?:\w+)\. (?:just|only)`},
		{1, []string{"heutigen"}, `in der heutigen (?:schnelllebigen |digitalen |modernen )?(?:welt|zeit)`},
		{1, []string{"zeitalter"}, `im (?:digitalen|heutigen) zeitalter`},
		{1, []string{"in der"}, `in einer (?:welt|zeit), in der`},
		{1, []string{"tauchen"}, `tauchen sie (?:tiefer )?ein`},
		{1, []string{"level", "stufe"}, `(?:auf das|aufs|auf ein|auf die) (?:nächste|neues|nächstes) (?:level|stufe)`},
		{1, []string{"nahtlos"}, `nahtlos(?:e|en|er)? (?:integration|integriert|erlebnis|zusammenarbeit|übergang|einfügen)`},
		{1, []string{"es ist"}, `es ist (?:wichtig|erwähnenswert|entscheidend) zu (?:beachten|erwähnen|wissen)`},
		{1, []string{"oder"}, `ob (?:anfänger|einsteiger|neuling) oder (?:profi|experte|fortgeschrittene)`},
		{1, []string{"entfesseln"}, `entfesseln sie`},
		{1, []string{"potenzial"}, `(?:schöpfen sie|entfalten sie) (?:das|ihr) (?:volle[sn]? )?potenzial`},
		{1, []string{". kein"}, `kein(?:e)? [a-zäöüß]+\. kein(?:e)? [a-zäöüß]+\. (?:nur|einfach|sondern)`},
		{2, []string{"mühelos"}, `mühelos\p{L}*`},
		{2, []string{"nahtlos"}, `nahtlos\p{L}*`},
		{2, []string{"maßgeschneidert"}, `maßgeschneidert\p{L}*`},
		{2, []string{"ganzheitlich"}, `ganzheitlich\p{L}*`},
		{2, []string{"innovativ"}, `innovativ\p{L}*`},
		{2, []string{"revolutionär"}, `revolutionär\p{L}*`},
		{2, []string{"entdecken sie"}, `entdecken sie`},
		{2, []string{"effortless"}, `effortless(?:ly)?`},
		{2, []string{"revolutionary"}, `revolutionary`},
		{2, []string{"holistic"}, `holistic`},
		{2, []string{"not "}, `not (?:just|only|merely) (?:a |an |the )?[^.!?;:]{1,60}?[,;—–-]? but`},
		{2, []string{"not "}, `(?:it'?s|this is|that'?s) not (?:just )?(?:about )?[^.!?;:]{1,50}?[.,;—–]\s*(?:it'?s|this is|that'?s)`},
		{2, []string{"nicht nur"}, `nicht nur [^.!?;:]{1,60}?,? sondern (?:auch)?`},
	}
	out := make([]tell, len(sources))
	for i, src := range sources {
		out[i] = tell{src.min, src.keys, regexp.MustCompile(`(?i)\b(?:` + src.pattern + `)\b`)}
	}
	return out
}()

// minDashes and dashesPer100 bound em-dash density: four or more, and more than
// one per hundred words. Human sales copy rarely reaches both.
const minDashes, dashesPer100 = 4, 1.0

// ProseDashes counts em dashes used as sentence breaks; ranges between
// numbers ("13—14 Nov") are not counted.
func ProseDashes(s string) int {
	n := 0
	for i := strings.Index(s, "—"); i >= 0; {
		before, after := strings.TrimRight(s[:i], " "), strings.TrimLeft(s[i+len("—"):], " ")
		if !(endsDigit(before) && startsDigit(after)) {
			n++
		}
		next := strings.Index(s[i+len("—"):], "—")
		if next < 0 {
			break
		}
		i += len("—") + next
	}
	return n
}

func endsDigit(s string) bool   { return s != "" && s[len(s)-1] >= '0' && s[len(s)-1] <= '9' }
func startsDigit(s string) bool { return s != "" && s[0] >= '0' && s[0] <= '9' }

func containsAny(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func normalizeQuotes(s string) string {
	return strings.NewReplacer("’", "'", "‘", "'", " ", " ").Replace(s)
}

// FindTells applies the copy tells of copy.tells to plain prose segments, such
// as the paragraphs of a book chapter, with the same thresholds.
func FindTells(segments []string) []TellHit {
	hits := make([]TellHit, len(copyTells))
	for i := range hits {
		hits[i].Segment = -1
	}
	for n, segment := range segments {
		text := normalizeQuotes(segment)
		lower := strings.ToLower(text)
		for i, t := range copyTells {
			if !containsAny(lower, t.keys) {
				continue
			}
			for _, m := range t.re.FindAllString(text, -1) {
				if hits[i].Count == 0 {
					hits[i].Segment, hits[i].Phrase = n, strings.Join(strings.Fields(m), " ")
				}
				hits[i].Count++
			}
		}
	}
	var out []TellHit
	for i, h := range hits {
		if h.Count == 0 || h.Count < copyTells[i].min {
			continue
		}
		h.Repeat = copyTells[i].min > 1
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Segment < out[j].Segment })
	return out
}

// Placeholder returns template filler found in text (lorem ipsum, TODO,
// "your … here", [insert …]), or "".
func Placeholder(text string) string {
	for _, re := range placeholderText {
		if m := re.FindString(text); m != "" {
			return m
		}
	}
	return ""
}

// MachineMessage says why detectors would flag a text.
func MachineMessage(what string, r aitext.Result) string {
	var why []string
	for _, s := range r.Signals {
		why = append(why, s.Detail)
	}
	reason := ""
	if len(why) > 0 {
		reason = ": " + strings.Join(why, "; ")
	}
	return fmt.Sprintf("AI-text detectors would likely flag %s as generated (%d words)%s.", what, r.Words, reason)
}

// MachineHint is the advice for a text detectors would flag: what to change
// for each habit measured, and never to game the detector.
func MachineHint(r aitext.Result) string {
	var do []string
	for _, s := range r.Signals {
		if a, ok := machineAdvice[s.Feature]; ok {
			do = append(do, a)
		}
	}
	if len(do) == 0 {
		do = append(do, "rewrite in one person's voice, with real names and numbers")
	}
	return "Revise the passages that read most generic: " + strings.Join(do, "; ") + ". Never add errors or random words to fool a detector; readers notice the same habits."
}

// ---------- prose tells for other text than the page ----------

// TellHit is one stock phrase or repeated construction found in a sequence of
// prose segments: the segment of its first match, that match and how often it
// occurred across all segments.
type TellHit struct {
	Segment int
	Phrase  string
	Count   int
	Repeat  bool // a construction that counts only from its second use
}

// machineAdvice is what to change for each measured habit.
var machineAdvice = map[string]string{
	"claude-likeness":         "say it the way the readers would, in their words from the research, not the stock phrasing models reach for",
	"model-likeness":          "drop the stock phrases of marketing copy and say the specific thing",
	"lexical-diversity":       "keep the plain word when it returns instead of a new synonym each time",
	"repeated-trigrams":       "let the key phrase come back where the thought returns, as people do",
	"word-length-spread":      "mix plain short words with the exact long term where it belongs",
	"mid-length-sentences":    "let a sentence run long where the thought needs it and stop short where it lands",
	"sentence-burstiness":     "let sentence length follow the thought: long for the explanation, short for the turn",
	"short-sentences":         "join the clipped fragments to the sentences they belong to",
	"asides":                  "keep the asides a person makes: a parenthesis, a doubt, a question the reader would ask",
	"fillers":                 "write as you would say it to the reader, with the small words of speech where they fit",
	"habits":                  "cut the set-ups that correct what nobody said, the announcements and the stock words",
	"participles":             "end sentences on their point, not on a participle clause that comments on it",
	"nominals":                "turn abstract nouns back into verbs with someone doing them",
	"even-paragraphs":         "let paragraphs differ: a long one for the case, a one-liner where it lands",
	"one-sentence-paragraphs": "merge the one-line paragraphs into the paragraphs they conclude",
	"paragraph-burstiness":    "let paragraph length follow the material",
	"opener-diversity":        "start sentences from what they are about",
}

var placeholderText = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\blorem ipsum\b`),
	regexp.MustCompile(`\b(?:TODO|TBD|FIXME)\b`),
	regexp.MustCompile(`(?i)\byour (?:headline|tagline|title|text|copy|name|company|product|description|logo) (?:goes )?here\b`),
	regexp.MustCompile(`(?i)\[(?:insert|add|your|placeholder)[^\]]{0,40}\]`),
}
