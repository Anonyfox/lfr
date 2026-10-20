// Package prose holds writing to the floor readers expect from a person: no
// stock phrases of generated copy, none of the structural habits current
// models have (correcting claims nobody made, announcing instead of saying,
// recap endings, label-colon bullets, pet words), concrete names and numbers,
// paragraphs and sentences shaped like human writing, and an estimate of
// whether AI-text detectors would flag it (package aitext).
//
// The lexicon, thresholds and detector come from lfm, which calibrated them on
// public-domain books, modern essays, edited guides and current model output
// in English and German; other languages get the language-agnostic checks.
// Everything runs in a few milliseconds for a newsletter issue.
package prose

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfr/internal/finding"
)

// Rule IDs.
const (
	RuleMachine     = "prose.machine"
	RuleTells       = "prose.tells"
	RuleCorrective  = "prose.corrective"
	RuleSignpost    = "prose.signpost"
	RulePets        = "prose.pets"
	RuleBold        = "prose.bold"
	RuleLabels      = "prose.labels"
	RuleConnectors  = "prose.connectors"
	RuleParticiples = "prose.participles"
	RuleHedges      = "prose.hedges"
	RuleStockNames  = "prose.stocknames"
	RuleSources     = "prose.sources"
	RuleArtifacts   = "prose.artifacts"
	RuleOpening     = "prose.opening"
	RuleEnding      = "prose.ending"
	RuleSpecifics   = "prose.specifics"
	RuleNominal     = "prose.nominal"
	RuleParagraphs  = "prose.paragraphs"
	RuleFragments   = "prose.fragments"
	RuleReadability = "prose.readability"
	RuleDashes      = "prose.dashes"
	RulePlaceholder = "prose.placeholder"
	RuleRepeat      = "prose.repeat"
	RuleRhythm      = "prose.rhythm"
	RuleOpeners     = "prose.openers"
	RuleQuestions   = "prose.questions"
)

// Rules describes every prose rule.
func Rules() []finding.Rule {
	const why = " Readers unsubscribe from newsletters that read as machine-written (lfr:V113, lfr:V015); an automated AI newsletter grew nothing (lfr:V122)."
	return []finding.Rule{
		{ID: RuleMachine, Summary: "Detectors would not flag the text as generated", Help: "An estimate of what AI-text detectors measure: sentence and paragraph rhythm, punctuation, vocabulary spread, how much the text compresses against model-written prose, the habits of current models. It runs offline in milliseconds; a flag names what to change." + why},
		{ID: RuleTells, Summary: "No stock phrases of generated copy", Help: "\"Delve into\", \"a testament to\", \"unlock the power of\", \"in today's fast-paced world\", \"seamless experience\", \"not just X, but Y\" used as a habit: phrases models use many times more often than people." + why},
		{ID: RuleCorrective, Summary: "No correcting claims nobody made", Help: "\"It isn't X. It's Y.\" sets up a belief no reader holds in order to knock it down: the most frequent structural habit of current models."},
		{ID: RuleSignpost, Summary: "Say it, do not announce it", Help: "\"In this issue we'll explore\", \"let's dive in\", \"here's everything you need to know\": announcements readers skip."},
		{ID: RulePets, Summary: "No pet words repeated", Help: "Words current models repeat where writers vary (reliable, robust, seamless, leverage, quietly): name the fact instead."},
		{ID: RuleBold, Summary: "Bold sparingly", Help: "Bold scattered over a text is a habit of generated writing and stops working as emphasis."},
		{ID: RuleLabels, Summary: "No label-colon bullets", Help: "Lists whose items start with a bold label and a colon read as generated; write sentences or a table."},
		{ID: RuleConnectors, Summary: "No formal connectors", Help: "Moreover, Furthermore, Additionally, Darüber hinaus at the start of sentences: people write and, but, so, or nothing."},
		{ID: RuleParticiples, Summary: "No trailing participle comments", Help: "\", highlighting …\", \", underscoring …\" at the end of sentences comment on the point instead of making it (Reinhart et al., PNAS 2025)."},
		{ID: RuleHedges, Summary: "Commit instead of hedging", Help: "\"May help\", \"can potentially\": say when it works and when it does not."},
		{ID: RuleStockNames, Summary: "No stock names of invented examples", Help: "Sarah, Alex and Maria are what models name invented people. Use a real case with its source, or say it is made up. Never invent the owner's experiences."},
		{ID: RuleSources, Summary: "Claims name their source", Help: "\"Studies show\", \"experts say\", \"research suggests\" without a link attribute a claim to nobody."},
		{ID: RuleArtifacts, Summary: "No chat-tool leftovers", Help: "oaicite markers, turn0search0 references and utm_source=chatgpt links are left over from copying a chat answer."},
		{ID: RuleOpening, Summary: "Open inside the matter", Help: "An issue that opens with \"Imagine …\" or a generalization without a name, number or quote loses readers in the preview line."},
		{ID: RuleEnding, Summary: "End on something useful", Help: "Recaps (\"In short\", \"To sum up\") and pep talks at the end are skipped; end on the last useful thing or a question readers can answer (lfr:E144)."},
		{ID: RuleSpecifics, Summary: "Names, numbers and quotes carry the text", Help: "Generic writing has too few proper names, figures and quoted words per thousand words; readers cite \"3 pages of insights that could be summarized in a sentence\" as a reason to leave (lfr:V006)."},
		{ID: RuleNominal, Summary: "Verbs, not abstract nouns", Help: "A high share of -tion, -ment, -ung, -heit nouns makes text abstract; turn them back into someone doing something."},
		{ID: RuleParagraphs, Summary: "Paragraphs vary and fit a phone", Help: "Feeds of one-line paragraphs, walls of text and paragraphs of identical length are all marks of generated or careless writing (lfr:E085)."},
		{ID: RuleFragments, Summary: "Few sentence fragments", Help: "Fragments land when rare; many of them read as a style imitation."},
		{ID: RuleReadability, Summary: "Easy to read", Help: "Flesch reading ease (English) or Amstad and the Wiener Sachtextformel (German), and the mean sentence length: newsletters are read on phones between other things (lfr:E123)."},
		{ID: RuleDashes, Summary: "Em dashes in moderation", Help: "Four or more em dashes at more than one per hundred words is a density human copy rarely reaches."},
		{ID: RulePlaceholder, Summary: "No template filler", Help: "lorem ipsum, TODO, [insert …] and \"your text here\" must never reach readers."},
		{ID: RuleRhythm, Summary: "Sentence length follows the thought", Help: "Sentences of nearly the same length are the most measurable habit of generated prose; people write long where they explain and short where it lands."},
		{ID: RuleOpeners, Summary: "Paragraphs begin from what they are about", Help: "Many paragraphs opening with the same two words read as a template."},
		{ID: RuleQuestions, Summary: "Answers, not rhetorical questions", Help: "Paragraphs that open with a question the reader did not ask are a habit of generated copy; answer instead."},
		{ID: RuleRepeat, Summary: "No passage repeats an earlier issue", Help: "Twelve words in a row, or a paragraph sharing a third of its word sequences with an earlier issue, means the point was made before; readers notice."},
	}
}

// Earlier is a text already published, for the repetition check.
type Earlier struct {
	File     string
	Markdown string
}

// Text is one piece of writing.
type Text struct {
	File     string // for findings
	Markdown string // without front matter
	Language string // BCP 47, e.g. en or de
	Earlier  []Earlier
}

// Check reports what in the text falls below the floor. Every finding is a
// warning, except template filler.
func Check(t Text) []finding.Finding {
	var out []finding.Finding
	add := func(sev, rule string, line int, message, fix string) {
		out = append(out, finding.Finding{Rule: rule, Severity: sev, File: t.File, Line: line, Message: message, Fix: fix})
	}
	warn := func(rule string, line int, message, fix string) { add(finding.Warning, rule, line, message, fix) }
	_, p := parseChapter([]byte(t.Markdown))
	q := measure(p, t.Language)
	report(q, p, t.Language, warn)

	texts := blockTexts(p)
	for _, h := range FindTells(texts) {
		line := 1
		if h.Segment >= 0 && h.Segment < len(p.blocks) {
			line = p.blocks[h.Segment].line
		}
		msg := fmt.Sprintf("%q is a stock phrase of generated copy.", h.Phrase)
		switch {
		case h.Repeat:
			msg = fmt.Sprintf("%q and %d more like it: a construction generated copy repeats.", h.Phrase, h.Count-1)
		case h.Count > 1:
			msg = fmt.Sprintf("%q appears %d times: a stock phrase of generated copy.", h.Phrase, h.Count)
		}
		warn(RuleTells, line, msg, "Say the concrete thing instead: the fact, the situation, the number it stands for.")
	}
	dashes, words, dashLine := 0, 0, 0
	for _, b := range p.blocks {
		if b.quote {
			continue
		}
		words += len(strings.Fields(b.text))
		if n := ProseDashes(b.text); n > 0 {
			if dashLine == 0 {
				dashLine = b.line
			}
			dashes += n
		}
		if m := Placeholder(b.text); m != "" {
			add(finding.Error, RulePlaceholder, b.line, fmt.Sprintf("Template filler %q.", m), "Replace it with the real content.")
		}
	}
	if dashes >= minDashes && float64(dashes)*100 > dashesPer100*float64(words) {
		warn(RuleDashes, dashLine, fmt.Sprintf("%d em dashes in %d words.", dashes, words), "Recast most of them as two sentences, a comma, a colon or parentheses; keep the few where the break is the point.")
	}
	shape(p, warn)
	if len(t.Earlier) > 0 {
		texts := []chapterText{{file: t.File, blocks: p.blocks}}
		for _, e := range t.Earlier {
			_, ep := parseChapter([]byte(e.Markdown))
			texts = append([]chapterText{{file: e.File, blocks: ep.blocks}}, texts...)
		}
		repetition(texts, func(file string, line int, message, fix string) {
			if file == t.File {
				warn(RuleRepeat, line, message, fix)
			}
		})
	}
	finding.Sort(out)
	return dedupe(out)
}

// dedupe drops a finding that quotes the same phrase on the same line as one
// before it: "Let's dive in" is a stock phrase and an announcement, but the
// writer needs to hear it once.
func dedupe(fs []finding.Finding) []finding.Finding {
	quoted := func(f finding.Finding) string {
		_, rest, ok := strings.Cut(f.Message, `"`)
		if !ok {
			return ""
		}
		q, _, _ := strings.Cut(rest, `"`)
		return strings.ToLower(strings.ReplaceAll(q, "’", "'"))
	}
	out := fs[:0]
	var kept []finding.Finding
	for _, f := range fs {
		q := quoted(f)
		dup := false
		for _, k := range kept {
			if kq := quoted(k); q != "" && kq != "" && k.Line == f.Line && (strings.Contains(kq, q) || strings.Contains(q, kq)) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, f)
			kept = append(kept, f)
		}
	}
	return out
}

// shape checks how sentences and paragraphs are built.
func shape(p prose, warn func(rule string, line int, message, fix string)) {
	if lengths := sentenceLengths(p); len(lengths) >= minSentencesForRhythm {
		if v := variation(lengths); v < uniformRhythm {
			warn(RuleRhythm, 1, fmt.Sprintf("Sentences barely vary in length (variation %.2f over %d sentences).", v, len(lengths)), "Mix short and long sentences the way the thought moves; uniform rhythm reads as generated.")
		}
	}
	questions, paragraphs := 0, 0
	openers := map[string][]int{}
	for _, b := range p.blocks {
		if b.quote {
			continue
		}
		for _, re := range bookTells {
			if m := re.FindString(b.text); m != "" {
				warn(RuleSignpost, b.line, fmt.Sprintf("%q is a stock line of generated writing.", m), "Start with the thing itself; readers do not need it announced or framed.")
				break
			}
		}
		if m := unsourced.FindString(b.text); m != "" && !b.link {
			warn(RuleSources, b.line, fmt.Sprintf("%q makes a claim without a source in the paragraph.", m), "Link the study, survey or source where the claim is made, name it, or cut the claim.")
		}
		if b.list {
			continue
		}
		paragraphs++
		if first := sentenceEnd.FindStringIndex(b.text); first != nil && strings.Contains(b.text[first[0]:first[1]], "?") {
			questions++
		}
		if o := opener(b.text); o != "" {
			openers[o] = append(openers[o], b.line)
		}
	}
	if questions >= 3 && float64(questions) > 0.2*float64(paragraphs) {
		warn(RuleQuestions, 1, fmt.Sprintf("%d paragraphs open with a question.", questions), "Answer instead of asking; keep a question where the reader would really ask it.")
	}
	for o, at := range openers {
		if len(at) >= minOpenerRepeats && float64(len(at)) > openerShare*float64(paragraphs) {
			warn(RuleOpeners, at[0], fmt.Sprintf("%d paragraphs open with %q.", len(at), o), "Vary how paragraphs begin; start from what the paragraph is about.")
		}
	}
}
