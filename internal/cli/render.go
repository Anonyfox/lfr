package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"
)

// table prints aligned columns.
func table(w io.Writer, header []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(header) > 0 {
		fmt.Fprintln(tw, strings.ToUpper(strings.Join(header, "\t")))
	}
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
}

// kv prints "key: value" lines, skipping empty values.
func kv(w io.Writer, pairs ...string) {
	width := 0
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			width = max(width, len(pairs[i]))
		}
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			fmt.Fprintf(w, "%-*s  %s\n", width+1, pairs[i]+":", pairs[i+1])
		}
	}
}

func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("2006-01-02")
}

func when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// colors are used only on terminals, never in pipes or with NO_COLOR.
func colored(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func paint(w io.Writer, status string) string {
	if !colored(w) {
		return status
	}
	code := map[string]string{"ok": "32", "warn": "33", "warning": "33", "fail": "31", "error": "31", "skip": "2", "info": "36"}[status]
	if code == "" {
		return status
	}
	return "\x1b[" + code + "m" + status + "\x1b[0m"
}
