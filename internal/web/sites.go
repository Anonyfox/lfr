package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfr/internal/i18n"
)

//go:embed templates/*.html
var defaults embed.FS

// Pages a site renders; each is its own template set.
var Pages = []string{"home", "archive", "issue", "message"}

// shared files every page set includes.
var shared = []string{"layout", "signup"}

// Sites loads the templates of each list: the embedded defaults, with any
// file of the same name in <dir>/<list-slug>/ taking its place. custom.html
// there is added to every page (to redefine "style", for example), and
// email.html replaces the email layout. Edits are picked up within seconds.
type Sites struct {
	Dir string

	mu    sync.Mutex
	cache map[string]*siteSet
}

type siteSet struct {
	pages   map[string]*template.Template
	email   *template.Template
	stamp   string
	checked time.Time
}

var funcs = template.FuncMap{"t": i18n.T}

// stamp fingerprints a site directory by names, sizes and times.
func stamp(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, e := range entries {
		if info, err := e.Info(); err == nil && strings.HasSuffix(e.Name(), ".html") {
			fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

func (s *Sites) dirOf(slug string) string { return filepath.Join(s.Dir, slug) }

func (s *Sites) load(slug string) (*siteSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]*siteSet{}
	}
	cur := s.cache[slug]
	if cur != nil && time.Since(cur.checked) < 2*time.Second {
		return cur, nil
	}
	dir := s.dirOf(slug)
	st := stamp(dir)
	if cur != nil && cur.stamp == st {
		cur.checked = time.Now()
		return cur, nil
	}
	set, err := build(dir)
	if err != nil {
		if cur != nil {
			// Keep serving the last good version while an edit is broken.
			cur.checked = time.Now()
			return cur, err
		}
		set, _ = build("")
	}
	set.stamp, set.checked = st, time.Now()
	s.cache[slug] = set
	return set, err
}

func read(dir, name string) (string, bool) {
	if dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, name+".html")); err == nil {
			return string(b), true
		}
	}
	b, err := fs.ReadFile(defaults, "templates/"+name+".html")
	if err != nil {
		return "", false
	}
	return string(b), true
}

func build(dir string) (*siteSet, error) {
	set := &siteSet{pages: map[string]*template.Template{}}
	custom, hasCustom := "", false
	if dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "custom.html")); err == nil {
			custom, hasCustom = string(b), true
		}
	}
	for _, page := range Pages {
		t := template.New("lfr:" + page).Funcs(funcs) // never a file name: reusing the root name drops definitions
		for _, name := range append(append([]string{}, shared...), page) {
			src, _ := read(dir, name)
			if _, err := t.New(name).Parse(src); err != nil {
				return nil, fmt.Errorf("%s.html: %w", name, err)
			}
		}
		if hasCustom {
			if _, err := t.New("custom").Parse(custom); err != nil {
				return nil, fmt.Errorf("custom.html: %w", err)
			}
		}
		set.pages[page] = t
	}
	if dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "email.html")); err == nil {
			t, err := template.New("email").Parse(string(b))
			if err != nil {
				return nil, fmt.Errorf("email.html: %w", err)
			}
			set.email = t
		}
	}
	return set, nil
}

// Page returns the template set of a page for a list.
func (s *Sites) Page(slug, page string) (*template.Template, error) {
	set, err := s.load(slug)
	if set == nil {
		return nil, err
	}
	return set.pages[page], nil
}

// Email returns a list's custom email layout, or nil for the built-in one.
func (s *Sites) Email(slug string) *template.Template {
	set, _ := s.load(slug)
	if set == nil {
		return nil
	}
	return set.email
}

// Check parses a list's templates and reports the first error.
func (s *Sites) Check(slug string) error {
	_, err := build(s.dirOf(slug))
	return err
}

// Templates are the built-in templates by file name, for editing on the
// machine where the agent works (lfr site <list> --eject).
func Templates() map[string]string {
	out := map[string]string{}
	for _, name := range append(append([]string{}, shared...), Pages...) {
		src, _ := read("", name)
		out[name+".html"] = src
	}
	return out
}

// Limits of an uploaded site.
const (
	maxSiteFile  = 2 << 20
	maxSiteTotal = 20 << 20
)

var siteAssetExt = map[string]bool{".css": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true, ".webp": true, ".woff": true, ".woff2": true, ".txt": true}

// Put replaces a list's templates and assets with files (relative path →
// content). The new set must parse before it goes live, and it replaces the
// old one in a single rename, so readers never see a half-written site.
func (s *Sites) Put(slug string, files map[string][]byte) error {
	known := map[string]bool{"custom.html": true, "email.html": true}
	for name := range Templates() {
		known[name] = true
	}
	total := 0
	for name, data := range files {
		clean := filepath.ToSlash(filepath.Clean(name))
		switch {
		case clean != name || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/"):
			return fmt.Errorf("%s: not a plain relative path", name)
		case strings.HasPrefix(clean, "assets/"):
			if !siteAssetExt[strings.ToLower(filepath.Ext(clean))] || strings.Count(clean, "/") > 3 {
				return fmt.Errorf("%s: assets are stylesheets, images and fonts", name)
			}
		case !known[clean]:
			return fmt.Errorf("%s: not a site template; they are layout, signup, home, archive, issue, message, custom and email (.html) and files in assets/", name)
		}
		if len(data) > maxSiteFile {
			return fmt.Errorf("%s: %d KB; at most %d KB per file", name, len(data)>>10, maxSiteFile>>10)
		}
		total += len(data)
	}
	if total > maxSiteTotal {
		return fmt.Errorf("the site is %d MB; at most %d MB", total>>20, maxSiteTotal>>20)
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(s.Dir, "."+slug+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for name, data := range files {
		p := filepath.Join(tmp, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return err
		}
	}
	if _, err := build(tmp); err != nil {
		return err
	}
	dir := s.dirOf(slug)
	old := dir + ".old"
	os.RemoveAll(old)
	if err := os.Rename(dir, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return err
	}
	os.RemoveAll(old)
	s.mu.Lock()
	delete(s.cache, slug)
	s.mu.Unlock()
	return nil
}

// Asset resolves a file under a list's assets directory, refusing paths that escape it.
func (s *Sites) Asset(slug, name string) (string, bool) {
	root := filepath.Join(s.dirOf(slug), "assets")
	clean := filepath.Clean("/" + name)
	p := filepath.Join(root, clean)
	if !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return "", false
	}
	return p, true
}
