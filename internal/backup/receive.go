// Package backup verifies streamed backups before marking them complete.
package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfr/internal/api"
	"github.com/Anonyfox/lfr/internal/ctl"
)

// ValidPath accepts only portable, clean relative backup paths.
func ValidPath(p string) bool {
	if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, "\\:\x00") || path.Clean(p) != p {
		return false
	}
	if p == "lfr.db" {
		return true
	}
	parts := strings.Split(p, "/")
	if len(parts) >= 2 && parts[0] == "sites" {
		return true
	}
	if len(parts) != 3 || parts[0] != "msg" || len(parts[1]) != 2 || len(parts[2]) != 68 || !strings.HasSuffix(parts[2], ".eml") {
		return false
	}
	h := strings.TrimSuffix(parts[2], ".eml")
	_, err := hex.DecodeString(h)
	return err == nil && h[:2] == parts[1] && strings.ToLower(h) == h
}

// Receive writes a backup on the caller's machine and marks it only after verification.
func Receive(ctx context.Context, client ctl.Client, dir string) (res api.BackupResult, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return res, err
	}
	res.Dir = dir
	entries, e := os.ReadDir(dir)
	if e == nil && len(entries) > 0 {
		return res, fmt.Errorf("%s is not empty; choose an empty backup directory", dir)
	}
	if e != nil && !os.IsNotExist(e) {
		return res, e
	}
	if e = os.MkdirAll(dir, 0o750); e != nil {
		return res, e
	}
	var f *os.File
	var name string
	var offset int64
	var digest hash.Hash
	var summary *api.BackupResult
	var complete string
	var blobs int
	var bytes int64
	dbSeen := false
	defer func() {
		if f != nil {
			f.Close()
		}
		if err != nil {
			err = fmt.Errorf("backup directory %s is incomplete; retry into an empty directory: %w", dir, err)
		}
	}()
	err = client.Stream(ctx, api.OpBackup, nil, func(raw json.RawMessage) error {
		var frame api.BackupFrame
		if len(raw) > 300<<10 {
			return fmt.Errorf("backup frame too large")
		}
		if err := json.Unmarshal(raw, &frame); err != nil {
			return err
		}
		if summary != nil {
			return fmt.Errorf("backup data after summary")
		}
		if frame.Summary != nil {
			if f != nil || frame.Path != "" || len(frame.Data) != 0 || frame.End || !dbSeen {
				return fmt.Errorf("invalid backup summary")
			}
			if frame.Summary.Blobs != blobs || frame.Summary.Bytes != bytes {
				return fmt.Errorf("backup summary mismatch")
			}
			lines := strings.Split(frame.Complete, "\n")
			if len(lines) != 5 || lines[0] != "lfr backup complete" || !strings.HasPrefix(lines[1], "at: ") || lines[2] != fmt.Sprintf("messages: %d", blobs) || lines[3] != fmt.Sprintf("bytes: %d", bytes) {
				return fmt.Errorf("invalid completion marker")
			}
			summary = frame.Summary
			complete = frame.Complete
			return nil
		}
		if !ValidPath(frame.Path) || len(frame.Data) > 192<<10 || frame.Offset < 0 {
			return fmt.Errorf("invalid backup file frame")
		}
		if f == nil {
			if frame.Offset != 0 {
				return fmt.Errorf("backup file starts at nonzero offset")
			}
			name = frame.Path
			offset = 0
			digest = sha256.New()
			target := filepath.Join(dir, filepath.FromSlash(name))
			// Reject symlink parents, including the target root.
			for parent := filepath.Dir(target); ; parent = filepath.Dir(parent) {
				st, e := os.Lstat(parent)
				if e == nil && (!st.IsDir() || st.Mode()&os.ModeSymlink != 0) {
					return fmt.Errorf("unsafe backup directory")
				}
				if e != nil && !os.IsNotExist(e) {
					return e
				}
				if parent == dir {
					break
				}
				if parent == filepath.Dir(parent) {
					return fmt.Errorf("invalid backup root")
				}
			}
			if e := os.MkdirAll(filepath.Dir(target), 0o750); e != nil {
				return e
			}
			var e error
			f, e = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
			if e != nil {
				return e
			}
		}
		if frame.Path != name || frame.Offset != offset {
			return fmt.Errorf("backup file offset or path mismatch")
		}
		if frame.End {
			if len(frame.Data) != 0 || frame.Size != offset || frame.SHA256 != hex.EncodeToString(digest.Sum(nil)) {
				return fmt.Errorf("backup file size or sha256 mismatch")
			}
			if strings.HasPrefix(name, "msg/") {
				if strings.TrimSuffix(path.Base(name), ".eml") != frame.SHA256 {
					return fmt.Errorf("backup blob hash mismatch")
				}
				blobs++
				bytes += offset
			}
			if name == "lfr.db" {
				dbSeen = true
			}
			if e := f.Sync(); e != nil {
				return e
			}
			e := f.Close()
			f = nil
			if e != nil {
				return e
			}
			return syncParents(filepath.Dir(filepath.Join(dir, filepath.FromSlash(name))), dir)
		}
		if len(frame.Data) == 0 || frame.SHA256 != "" || frame.Size != 0 {
			return fmt.Errorf("invalid backup data chunk")
		}
		n, e := f.Write(frame.Data)
		if e != nil {
			return e
		}
		digest.Write(frame.Data)
		offset += int64(n)
		return nil
	})
	if err != nil {
		return res, err
	}
	if ctx.Err() != nil {
		return res, ctx.Err()
	}
	if summary == nil || f != nil {
		return res, fmt.Errorf("backup ended before completion")
	}
	marker, e := os.OpenFile(filepath.Join(dir, "BACKUP-COMPLETE"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if e != nil {
		return res, e
	}
	_, e = marker.WriteString(complete)
	if e == nil {
		e = marker.Sync()
	}
	closeErr := marker.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		os.Remove(filepath.Join(dir, "BACKUP-COMPLETE"))
		return res, e
	}
	if e = ctx.Err(); e != nil {
		os.Remove(filepath.Join(dir, "BACKUP-COMPLETE"))
		return res, e
	}
	if e = syncParents(dir, dir); e != nil {
		os.Remove(filepath.Join(dir, "BACKUP-COMPLETE"))
		return res, e
	}
	res = *summary
	res.Dir = dir
	return res, nil
}

func syncParents(dir, root string) error {
	for {
		d, err := os.Open(dir)
		if err != nil {
			return err
		}
		err = d.Sync()
		closeErr := d.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if dir == root {
			return nil
		}
		dir = filepath.Dir(dir)
	}
}
