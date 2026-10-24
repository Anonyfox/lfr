package lists

import (
	"context"
	"database/sql"

	"github.com/Anonyfox/lfr/internal/i18n"
)

// Text resolves a reader-facing key using this list's overrides.
func (l *List) Text(key string) string {
	if v, ok := l.Texts[key]; ok {
		return v
	}
	return i18n.T(l.Language, key)
}

// TextValue includes the effective value and its source.
type TextValue struct {
	Value      string `json:"value"`
	Overridden bool   `json:"overridden"`
}

// TextValues returns all effective reader texts.
func (l *List) TextValues() map[string]TextValue {
	out := map[string]TextValue{}
	for _, k := range i18n.Keys() {
		_, ok := l.Texts[k]
		out[k] = TextValue{l.Text(k), ok}
	}
	return out
}

// SetTexts validates the whole batch before applying it atomically.
func (s *Service) SetTexts(ctx context.Context, slug string, values map[string]string) (*List, error) {
	for k, v := range values {
		if err := i18n.Validate(k, v); err != nil {
			return nil, err
		}
	}
	l, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	err = s.DB.Tx(ctx, func(tx *sql.Tx) error {
		for k, v := range values {
			var err error
			if v == "" {
				_, err = tx.ExecContext(ctx, "DELETE FROM list_texts WHERE list_id=? AND key=?", l.ID, k)
			} else {
				_, err = tx.ExecContext(ctx, "INSERT INTO list_texts(list_id,key,value) VALUES(?,?,?) ON CONFLICT(list_id,key) DO UPDATE SET value=excluded.value", l.ID, k, v)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, slug)
}
