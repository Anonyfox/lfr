package aitext

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// Pattern is a Words pattern with a literal prefilter: every match contains
// one of its keys, so the regular expression only runs on text holding one.
// Go's regexp cannot skip ahead through an alternation framed by word
// boundaries; the prefilter makes the common case (no key in the paragraph)
// a few substring searches.
type Pattern struct {
	re   *regexp.Regexp
	keys []string // lowercase; nil when no key could be derived
}

// minKey is the shortest key worth testing in running text; shorter keys
// match nearly every paragraph.
const minKey = 4

func newPattern(expr, alternation string) *Pattern {
	return &Pattern{re: regexp.MustCompile(expr), keys: literalKeys(alternation, minKey)}
}

// Compile is a regular expression with a prefilter of the literals every
// match contains, each at least that many runes long (3 suits single words, 4
// running text).
func Compile(expr string, least int) *Pattern {
	return &Pattern{re: regexp.MustCompile(expr), keys: literalKeys(expr, least)}
}

// literalKeys returns literals one of which every match contains, or nil
// when the alternation has no selective set of them.
func literalKeys(alternation string, least int) []string {
	re, err := syntax.Parse(alternation, syntax.Perl|syntax.FoldCase)
	if err != nil {
		return nil
	}
	keys := required(re, least)
	seen := map[string]bool{}
	out := keys[:0]
	for _, k := range keys {
		if k = strings.ToLower(k); !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// required returns a set of literals one of which every match of re
// contains, each at least that many runes long, or nil.
func required(re *syntax.Regexp, least int) []string {
	switch re.Op {
	case syntax.OpLiteral:
		if utf8.RuneCountInString(string(re.Rune)) >= least {
			return []string{string(re.Rune)}
		}
	case syntax.OpCapture, syntax.OpPlus:
		return required(re.Sub[0], least)
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return required(re.Sub[0], least)
		}
	case syntax.OpAlternate:
		var all []string
		for _, sub := range re.Sub {
			k := required(sub, least)
			if k == nil {
				return nil
			}
			all = append(all, k...)
		}
		return all
	case syntax.OpConcat:
		// Any child's set works; keep the most selective: the one whose
		// shortest literal is longest.
		var best []string
		bestLen := 0
		for _, sub := range re.Sub {
			k := required(sub, least)
			if k == nil {
				continue
			}
			shortest := 1 << 30
			for _, x := range k {
				shortest = min(shortest, utf8.RuneCountInString(x))
			}
			if shortest > bestLen {
				best, bestLen = k, shortest
			}
		}
		return best
	}
	return nil
}

// may reports whether s can contain a match.
func (p *Pattern) may(s string) bool {
	if p.keys == nil {
		return true
	}
	lower := strings.ToLower(s)
	for _, k := range p.keys {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// MatchString reports whether s contains a match.
func (p *Pattern) MatchString(s string) bool { return p.may(s) && p.re.MatchString(s) }

// FindString returns the leftmost match, or "".
func (p *Pattern) FindString(s string) string {
	if !p.may(s) {
		return ""
	}
	return p.re.FindString(s)
}

// FindAllString returns up to n matches (all for n < 0).
func (p *Pattern) FindAllString(s string, n int) []string {
	if !p.may(s) {
		return nil
	}
	return p.re.FindAllString(s, n)
}

// FindStringIndex returns the location of the leftmost match, or nil.
func (p *Pattern) FindStringIndex(s string) []int {
	if !p.may(s) {
		return nil
	}
	return p.re.FindStringIndex(s)
}
