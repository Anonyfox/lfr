package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/web"
)

func runSiteEject(c *call) error {
	if err := need(c, 1); err != nil {
		return err
	}
	dir := "site"
	if len(c.args) > 1 {
		dir = c.args[1]
	}
	var written []string
	for name, src := range web.Templates() {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			continue // keep edits
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			return err
		}
		written = append(written, p)
	}
	sort.Strings(written)
	return c.result(map[string]any{"dir": dir, "written": written}, func() {
		for _, f := range written {
			c.printf("  wrote %s\n", f)
		}
		c.next("edit them (assets in %s/assets/), then lfr site %s %s uploads them", dir, c.args[0], dir)
	})
}

// siteFiles reads a local site folder for upload: templates and assets/.
func siteFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && rel != "assets" && !strings.HasPrefix(rel, "assets/") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || (!strings.HasPrefix(rel, "assets/") && !strings.HasSuffix(rel, ".html")) {
			return nil
		}
		b, err := os.ReadFile(p)
		files[rel] = b
		return err
	})
	return files, err
}

func runSitePut(c *call) error {
	files, err := siteFiles(c.args[1])
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return usage("%s holds no templates; lfr site %s --eject %s copies them out", c.args[1], c.args[0], c.args[1])
	}
	var res map[string]any
	if err := c.do(api.OpSitePut, api.SitePut{List: c.args[0], Files: files}, &res); err != nil {
		return err
	}
	return c.result(res, func() {
		c.printf("Uploaded %s to %s; it is live now\n", count(len(files), "file", "files"), c.args[0])
	})
}

func runSiteCheck(c *call) error {
	if err := exactly(c, 1); err != nil {
		return err
	}
	if err := c.do(api.OpSiteCheck, api.Name{Name: c.args[0]}, nil); err != nil {
		return err
	}
	return c.result(map[string]string{"status": "ok"}, func() { c.printf("Templates of %s parse cleanly\n", c.args[0]) })
}

func runSite(c *call) error {
	switch {
	case c.has("eject"):
		return runSiteEject(c)
	case len(c.args) == 2:
		return runSitePut(c)
	}
	return runSiteCheck(c)
}
