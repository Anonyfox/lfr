package project

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/finding"
)

// Draft is an issue on disk: issues/<slug>.md, or welcome.md.
//
//	---
//	subject: Your starter is hungry, not dead
//	preheader: Two feeds a day fix the smell
//	idea: I007
//	---
//	Body in Markdown…
type Draft struct {
	File      string `json:"file"`
	Slug      string `json:"slug"`
	Subject   string `json:"subject"`
	Preheader string `json:"preheader,omitempty"`
	Idea      string `json:"idea,omitempty"`
	Welcome   bool   `json:"welcome,omitempty"`
	Source    string `json:"-"` // the whole file, as the server receives it
	Markdown  string `json:"-"` // the body after the front matter
}

// ParseDraft reads an issue file the way the server will.
func ParseDraft(rel, src string) Draft {
	meta, body := emailmd.FrontMatter(src)
	d := Draft{File: rel, Source: src, Markdown: body, Subject: meta["subject"], Preheader: meta["preheader"],
		Idea: strings.ToUpper(strings.TrimSpace(meta["idea"])), Slug: strings.ToLower(meta["slug"])}
	if d.Slug == "" {
		d.Slug = strings.TrimSuffix(path.Base(filepath.ToSlash(rel)), ".md")
	}
	if d.Subject == "" {
		if doc, err := emailmd.Render(body); err == nil {
			d.Subject = doc.Title
		}
	}
	return d
}

// Drafts reads issues/*.md in name order.
func Drafts(dir string) ([]Draft, error) {
	entries, err := os.ReadDir(filepath.Join(dir, IssuesDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []Draft
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, IssuesDir, n))
		if err != nil {
			return nil, err
		}
		out = append(out, ParseDraft(IssuesDir+"/"+n, string(b)))
	}
	return out, nil
}

// BodyLine is the file line where the body after the front matter begins.
func (d Draft) BodyLine() int {
	if !strings.HasSuffix(d.Source, d.Markdown) {
		return 1
	}
	return strings.Count(d.Source[:len(d.Source)-len(d.Markdown)], "\n") + 1
}

// inFile moves findings about d's body to the lines of its file.
func (d Draft) inFile(fs []finding.Finding) []finding.Finding {
	shift := d.BodyLine() - 1
	for i := range fs {
		if fs[i].File == d.File && fs[i].Line > 0 {
			fs[i].Line += shift
		}
	}
	return fs
}

// Same reports whether the server's copy carries this content.
func (d Draft) Same(w *RemoteIssue) bool {
	return w != nil && w.Subject == d.Subject && w.Preheader == d.Preheader && w.Markdown == d.Markdown
}
