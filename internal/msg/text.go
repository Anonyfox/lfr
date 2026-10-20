package msg

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// HTMLToText renders HTML as plain text: blocks become paragraphs, list items
// get dashes, links keep their target when it differs from their text, and
// scripts, styles and hidden preheaders disappear.
func HTMLToText(src string) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return clean(src)
	}
	var b strings.Builder
	var walk func(n *html.Node)
	newline := func(count int) {
		s := b.String()
		trail := len(s) - len(strings.TrimRight(s, "\n"))
		for ; trail < count && b.Len() > 0; trail++ {
			b.WriteByte('\n')
		}
	}
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			t := strings.Join(strings.Fields(n.Data), " ")
			if t == "" {
				if strings.TrimSpace(n.Data) == "" && len(n.Data) > 0 && b.Len() > 0 && !strings.HasSuffix(b.String(), " ") && !strings.HasSuffix(b.String(), "\n") {
					b.WriteByte(' ')
				}
				return
			}
			if (n.Data[0] == ' ' || n.Data[0] == '\n' || n.Data[0] == '\t') && b.Len() > 0 && !strings.HasSuffix(b.String(), " ") && !strings.HasSuffix(b.String(), "\n") {
				b.WriteByte(' ')
			}
			b.WriteString(t)
			if last := n.Data[len(n.Data)-1]; last == ' ' || last == '\n' || last == '\t' {
				b.WriteByte(' ')
			}
			return
		case html.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Head, atom.Title, atom.Template, atom.Noscript:
				return
			case atom.Br:
				b.WriteByte('\n')
				return
			case atom.Img:
				if alt := attr(n, "alt"); alt != "" {
					b.WriteString("[" + alt + "]")
				}
				return
			}
			if hidden(n) {
				return
			}
		}
		block := n.Type == html.ElementNode && isBlock(n.DataAtom)
		if block {
			newline(2)
		}
		if n.DataAtom == atom.Li {
			newline(1)
			b.WriteString("- ")
		}
		start := b.Len()
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.DataAtom == atom.A {
			href := attr(n, "href")
			text := strings.TrimSpace(b.String()[start:])
			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "mailto:") && text != href && !strings.Contains(text, href) {
				b.WriteString(" (" + href + ")")
			}
		}
		if block {
			newline(2)
		}
	}
	walk(doc)
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	out := strings.Join(lines, "\n")
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return clean(strings.TrimSpace(out))
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hidden(n *html.Node) bool {
	style := strings.ReplaceAll(strings.ToLower(attr(n, "style")), " ", "")
	return strings.Contains(style, "display:none") || attr(n, "hidden") != "" || attr(n, "aria-hidden") == "true" && strings.Contains(style, "max-height:0")
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol,
		atom.Table, atom.Tr, atom.Blockquote, atom.Pre, atom.Hr, atom.Section, atom.Article,
		atom.Header, atom.Footer, atom.Main, atom.Nav, atom.Aside, atom.Figure, atom.Form, atom.Body:
		return true
	}
	return false
}
