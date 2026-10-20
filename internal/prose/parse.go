package prose

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/Anonyfox/lfr/internal/aitext"
)

// quoteStyles are the quotation marks of a language: left and right double,
// then left and right single. English marks are the default.
var quoteStyles = map[string][4]string{
	"en":    {"“", "”", "‘", "’"},
	"de":    {"„", "“", "‚", "‘"},
	"de-ch": {"«", "»", "‹", "›"},
	"fr":    {"«\u00a0", "\u00a0»", "‹\u00a0", "\u00a0›"},
	"es":    {"«", "»", "“", "”"},
	"it":    {"«", "»", "“", "”"},
	"nl":    {"“", "”", "‘", "’"},
	"pl":    {"„", "”", "‚", "’"},
	"cs":    {"„", "“", "‚", "‘"},
	"sv":    {"”", "”", "’", "’"},
	"da":    {"»", "«", "›", "‹"},
}

// typographerFor turns straight quotes, dashes and dots into the typographic
// characters of a language. The characters themselves, not HTML entities:
// XHTML in an EPUB knows only the five XML entities.
func typographerFor(language string) goldmark.Extender {
	lang := strings.ToLower(language)
	q, ok := quoteStyles[lang]
	if !ok {
		base, _, _ := strings.Cut(lang, "-")
		if q, ok = quoteStyles[base]; !ok {
			q = quoteStyles["en"]
		}
	}
	return extension.NewTypographer(extension.WithTypographicSubstitutions(extension.TypographicSubstitutions{
		extension.LeftDoubleQuote:  []byte(q[0]),
		extension.RightDoubleQuote: []byte(q[1]),
		extension.LeftSingleQuote:  []byte(q[2]),
		extension.RightSingleQuote: []byte(q[3]),
		extension.EnDash:           []byte("–"),
		extension.EmDash:           []byte("—"),
		extension.Ellipsis:         []byte("…"),
		extension.LeftAngleQuote:   []byte("«"),
		extension.RightAngleQuote:  []byte("»"),
		extension.Apostrophe:       []byte("’"),
	}))
}

var typographer = typographerFor("en")

// markdown is the one Markdown dialect of the book: CommonMark with tables,
// strikethrough, footnotes and typographic quotes and dashes.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote, typographer))

type heading struct {
	level int
	text  string
	line  int
}

type block struct {
	text    string
	line    int
	list    bool
	quote   bool
	link    bool // the block links a source
	at      int  // words of the chapter before the block
	section int  // index of the ## section the block is in; 0 before the first
	bold    int  // strong spans in the block
	label   bool // a list item that starts with a bold label ending in a colon
	allBold bool // the whole block is strong
}

type ref struct {
	dest, alt string
	line      int
}

// prose is what the checks need from one chapter.
type prose struct {
	headings     []heading
	blocks       []block // paragraphs and list item text, in order
	images       []ref
	links        []ref
	words        int // all words outside code
	listWords    int
	quoteWords   int
	sectionEmpty []heading // headings directly followed by a heading of the same or a higher level
	rawHTML      []int     // lines with HTML, which the book does not render
	tables       []int     // word offsets of tables
	external     int       // links to https sources
}

// lines maps byte offsets to 1-based line numbers.
type lines []int

func newLines(src []byte) lines {
	starts := lines{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (l lines) at(offset int) int { return sort.SearchInts(l, offset+1) }

func blockLine(n ast.Node, l lines) int {
	for c := n; c != nil; c = c.Parent() {
		if c.Type() == ast.TypeBlock && c.Lines().Len() > 0 {
			return l.at(c.Lines().At(0).Start)
		}
		if c.Type() == ast.TypeBlock {
			if first := c.FirstChild(); first != nil && first.Type() == ast.TypeBlock && first.Lines().Len() > 0 {
				return l.at(first.Lines().At(0).Start)
			}
		}
	}
	return 1
}

// inlineText collects the readable text of an inline subtree.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *ast.Image:
			if c != n {
				return ast.WalkSkipChildren, nil // an image's alt text is not prose
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func parseChapter(src []byte) (ast.Node, prose) {
	doc := markdown.Parser().Parse(text.NewReader(src))
	l := newLines(src)
	var p prose
	section := 0
	inside := func(n ast.Node, kind ast.NodeKind) bool {
		for c := n.Parent(); c != nil; c = c.Parent() {
			if c.Kind() == kind {
				return true
			}
		}
		return false
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Heading:
			p.headings = append(p.headings, heading{v.Level, inlineText(v, src), blockLine(v, l)})
			if v.Level == 2 {
				section++
			}
			return ast.WalkSkipChildren, nil
		case *east.Table:
			p.tables = append(p.tables, p.words)
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock:
			p.rawHTML = append(p.rawHTML, blockLine(v, l))
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			p.rawHTML = append(p.rawHTML, blockLine(v, l))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.TextBlock:
			t := inlineText(v, src)
			b := block{text: t, line: blockLine(v, l), list: inside(v, ast.KindListItem), quote: inside(v, ast.KindBlockquote), at: p.words, section: section}
			strongWords := 0
			_ = ast.Walk(v, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				if c.Kind() == ast.KindLink || c.Kind() == ast.KindAutoLink {
					b.link = true
				}
				if e, ok := c.(*ast.Emphasis); ok && e.Level == 2 {
					b.bold++
					strong := inlineText(e, src)
					strongWords += len(strings.Fields(strong))
					if c.Parent() == v && v.FirstChild() == c && b.list && strings.HasSuffix(strings.TrimSpace(strong), ":") {
						b.label = true
					}
					return ast.WalkSkipChildren, nil
				}
				return ast.WalkContinue, nil
			})
			b.allBold = strongWords > 0 && strongWords == len(strings.Fields(t)) && strongWords >= 8
			p.blocks = append(p.blocks, b)
			w := len(strings.Fields(t))
			p.words += w
			if b.list {
				p.listWords += w
			}
			if b.quote {
				p.quoteWords += w
			}
		case *ast.Image:
			p.images = append(p.images, ref{string(v.Destination), inlineText(v, src), blockLine(v, l)})
		case *ast.Link:
			p.links = append(p.links, ref{dest: string(v.Destination), line: blockLine(v, l)})
			if strings.HasPrefix(string(v.Destination), "https://") {
				p.external++
			}
		case *ast.AutoLink:
			p.links = append(p.links, ref{dest: string(v.URL(src)), line: blockLine(v, l)})
			if strings.HasPrefix(string(v.URL(src)), "https://") {
				p.external++
			}
		}
		return ast.WalkContinue, nil
	})
	// An empty section: a heading followed by a heading of the same or a
	// higher level with nothing between them.
	var prev ast.Node
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok {
			if ph, ok := prev.(*ast.Heading); ok && h.Level <= ph.Level {
				p.sectionEmpty = append(p.sectionEmpty, heading{ph.Level, inlineText(ph, src), blockLine(ph, l)})
			}
		}
		prev = c
	}
	return doc, p
}

var sentenceEnd = regexp.MustCompile(`[.!?…]+["”’)]*(\s+|$)`)

// sentenceLengths splits prose paragraphs (not lists or quotes) into sentences
// and returns their word counts.
func sentenceLengths(p prose) []int {
	var out []int
	for _, b := range p.blocks {
		if b.list || b.quote {
			continue
		}
		for _, s := range sentenceEnd.Split(b.text, -1) {
			if n := len(strings.Fields(s)); n > 0 {
				out = append(out, n)
			}
		}
	}
	return out
}

// variation is the coefficient of variation of the values.
func variation(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += float64(v)
	}
	mean := sum / float64(len(values))
	var sq float64
	for _, v := range values {
		sq += (float64(v) - mean) * (float64(v) - mean)
	}
	return math.Sqrt(sq/float64(len(values))) / mean
}

// Thresholds of the shape checks; each is far from what edited human prose shows.
const (
	minSentencesForRhythm = 40   // sentences before rhythm is judged
	uniformRhythm         = 0.32 // coefficient of variation of sentence length below this reads as generated
	openerShare           = 0.15
	minOpenerRepeats      = 4
)

// bookTells are the habits of generated nonfiction beyond the copy tells:
// announcing the chapter, travel metaphors, and claims of unnamed studies.
var bookTells = []*aitext.Pattern{
	aitext.Compile(`(?i)\bin (?:this|today'?s) (?:chapter|section|book|issue|newsletter|edition|post|email),? (?:we|you|i)(?:'ll| will| are going to| am going to)\b`, 4),
	aitext.Compile(`(?i)\bin (?:diesem|dieser|der heutigen) (?:kapitel|abschnitt|buch|ausgabe|newsletter) (?:werden wir|wirst du|lernst du|erfährst du|schauen wir|zeige ich)\b`, 4),
	aitext.Compile(`(?i)\b(?:fascinating|exciting) world of\b`, 4),
	aitext.Compile(`(?i)\b(?:faszinierende|spannende) welt\b`, 4),
	aitext.Compile(`(?i)\b(?:ein)?tauch(?:en|t|e) (?:wir |du |sie )?(?:tief(?:er)? )?(?:ein|in)\b`, 4),
	aitext.Compile(`(?i)\bjourney,? not (?:a|the) destination\b`, 4),
	aitext.Compile(`(?i)\breise,? (?:kein|nicht das) ziel\b`, 4),
	aitext.Compile(`(?i)\blet'?s (?:explore|dive|take a (?:closer|deeper) look)\b`, 4),
	aitext.Compile(`(?i)\blass(?:t)? uns (?:gemeinsam )?(?:eintauchen|entdecken|einen (?:genaueren|tieferen) blick)\b`, 4),
}

// unsourced matches claims that need a source in the same paragraph: studies,
// research and experts in general, and shares of people.
var unsourced = aitext.Compile(`(?i)\b(?:studies|research|experts|surveys?) (?:show|shows|have shown|has shown|suggest|suggests|agree|say|found)\b|\b(?:studien|untersuchungen|umfragen|experten|forscher) (?:zeigen|belegen|haben gezeigt|sind sich einig|sagen|fanden)\b|\b(?:zeigen|belegen) (?:aktuelle |neue |zahlreiche |viele )?(?:studien|untersuchungen|umfragen)\b|\blaut (?:einer |neuen |aktuellen )?(?:studie|umfrage|untersuchung)\b|\baccording to (?:a |one |recent )?(?:study|survey|research)\b|\b\d+(?:[.,]\d+)?\s?(?:%|percent|prozent) (?:of|aller|der|von den)\b`, 4)

func opener(s string) string {
	words := aitext.Runs(strings.ToLower(s), 2, '\'')
	if len(words) < 2 {
		return ""
	}
	return words[0] + " " + words[1]
}
