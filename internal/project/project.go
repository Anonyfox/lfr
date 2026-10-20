// Package project is a newsletter's editorial workspace on disk: research,
// the concept, the backlog of ideas, issue drafts and what each sent issue
// taught. lfr check reads it, holds every file to its format and the issues
// to the concept, compares it with the list on the server, and names the one
// step that comes next, so an agent can run a newsletter for months with
// nobody watching, or pick up a topic the owner hands in.
package project

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Anonyfox/lfr/internal/addr"
	"github.com/Anonyfox/lfr/internal/concept"
	"github.com/Anonyfox/lfr/internal/editorial"
	"github.com/Anonyfox/lfr/internal/research"
)

// Files and folders of a project.
const (
	ConfigFile    = "lfr.json"
	BacklogFile   = "backlog.md"
	LearningsFile = "learnings.md"
	WelcomeFile   = "welcome.md"
	IssuesDir     = "issues"
	SiteDir       = "site"
)

// Autonomy settings.
const (
	AutonomyDraft = "draft" // the agent drafts and tests; the owner approves each send
	AutonomySend  = "send"  // the agent sends when lfr check is clean
)

// Config is lfr.json.
type Config struct {
	SchemaVersion int    `json:"schemaVersion"`
	List          string `json:"list"`               // the list's slug on the server
	Language      string `json:"language"`           // en or de
	Owner         string `json:"owner,omitempty"`    // where test copies go
	Autonomy      string `json:"autonomy,omitempty"` // draft (default) or send
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Validate checks the config.
func (c Config) Validate() error {
	var errs []string
	if c.SchemaVersion != 1 {
		errs = append(errs, "schemaVersion must be 1")
	}
	if !slugRE.MatchString(c.List) {
		errs = append(errs, "list must be the list's slug (lowercase letters, digits, dashes)")
	}
	if c.Language == "" {
		errs = append(errs, "language is required (en or de)")
	}
	if c.Owner != "" {
		if _, err := addr.Email(c.Owner); err != nil {
			errs = append(errs, "owner: "+err.Error())
		}
	}
	if c.Autonomy != "" && c.Autonomy != AutonomyDraft && c.Autonomy != AutonomySend {
		errs = append(errs, "autonomy must be draft or send")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Mode returns the autonomy, defaulting to draft.
func (c Config) Mode() string {
	if c.Autonomy == "" {
		return AutonomyDraft
	}
	return c.Autonomy
}

// Find walks up from dir to the folder holding lfr.json.
func Find(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ConfigFile)); err == nil {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

// LoadConfig reads lfr.json in dir.
func LoadConfig(dir string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %v", ConfigFile, err)
	}
	return c, c.Validate()
}

//go:embed all:files
var scaffold embed.FS

// Init creates a project in dir for a list. dir must be empty or missing.
func Init(dir string, c Config, name string) ([]string, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				return nil, fmt.Errorf("%s is not empty; start a newsletter project in a new folder", dir)
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	written = append(written, ConfigFile)
	r := strings.NewReplacer("__NAME__", name, "__LIST__", c.List, "__LANG__", c.Language)
	err := fs.WalkDir(scaffold, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, "files/")
		if rel == "gitignore" {
			rel = ".gitignore"
		}
		src, err := scaffold.ReadFile(p)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(r.Replace(string(src))), 0o644); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	})
	return written, err
}

// Facts are the numbers and names skills render with, so a skill never
// states a minimum or a format the checks do not enforce.
func Facts() map[string]any {
	var sections []string
	for _, sec := range concept.Sections {
		name := sec.Name
		if sec.Optional {
			name += " (optional)"
		}
		sections = append(sections, name)
	}
	return map[string]any{
		"Config": ConfigFile, "Concept": concept.File, "Backlog": BacklogFile, "Learnings": LearningsFile,
		"Welcome": WelcomeFile, "Issues": IssuesDir, "Site": SiteDir,
		"Voices": "research/" + research.VoicesFile, "Peers": "research/" + research.PeersFile, "Evidence": "research/" + research.EvidenceFile,
		"MinVoices": research.MinVoices, "MinPlatforms": research.MinPlatforms, "MinPeers": research.MinPeers,
		"Authors": strings.Join(research.Authors, "|"), "Methods": strings.Join(research.Methods, "|"),
		"PeerFields": research.PeerFields, "EvidenceKinds": strings.Join(research.EvidenceKinds, "|"),
		"Dimensions": strings.Join(research.Dimensions, ", "),
		"Sections":   strings.Join(sections, ", "), "MinVoiceNotes": concept.MinVoiceNotes, "MinAxes": concept.MinAxes,
		"MinFormat": concept.MinFormat, "MaxFormat": concept.MaxFormat, "MinPillars": concept.MinPillars, "MaxPillars": concept.MaxPillars,
		"MinExamples": concept.MinExamples, "MinUse": concept.MinUse, "MinAvoid": concept.MinAvoid,
		"Statuses": strings.Join(Statuses, "|"), "MinOpenIdeas": MinOpenIdeas, "SettleHours": int(SettleTime.Hours()),
		"MaxSubject": editorial.DefaultMaxSubject, "MaxPreheader": editorial.MaxPreheader,
		"AutonomyDraft": AutonomyDraft, "AutonomySend": AutonomySend,
	}
}
