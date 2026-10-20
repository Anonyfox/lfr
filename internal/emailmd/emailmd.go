// Package emailmd renders Markdown into what a newsletter needs: email HTML
// with every style inlined (mail clients ignore most stylesheets), clean HTML
// for the web archive, and a readable plain-text part.
package emailmd

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
)

// Doc is rendered Markdown.
type Doc struct {
	HTML    string   // semantic HTML for the web
	Title   string   // text of the first heading
	Summary string   // text of the first paragraph
	Links   []string // http(s) link targets in document order, duplicates kept
}

// Render converts Markdown to HTML and extracts title, summary and links.
func Render(source string) (Doc, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(source), &buf); err != nil {
		return Doc{}, err
	}
	d := Doc{HTML: buf.String()}
	root, err := html.Parse(strings.NewReader("<body>" + d.HTML + "</body>"))
	if err != nil {
		return d, nil
	}
	walk(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.DataAtom {
		case atom.H1, atom.H2:
			if d.Title == "" {
				d.Title = textOf(n)
			}
		case atom.P:
			if d.Summary == "" && !onlyImage(n) {
				d.Summary = textOf(n)
			}
		case atom.A:
			if h := attr(n, "href"); isHTTP(h) {
				d.Links = append(d.Links, h)
			}
		}
	})
	return d, nil
}

// LocalImages returns the image sources of Markdown that are paths, not
// URLs: files next to the Markdown that must be uploaded with it.
func LocalImages(source string) []string {
	var buf bytes.Buffer
	if md.Convert([]byte(source), &buf) != nil {
		return nil
	}
	root, err := html.Parse(strings.NewReader("<body>" + buf.String() + "</body>"))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	walk(root, func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			if src := attr(n, "src"); Local(src) && !seen[src] {
				seen[src] = true
				out = append(out, src)
			}
		}
	})
	return out
}

// Image limits: formats every mail client shows (Outlook shows neither WebP
// nor SVG), small enough that an issue stays quick on a phone.
const (
	MaxImage   = 1 << 20   // bytes
	HeavyImage = 300 << 10 // above this, an image slows the issue down
)

// ImageTypes maps the accepted content types to file extensions.
var ImageTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif"}

// CheckImage validates an image file an issue references: a clean relative
// path and an accepted format within the size limit. It returns the type.
func CheckImage(p string, data []byte) (string, error) {
	if !Local(p) || strings.Contains(p, "\\") || path.Clean(p) != p || strings.HasPrefix(p, "../") || p == ".." {
		return "", fmt.Errorf("image %q: use a plain relative path next to the Markdown", p)
	}
	if len(data) > MaxImage {
		return "", fmt.Errorf("image %s is %d KB; the limit is %d KB: resize or compress it", p, len(data)>>10, MaxImage>>10)
	}
	t := http.DetectContentType(data)
	if _, ok := ImageTypes[t]; !ok {
		return "", fmt.Errorf("image %s is %s; mail clients show JPEG, PNG and GIF (Outlook shows neither WebP nor SVG)", p, t)
	}
	return t, nil
}

// Local reports whether an image source is a relative file path.
func Local(src string) bool {
	if src == "" || strings.HasPrefix(src, "/") || strings.HasPrefix(src, "//") {
		return false
	}
	if i := strings.IndexAny(src, ":/"); i >= 0 && src[i] == ':' {
		return false // a scheme: https:, data:, cid:
	}
	return true
}

// RewriteImages replaces the src of every image in an HTML fragment.
func RewriteImages(fragment string, fn func(src string) string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return fragment
	}
	var b bytes.Buffer
	for _, n := range nodes {
		walk(n, func(c *html.Node) {
			if c.Type == html.ElementNode && c.DataAtom == atom.Img {
				setAttr(c, "src", fn(attr(c, "src")))
			}
		})
		html.Render(&b, n)
	}
	return b.String()
}

func isHTTP(h string) bool {
	l := strings.ToLower(h)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

func walk(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

func onlyImage(n *html.Node) bool {
	return textOf(n) == "" && n.FirstChild != nil
}

func attr(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, k, v string) {
	for i, a := range n.Attr {
		if a.Key == k {
			n.Attr[i].Val = v
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: k, Val: v})
}

// Theme colors and fonts of an email.
type Theme struct {
	Accent string       // links and buttons, e.g. #0f766e
	Font   template.CSS // trusted: set by lfr, never by readers
}

// DefaultTheme reads well everywhere.
var DefaultTheme = Theme{
	Accent: "#0f766e",
	Font:   `-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif`,
}

func styles(t Theme) map[atom.Atom]string {
	return map[atom.Atom]string{
		atom.H1:         "margin:0 0 16px;font-size:28px;line-height:1.25;font-weight:700;color:#18181b;",
		atom.H2:         "margin:28px 0 12px;font-size:22px;line-height:1.3;font-weight:700;color:#18181b;",
		atom.H3:         "margin:24px 0 8px;font-size:18px;line-height:1.35;font-weight:700;color:#18181b;",
		atom.H4:         "margin:20px 0 8px;font-size:16px;line-height:1.4;font-weight:700;color:#18181b;",
		atom.P:          "margin:0 0 16px;",
		atom.A:          "color:" + t.Accent + ";text-decoration:underline;",
		atom.Img:        "display:block;max-width:100%;height:auto;border:0;margin:0 0 16px;",
		atom.Blockquote: "margin:0 0 16px;padding:4px 0 4px 16px;border-left:4px solid #e4e4e7;color:#52525b;",
		atom.Code:       "font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:14px;background:#f4f4f5;padding:2px 4px;border-radius:4px;",
		atom.Pre:        "margin:0 0 16px;padding:12px;background:#f4f4f5;border-radius:6px;font-size:14px;line-height:1.45;white-space:pre-wrap;word-break:break-word;",
		atom.Hr:         "border:0;border-top:1px solid #e4e4e7;margin:28px 0;",
		atom.Ul:         "margin:0 0 16px;padding:0 0 0 24px;",
		atom.Ol:         "margin:0 0 16px;padding:0 0 0 24px;",
		atom.Li:         "margin:0 0 6px;",
		atom.Table:      "border-collapse:collapse;width:100%;margin:0 0 16px;font-size:15px;",
		atom.Th:         "border:1px solid #e4e4e7;padding:6px 10px;text-align:left;background:#fafafa;",
		atom.Td:         "border:1px solid #e4e4e7;padding:6px 10px;text-align:left;",
	}
}

var buttonText = regexp.MustCompile(`\s*(→|->|&rarr;)\s*$`)

// Inline adds inline styles to rendered HTML for email. A paragraph holding
// nothing but one link whose text ends in "→" becomes a button.
func Inline(fragment string, t Theme) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return fragment
	}
	st := styles(t)
	var b bytes.Buffer
	for _, n := range nodes {
		walk(n, func(c *html.Node) {
			if c.Type != html.ElementNode {
				return
			}
			if c.DataAtom == atom.Code && c.Parent != nil && c.Parent.DataAtom == atom.Pre {
				setAttr(c, "style", "font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;")
				return
			}
			if s, ok := st[c.DataAtom]; ok {
				setAttr(c, "style", s+attr(c, "style"))
			}
			if c.DataAtom == atom.P && isButton(c) {
				a := firstElement(c)
				label := strings.TrimSpace(buttonText.ReplaceAllString(textOf(a), ""))
				for a.FirstChild != nil {
					a.RemoveChild(a.FirstChild)
				}
				a.AppendChild(&html.Node{Type: html.TextNode, Data: label + " →"})
				setAttr(a, "style", "display:inline-block;padding:12px 22px;border-radius:6px;background:"+t.Accent+";color:#ffffff;font-weight:600;text-decoration:none;")
				setAttr(a, "class", "lfr-button")
				setAttr(c, "style", "margin:8px 0 24px;")
			}
		})
		html.Render(&b, n)
	}
	return b.String()
}

func firstElement(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

func isButton(p *html.Node) bool {
	var link *html.Node
	for c := p.FirstChild; c != nil; c = c.NextSibling {
		switch {
		case c.Type == html.TextNode && strings.TrimSpace(c.Data) == "":
		case c.Type == html.ElementNode && c.DataAtom == atom.A && link == nil:
			link = c
		default:
			return false
		}
	}
	return link != nil && buttonText.MatchString(textOf(link)) && isHTTP(attr(link, "href"))
}

// MarkButtons gives the button links of a web fragment class="button".
func MarkButtons(fragment string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return fragment
	}
	var b bytes.Buffer
	for _, n := range nodes {
		walk(n, func(c *html.Node) {
			if c.Type == html.ElementNode && c.DataAtom == atom.P && isButton(c) {
				a := firstElement(c)
				setAttr(a, "class", strings.TrimSpace(attr(a, "class")+" button"))
				setAttr(c, "class", "cta")
			}
		})
		html.Render(&b, n)
	}
	return b.String()
}

// RewriteLinks replaces every http(s) href in fragment with fn(index, url),
// counting in document order like Doc.Links.
func RewriteLinks(fragment string, fn func(i int, href string) string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return fragment
	}
	i := 0
	var b bytes.Buffer
	for _, n := range nodes {
		walk(n, func(c *html.Node) {
			if c.Type == html.ElementNode && c.DataAtom == atom.A {
				if h := attr(c, "href"); isHTTP(h) {
					setAttr(c, "href", fn(i, h))
					i++
				}
			}
		})
		html.Render(&b, n)
	}
	return b.String()
}

var (
	mdImage = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	mdLink  = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	mdEmph  = regexp.MustCompile(`(\*\*|__)(\S(?:.*?\S)?)(\*\*|__)`)
	mdHTML  = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
)

// Text turns Markdown into the plain-text part: links become "text (url)",
// images their alt text, emphasis markers and inline HTML disappear.
func Text(source string) string {
	var out []string
	inFence := false
	for _, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			line = mdImage.ReplaceAllString(line, "[$1]")
			line = mdLink.ReplaceAllStringFunc(line, func(m string) string {
				sub := mdLink.FindStringSubmatch(m)
				text := strings.TrimSpace(buttonText.ReplaceAllString(sub[1], ""))
				if text == sub[2] {
					return sub[2]
				}
				return text + " (" + sub[2] + ")"
			})
			line = mdEmph.ReplaceAllString(line, "$2")
			line = mdHTML.ReplaceAllString(line, "")
			if strings.HasPrefix(line, "#") {
				line = strings.TrimSpace(strings.TrimLeft(line, "#"))
				if line != "" {
					line = strings.ToUpper(line[:1]) + line[1:]
				}
			}
		}
		out = append(out, line)
	}
	s := strings.Join(out, "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// Layout is the frame around a newsletter.
type Layout struct {
	Lang       string
	Title      string // subject
	Preheader  string // preview text shown by inboxes after the subject
	Brand      string // list name, shown above the content
	BrandURL   string
	Body       template.HTML // inlined content
	FooterHTML template.HTML // unsubscribe, address, legal links
	Pixel      string        // open-tracking image URL, empty when off
	Theme      Theme
}

// The layout follows what readers and mail clients need: the preheader is
// visible text, not hidden markup (Gmail flags hidden content, lfr:E006);
// text is 16px with 1.6 line height and footers 14px (lfr:E066, lfr:E124);
// muted colors keep 7:1 contrast (lfr:E061); content carries lang and dir
// because clients drop them from <html> (lfr:E064); layout tables are
// role="presentation"; the card is #fffffe, which Apple Mail does not invert
// like pure white (lfr:E093).
var layout = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html lang="{{.Lang}}" dir="ltr" xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="supported-color-schemes" content="light dark">
<title>{{.Title}}</title>
<style>
body{margin:0;padding:0;-webkit-text-size-adjust:100%}
@media (prefers-color-scheme:dark){
.lfr-bg{background:#18181b!important}
.lfr-card{background:#27272a!important;color:#e4e4e7!important}
.lfr-card h1,.lfr-card h2,.lfr-card h3,.lfr-card h4{color:#fafafa!important}
.lfr-card pre,.lfr-card code{background:#3f3f46!important}
.lfr-muted,.lfr-muted a{color:#d4d4d8!important}
}
@media (max-width:620px){.lfr-card{padding:24px 18px!important}}
</style>
</head>
<body class="lfr-bg" style="margin:0;padding:0;background:#f4f4f5;">
<div lang="{{.Lang}}" dir="ltr">
<table role="presentation" class="lfr-bg" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#f4f4f5;">
<tr><td align="center" style="padding:24px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:600px;">
{{if .Preheader}}<tr><td class="lfr-muted" style="padding:0 4px 8px;font-family:{{.Theme.Font}};font-size:14px;line-height:1.5;color:#52525b;">{{.Preheader}}</td></tr>{{end}}
{{if .Brand}}<tr><td class="lfr-muted" style="padding:0 4px 12px;font-family:{{.Theme.Font}};font-size:14px;font-weight:600;color:#3f3f46;">{{if .BrandURL}}<a href="{{.BrandURL}}" class="lfr-muted" style="color:#3f3f46;text-decoration:none;">{{.Brand}}</a>{{else}}{{.Brand}}{{end}}</td></tr>{{end}}
<tr><td class="lfr-card" style="background:#fffffe;border-radius:8px;padding:32px;font-family:{{.Theme.Font}};font-size:16px;line-height:1.6;color:#27272a;text-align:left;">
{{.Body}}
</td></tr>
<tr><td class="lfr-muted" style="padding:20px 4px;font-family:{{.Theme.Font}};font-size:14px;line-height:1.5;color:#52525b;">
{{.FooterHTML}}
</td></tr>
</table>
</td></tr>
</table>
</div>
{{if .Pixel}}<img src="{{.Pixel}}" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0;">{{end}}
</body>
</html>`))

// Email renders the full email document.
func Email(l Layout) (string, error) {
	if l.Theme.Accent == "" {
		l.Theme = DefaultTheme
	}
	if l.Lang == "" {
		l.Lang = "en"
	}
	var b bytes.Buffer
	if err := layout.Execute(&b, l); err != nil {
		return "", fmt.Errorf("email layout: %w", err)
	}
	return b.String(), nil
}

// FrontMatter splits optional "---" key: value lines from the body.
func FrontMatter(src string) (map[string]string, string) {
	src = strings.TrimPrefix(strings.ReplaceAll(src, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(src, "---\n") {
		return nil, src
	}
	end := strings.Index(src[4:], "\n---")
	if end < 0 {
		return nil, src
	}
	meta := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(src[4 : 4+end]))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok {
			meta[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	body := src[4+end+4:]
	return meta, strings.TrimLeft(body, "\n")
}
