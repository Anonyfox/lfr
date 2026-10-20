package issues

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Anonyfox/lfr/internal/emailmd"
	"github.com/Anonyfox/lfr/internal/store"
)

// Images an issue references by a local path travel with it and are served
// from the list's website; emailmd.CheckImage holds the limits.
const (
	MaxMedia   = emailmd.MaxImage
	HeavyMedia = emailmd.HeavyImage
)

// mediaTypes maps accepted content types to file extensions.
var mediaTypes = emailmd.ImageTypes

// File is an image as the Markdown references it.
type File struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

// Media is one stored image of an issue.
type Media struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// Name is the file name the website serves the image under.
func (m Media) Name() string { return m.Hash + mediaTypes[m.Type] }

// CheckMedia validates an image the way SetMedia will.
func CheckMedia(p string, data []byte) (string, error) { return emailmd.CheckImage(p, data) }

// SetMedia replaces an issue's images. A change resets its test copy, like
// a change of the text.
func (s *Service) SetMedia(ctx context.Context, issueID int64, files []File) error {
	if s.Blobs == nil {
		return errors.New("no blob store")
	}
	type row struct {
		Media
		data []byte
	}
	var rows []row
	for _, f := range files {
		t, err := CheckMedia(f.Path, f.Data)
		if err != nil {
			return err
		}
		rows = append(rows, row{Media{Path: f.Path, Type: t, Size: len(f.Data)}, f.Data})
	}
	old, err := s.Media(ctx, issueID)
	if err != nil {
		return err
	}
	for i := range rows {
		if rows[i].Hash, err = s.Blobs.Put(rows[i].data); err != nil {
			return err
		}
	}
	same := len(old) == len(rows)
	for _, r := range rows {
		if m, ok := old[r.Path]; !ok || m.Hash != r.Hash {
			same = false
		}
	}
	if same {
		return nil
	}
	return s.DB.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM media WHERE issue_id = ?", issueID); err != nil {
			return err
		}
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx, "INSERT INTO media(issue_id, path, hash, type, size) VALUES(?, ?, ?, ?, ?)", issueID, r.Path, r.Hash, r.Type, r.Size); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE issues SET tested = 0, updated = ? WHERE id = ?", s.Now().Unix(), issueID)
		return err
	})
}

// Media returns an issue's images by path.
func (s *Service) Media(ctx context.Context, issueID int64) (map[string]Media, error) {
	rows, err := s.DB.R.QueryContext(ctx, "SELECT path, hash, type, size FROM media WHERE issue_id = ?", issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Media{}
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.Path, &m.Hash, &m.Type, &m.Size); err != nil {
			return nil, err
		}
		out[m.Path] = m
	}
	return out, rows.Err()
}

// MediaFile returns an image of a list's issues by its served name.
func (s *Service) MediaFile(ctx context.Context, listID int64, name string) ([]byte, string, error) {
	hash, ext, _ := strings.Cut(name, ".")
	var t string
	err := s.DB.R.QueryRowContext(ctx, `SELECT m.type FROM media m JOIN issues i ON i.id = m.issue_id
		WHERE i.list_id = ? AND m.hash = ? LIMIT 1`, listID, hash).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) || mediaTypes[t] != "."+ext {
		return nil, "", store.ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	data, err := s.Blobs.Get(hash)
	return data, t, err
}
