// Package finding is the one shape every check in lfr reports in: a rule ID,
// a severity, where, what is wrong and how to fix it. The CLI prints findings
// the same way whether they come from the server's issue lint or from a
// newsletter project on disk.
package finding

import (
	"fmt"
	"sort"
)

// Severities, strongest first.
const (
	Error   = "error"   // blocks the next step (sending, building)
	Warning = "warning" // lowers quality; fix or justify
	Info    = "info"    // a suggestion
)

// Finding is one problem.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

// Where renders file:line, or "" without a file.
func (f Finding) Where() string {
	switch {
	case f.File == "":
		return ""
	case f.Line > 0:
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return f.File
}

// Blocking reports whether any finding is an error.
func Blocking(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

// Count returns the number of findings per severity.
func Count(fs []Finding) (errors, warnings, infos int) {
	for _, f := range fs {
		switch f.Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		default:
			infos++
		}
	}
	return
}

func rank(s string) int {
	switch s {
	case Error:
		return 0
	case Warning:
		return 1
	}
	return 2
}

// Sort orders findings by severity, then file and line.
func Sort(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if rank(fs[i].Severity) != rank(fs[j].Severity) {
			return rank(fs[i].Severity) < rank(fs[j].Severity)
		}
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		return fs[i].Line < fs[j].Line
	})
}

// Rule describes a check for `lfr rules`.
type Rule struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Help    string `json:"help"`
}
