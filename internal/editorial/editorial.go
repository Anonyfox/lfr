// Package editorial checks the words of an issue: subject, preheader, length,
// links, images, tone and the brief's own phrases. The server runs it on
// every issue before sending (a floor no newsletter goes below); a newsletter
// project runs it with its concept, so each issue is also held to its brief.
// Rules report, they never rewrite.
package editorial

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/finding"
	"github.com/Anonyfox/lfr/internal/prose"
)

// Brief is what a concept says about each issue; zero values mean "no brief".
type Brief struct {
	MaxSubject int
	MinWords   int
	MaxWords   int
	Use        []string
	Avoid      []string
}

// Issue is the text to check.
type Issue struct {
	File      string // for findings; empty on the server
	Subject   string
	Preheader string
	Markdown  string
	Language  string
	Earlier   []prose.Earlier // issues already sent, for the repetition check
}

// Defaults when there is no brief.
const (
	DefaultMaxSubject = 60
	MaxPreheader      = 90 // characters a preview line shows (lfr:E126)
	HardMaxSubject    = 90
	DefaultMaxWords   = 3000
)

// Rule IDs.
const (
	SubjectMissing     = "subject.missing"
	SubjectLength      = "subject.length"
	SubjectCaps        = "subject.caps"
	SubjectEmoji       = "subject.emoji"
	SubjectPunctuation = "subject.punctuation"
	SubjectDeceptive   = "subject.deceptive"
	PreheaderMissing   = "preheader.missing"
	PreheaderLength    = "preheader.length"
	PreheaderRepeat    = "preheader.repeat"
	LengthWords        = "length.words"
	LinksText          = "links.text"
	LinksCount         = "links.count"
	ImagesAlt          = "images.alt"
	ImagesOnly         = "images.only"
	Personalization    = "copy.tokens"
	CopyAvoid          = "copy.avoid"
	CopyUse            = "copy.use"
	CopySpammy         = "copy.spammy"
	CopyShouting       = "copy.shouting"
	ContentTeaser      = "content.teaser"
	StructureHeadings  = "structure.headings"
	StructureReply     = "structure.reply"
)

// Rules describes every editorial rule, the prose rules included.
func Rules() []finding.Rule {
	return append(issueRules(), prose.Rules()...)
}

func issueRules() []finding.Rule {
	return []finding.Rule{
		{ID: SubjectMissing, Summary: "An issue has a subject", Help: "Without a subject the inbox shows nothing to decide on. Write one that says what is inside."},
		{ID: SubjectLength, Summary: "The subject fits a phone's inbox line", Help: "Mobile inboxes show 25 to 40 characters of a subject, desktops about 60 (lfr:E088, lfr:E127); short subjects get the most opens (lfr:E106). The brief's Max: (default 60) keeps the point visible. Put the key words first."},
		{ID: SubjectCaps, Summary: "The subject is not shouted", Help: "An all-capitals subject reads as spam to readers and filters. Write it in normal case."},
		{ID: SubjectEmoji, Summary: "The subject has no emoji", Help: "Emoji subjects raise negative reactions by 26% without lifting opens (lfr:E089) and open less in sender data (lfr:E106); readers file them with spam (lfr:V008). Google warns against characters that imitate graphics (lfr:E006). Use words."},
		{ID: SubjectPunctuation, Summary: "The subject has at most three punctuation marks and none repeated", Help: "Repeated marks, dollar signs and more than three punctuation marks look like spam (lfr:E079). One mark at most."},
		{ID: SubjectDeceptive, Summary: "The subject does not pretend to be a reply or forward", Help: "Re: and Fwd: on a newsletter deceive readers; Google's guidelines forbid them (lfr:E006), CAN-SPAM and German law forbid deceptive subjects (lfr:E046, lfr:E045). Drop the prefix."},
		{ID: PreheaderMissing, Summary: "An issue has a preheader", Help: "Readers decide on subject plus preview (lfr:E087); without a preheader inboxes show the first words of the body. Write a sentence that continues the subject, under 90 characters (lfr:E126). lfr shows it as visible text at the top, never hidden (lfr:E006)."},
		{ID: PreheaderLength, Summary: "The preheader fits the preview line", Help: "Inboxes show anywhere from none to about 278 characters of preview; Litmus recommends staying under 90 (lfr:E067, lfr:E126). Cut it to one short sentence."},
		{ID: PreheaderRepeat, Summary: "The preheader adds to the subject", Help: "A preheader that repeats the subject wastes the second line of the inbox (lfr:E085). Say what the subject leaves out."},
		{ID: LengthWords, Summary: "The issue's length fits the brief", Help: "The concept sets Words: <min>-<max> from what readers asked for; NN/g finds shorter is better, especially on phones (lfr:E085). An issue far outside the range breaks the promise readers subscribed to."},
		{ID: LinksText, Summary: "Link texts say where they go", Help: "\"Click here\", \"read more\" and bare URLs tell screen-reader users and skimmers nothing; 99.59% of audited beehiiv emails fail this (lfr:E063), and Google wants links readers can understand (lfr:E006). Link the words that name the destination."},
		{ID: LinksCount, Summary: "An issue does not drown in links", Help: "Dozens of links make an issue hard to scan; experts advise a few clear calls to action (lfr:E133). Keep the links that carry the issue."},
		{ID: ImagesAlt, Summary: "Images have alt text", Help: "Classic Outlook blocks images by default (lfr:E075); 47.88% of emails lack alt text (lfr:E065), and readers with images off see nothing (lfr:V041). Describe what the image shows."},
		{ID: ImagesOnly, Summary: "Text carries the message, not images", Help: "Image-only mail says nothing with images off (lfr:E121), scores as spam (lfr:E072) and annoys readers (lfr:V042). Write the message as text."},
		{ID: Personalization, Summary: "No merge tokens", Help: "lfr does not fill {first_name} tokens: the effect does not replicate (lfr:E138, lfr:E107) and readers find fake familiarity rude (lfr:V014). Write to readers without a mail-merge greeting."},
		{ID: CopyAvoid, Summary: "The issue avoids the words the brief forbids", Help: "The concept's Avoid: list holds words readers dislike or that do not fit the newsletter. Replace them."},
		{ID: CopyUse, Summary: "The issue speaks the readers' language", Help: "The concept's Use: list holds readers' own words. An issue using none of them drifts away from how readers talk."},
		{ID: CopySpammy, Summary: "No sales phrases spam filters weigh against you", Help: "Persuasive sales tactics frustrate readers (lfr:E140) and phrases like \"act now\" read as spam. Say it plainly."},
		{ID: CopyShouting, Summary: "No shouted paragraphs", Help: "Long runs of capitals read as shouting and as spam. Use emphasis sparingly."},
		{ID: ContentTeaser, Summary: "The content lives in the email", Help: "Readers resent teasers that send them elsewhere (lfr:V017, lfr:V052); good newsletters carry the substance in the email. Link for depth."},
		{ID: StructureHeadings, Summary: "Headings form an outline", Help: "One top heading and ordered subheadings let screen readers and skimmers navigate; 74% of emails lack a level-one heading (lfr:E064). Do not skip levels."},
		{ID: StructureReply, Summary: "The issue invites a reply", Help: "Replies are the honest engagement signal (lfr:E112) and readers reply when someone answers (lfr:V112, lfr:V024). Ask one question near the end; questions lift responses (lfr:E144)."},
	}
}

var (
	emojiRE    = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{2B00}-\x{2BFF}\x{1F900}-\x{1F9FF}\x{FE0F}]`)
	punctRE    = regexp.MustCompile(`[!?]{2,}|\${2,}|!\s*!`)
	deceptive  = regexp.MustCompile(`(?i)^\s*(re|fwd?|aw|wg)\s*:`)
	tokenRE    = regexp.MustCompile(`\{\{\s*\w+\s*\}\}|\{\s*(first_?name|name|vorname)\s*\}|\*\|[A-Z_]+\|\*|%(first_?name|name)%`)
	wordRE     = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'’-]*`)
	spammy     = []string{"100% free", "act now", "risk-free", "risk free", "winner", "guaranteed", "no obligation", "cash bonus", "$$$", "earn money", "buy now", "limited time offer", "click below", "you have been selected", "once in a lifetime"}
	vagueLinks = []string{"here", "click here", "click", "read more", "more", "this", "link", "learn more", "this link", "go", "continue", "hier", "hier klicken", "klicken sie hier", "klick hier", "mehr", "weiterlesen", "mehr lesen", "dieser link", "→", ">>"}
	replyWords = []string{"reply", "hit reply", "write back", "tell me", "let me know", "antworte", "schreib mir", "schreibt mir", "erzähl", "verrat mir"}
)

type checker struct {
	w     Issue
	out   []finding.Finding
	brief Brief
}

func (c *checker) add(rule, sev, msg, fix string) {
	c.out = append(c.out, finding.Finding{Rule: rule, Severity: sev, File: c.w.File, Message: msg, Fix: fix})
}

// Check runs every editorial rule on an issue.
func Check(w Issue, b Brief) []finding.Finding {
	c := &checker{w: w, brief: b}
	c.subject()
	c.preheader()
	doc, err := emailmd.Render(w.Markdown)
	if err != nil {
		c.add(LengthWords, finding.Error, "The Markdown does not render: "+err.Error(), "Fix the Markdown syntax.")
		return c.out
	}
	text := emailmd.Text(w.Markdown)
	words := wordRE.FindAllString(text, -1)
	c.length(len(words))
	c.markup(doc.HTML, len(words))
	c.copy(text, words)
	finding.Sort(c.out)
	return c.out
}

func (c *checker) subject() {
	s := strings.TrimSpace(c.w.Subject)
	if s == "" {
		c.add(SubjectMissing, finding.Error, "The issue has no subject.", "Add subject: to the front matter or a first heading.")
		return
	}
	max := c.brief.MaxSubject
	if max == 0 {
		max = DefaultMaxSubject
	}
	n := utf8.RuneCountInString(s)
	switch {
	case n > HardMaxSubject:
		c.add(SubjectLength, finding.Error, fmt.Sprintf("The subject has %d characters; inboxes cut it long before that.", n), "Put the point in the first 40 characters and cut the rest.")
	case n > max:
		c.add(SubjectLength, finding.Warning, fmt.Sprintf("The subject has %d characters; the brief allows %d.", n, max), "Shorten it, point first.")
	}
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters >= 8 && upper*10 >= letters*8 {
		c.add(SubjectCaps, finding.Warning, "The subject is in capitals.", "Write it in normal case.")
	}
	if emojiRE.MatchString(s) {
		c.add(SubjectEmoji, finding.Warning, "The subject has an emoji.", "Say it in words; readers file emoji subjects with spam.")
	}
	// The subject and preheader are the first words a reader sees, and the
	// first place generated copy shows.
	for _, h := range prose.FindTells([]string{s, c.w.Preheader}) {
		where := "subject"
		if h.Segment == 1 {
			where = "preheader"
		}
		c.add(prose.RuleTells, finding.Warning, fmt.Sprintf("The %s uses %q, a stock phrase of generated copy.", where, h.Phrase), "Name what the issue is about in the readers' plain words.")
	}
	marks := 0
	for _, r := range s {
		if strings.ContainsRune("!?.,:;$%&*#", r) {
			marks++
		}
	}
	switch {
	case punctRE.MatchString(s):
		c.add(SubjectPunctuation, finding.Warning, "The subject repeats ! ? or $.", "Use one mark at most.")
	case marks > 3:
		c.add(SubjectPunctuation, finding.Warning, fmt.Sprintf("The subject has %d punctuation marks.", marks), "Use three at most; fewer reads calmer.")
	}
	if deceptive.MatchString(s) {
		c.add(SubjectDeceptive, finding.Error, "The subject pretends to be a reply or forward.", "Remove the Re:/Fwd: prefix.")
	}
}

func (c *checker) preheader() {
	p := strings.TrimSpace(c.w.Preheader)
	if p == "" {
		c.add(PreheaderMissing, finding.Info, "The issue has no preheader.", "Add preheader: to the front matter: one sentence that continues the subject.")
		return
	}
	if n := utf8.RuneCountInString(p); n > MaxPreheader {
		c.add(PreheaderLength, finding.Warning, fmt.Sprintf("The preheader has %d characters; previews show about %d.", n, MaxPreheader), "Cut it to one short sentence.")
	}
	norm := func(s string) string { return strings.ToLower(strings.Join(wordRE.FindAllString(s, -1), " ")) }
	if norm(p) == norm(c.w.Subject) || strings.HasPrefix(norm(p), norm(c.w.Subject)) && norm(c.w.Subject) != "" {
		c.add(PreheaderRepeat, finding.Warning, "The preheader repeats the subject.", "Say what the subject leaves out.")
	}
}

func (c *checker) length(n int) {
	b := c.brief
	switch {
	case b.MaxWords > 0 && n > b.MaxWords*13/10:
		c.add(LengthWords, finding.Warning, fmt.Sprintf("%d words; the brief says %d to %d.", n, b.MinWords, b.MaxWords), "Cut to the brief, or move depth to a linked page.")
	case b.MinWords > 0 && n < b.MinWords*7/10:
		c.add(LengthWords, finding.Warning, fmt.Sprintf("%d words; the brief says %d to %d.", n, b.MinWords, b.MaxWords), "Give readers the substance the brief promises.")
	case b.MaxWords == 0 && n > DefaultMaxWords:
		c.add(LengthWords, finding.Info, fmt.Sprintf("%d words is a long read for an inbox.", n), "Consider splitting it, or set Words: in the concept if long is the point.")
	}
}

type link struct{ href, text string }

func (c *checker) markup(fragment string, words int) {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return
	}
	var links []link
	images, missingAlt := 0, 0
	var levels []int
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.A:
				links = append(links, link{attr(n, "href"), strings.TrimSpace(textOf(n))})
			case atom.Img:
				images++
				if strings.TrimSpace(attr(n, "alt")) == "" {
					missingAlt++
				}
			case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				levels = append(levels, int(n.Data[1]-'0'))
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	vague := 0
	var examples []string
	for _, l := range links {
		t := strings.ToLower(strings.Trim(l.text, " .:!→»>"))
		isURL := strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") || strings.HasPrefix(t, "www.")
		if contains(vagueLinks, t) || isURL && t == strings.ToLower(l.href) {
			vague++
			if len(examples) < 3 {
				examples = append(examples, "\""+l.text+"\"")
			}
		}
	}
	if vague > 0 {
		c.add(LinksText, finding.Warning, fmt.Sprintf("%d links say nothing about where they go: %s.", vague, strings.Join(examples, ", ")), "Link the words that name the destination, e.g. [the full recipe](…).")
	}
	if len(links) > 40 {
		c.add(LinksCount, finding.Warning, fmt.Sprintf("%d links.", len(links)), "Keep the links that carry the issue; move the rest to the archive page.")
	}
	if missingAlt > 0 {
		c.add(ImagesAlt, finding.Warning, fmt.Sprintf("%d of %d images have no alt text.", missingAlt, images), "Describe what each image shows: ![a jar of bubbly starter](…).")
	}
	if images > 0 && words < 60 {
		c.add(ImagesOnly, finding.Warning, fmt.Sprintf("%d words next to %d images.", words, images), "Write the message as text; images support it.")
	}
	h1 := 0
	prev := 0
	skipped := false
	for _, l := range levels {
		if l == 1 {
			h1++
		}
		if prev > 0 && l > prev+1 {
			skipped = true
		}
		prev = l
	}
	if h1 > 1 || skipped {
		c.add(StructureHeadings, finding.Info, "Headings skip levels or repeat the top level.", "Use one # heading, then ## and ### in order.")
	}
	more := 0
	for _, l := range links {
		t := strings.ToLower(l.text)
		if strings.Contains(t, "read more") || strings.Contains(t, "weiterlesen") || strings.Contains(t, "full article") || strings.Contains(t, "continue reading") {
			more++
		}
	}
	if words < 150 && more > 0 {
		c.add(ContentTeaser, finding.Warning, fmt.Sprintf("%d words and %d read-more links: the issue is a teaser.", words, more), "Put the substance in the issue itself; link for depth.")
	}
}

func (c *checker) copy(text string, words []string) {
	low := strings.ToLower(text)
	if tokenRE.MatchString(text) {
		c.add(Personalization, finding.Error, "The issue has a merge token like "+tokenRE.FindString(text)+".", "lfr does not fill tokens; write without one.")
	}
	c.out = append(c.out, prose.Check(prose.Text{File: c.w.File, Markdown: c.w.Markdown, Language: c.w.Language, Earlier: c.w.Earlier})...)
	for _, p := range spammy {
		if strings.Contains(low, p) {
			c.add(CopySpammy, finding.Warning, "The issue says \""+p+"\", which spam filters weigh against you.", "Say it plainly.")
		}
	}
	for _, a := range c.brief.Avoid {
		if a != "" && strings.Contains(low, strings.ToLower(a)) {
			c.add(CopyAvoid, finding.Warning, "The issue says \""+a+"\", which the brief's Avoid: list forbids.", "Replace it with the readers' own words.")
		}
	}
	if len(c.brief.Use) > 0 {
		used := false
		for _, u := range c.brief.Use {
			if strings.Contains(low, strings.ToLower(u)) {
				used = true
			}
		}
		if !used {
			c.add(CopyUse, finding.Info, "The issue uses none of the brief's Use: phrases.", "Where it fits, say it the way readers do: "+strings.Join(quoteAll(c.brief.Use, 4), ", ")+".")
		}
	}
	caps := 0
	for _, w := range words {
		if utf8.RuneCountInString(w) >= 4 && strings.ToUpper(w) == w && strings.ToLower(w) != w {
			caps++
		}
	}
	if len(words) >= 50 && caps*100 > len(words)*8 {
		c.add(CopyShouting, finding.Warning, fmt.Sprintf("%d words in capitals.", caps), "Use emphasis sparingly.")
	}
	tail := low
	if len(tail) > 0 {
		tail = tail[len(tail)*6/10:]
	}
	asks := strings.Contains(tail, "?")
	for _, r := range replyWords {
		if strings.Contains(tail, r) {
			asks = true
		}
	}
	if !asks && len(words) >= 80 {
		c.add(StructureReply, finding.Info, "The issue does not invite a reply.", "End with a question or an invitation to hit reply; replies reach the sender's mailbox.")
	}
}

func quoteAll(l []string, n int) []string {
	var out []string
	for i, s := range l {
		if i >= n {
			break
		}
		out = append(out, "\""+s+"\"")
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func attr(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			b.WriteString(attr(n, "alt"))
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
