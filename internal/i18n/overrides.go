package i18n

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Keys returns the complete, stable reader text vocabulary.
func Keys() []string {
	keys := make([]string, 0, len(text["en"]))
	for k := range text["en"] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Validate checks an override without changing state; an empty value resets it.
func Validate(key, value string) error {
	original, ok := text["en"][key]
	if !ok {
		return fmt.Errorf("unknown text key %q; use lfr list text <slug> to see the keys", key)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 2000 {
		return fmt.Errorf("text %s must be valid UTF-8 with at most 2000 characters; shorten it", key)
	}
	if value == "" {
		return nil
	}
	if key == "date" {
		sample := time.Date(2024, 11, 23, 0, 0, 0, 0, time.UTC)
		formatted := sample.Format(value)
		parsed, err := time.Parse(value, formatted)
		if err != nil || parsed.Year() != 2024 || parsed.Month() != 11 || parsed.Day() != 23 {
			return fmt.Errorf("text date must be a Go layout with year, month and day; use 2006-01-02")
		}
		return nil
	}
	verbs := func(s string) (string, bool) {
		var out strings.Builder
		for i := 0; i < len(s); i++ {
			if s[i] != '%' {
				continue
			}
			i++
			if i == len(s) {
				return "", false
			}
			switch s[i] {
			case '%':
			case 's':
				out.WriteByte('s')
			default:
				return "", false
			}
		}
		return out.String(), true
	}
	want, _ := verbs(original)
	got, valid := verbs(value)
	if !valid || got != want {
		return fmt.Errorf("text %s must keep the English %%s placeholders in order; use %d %%s placeholders and %%%% for a literal percent", key, len(want))
	}
	return nil
}
