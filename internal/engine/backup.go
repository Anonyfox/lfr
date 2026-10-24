package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Anonyfox/lfr/internal/api"
)

// Backup streams a snapshot and immutable blobs, bounded by socket backpressure.
func (e *Engine) Backup(ctx context.Context, send func(any) error) error {
	if !e.backupMu.TryLock() {
		return invalid("a backup or blob cleanup is running; retry shortly")
	}
	defer e.backupMu.Unlock()
	tmp, err := os.MkdirTemp(e.Opts.Data, ".backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	snapshot := filepath.Join(tmp, DBFile)
	if err = e.DB.Backup(ctx, snapshot); err != nil {
		return err
	}
	res := api.BackupResult{}
	file := func(path, source string) (int64, error) {
		f, err := os.Open(source)
		if err != nil {
			return 0, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return 0, err
		}
		if !info.Mode().IsRegular() {
			return 0, fmt.Errorf("backup file %s is not regular; remove it and retry", path)
		}
		hash := sha256.New()
		buf := make([]byte, 192<<10)
		var offset int64
		for {
			if err := ctx.Err(); err != nil {
				return offset, err
			}
			n, err := f.Read(buf)
			if n > 0 {
				hash.Write(buf[:n])
				if serr := send(api.BackupFrame{Path: filepath.ToSlash(path), Offset: offset, Data: buf[:n]}); serr != nil {
					return offset, serr
				}
				offset += int64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return offset, err
			}
		}
		return offset, send(api.BackupFrame{Path: filepath.ToSlash(path), Offset: offset, End: true, Size: offset, SHA256: hex.EncodeToString(hash.Sum(nil))})
	}
	if _, err = file(DBFile, snapshot); err != nil {
		return err
	}
	if err = walkBackupFiles(ctx, filepath.Join(e.Opts.Data, BlobDir), func(p string) error {
		rel, err := filepath.Rel(e.Opts.Data, p)
		if err != nil {
			return err
		}
		// Only immutable message blobs belong to the backup.
		if filepath.Ext(p) != ".eml" {
			return nil
		}
		n, err := file(rel, p)
		if err == nil {
			res.Blobs++
			res.Bytes += n
		}
		return err
	}); err != nil {
		return err
	}

	root := filepath.Join(e.Opts.Data, SiteDir)
	if _, err = os.Stat(root); err == nil {
		err = walkBackupFiles(ctx, root, func(p string) error {
			rel, err := filepath.Rel(e.Opts.Data, p)
			if err != nil {
				return err
			}
			_, err = file(rel, p)
			return err
		})
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	done := fmt.Sprintf("lfr backup complete\nat: %s\nmessages: %d\nbytes: %d\n", e.Opts.Now().UTC().Format(time.RFC3339), res.Blobs, res.Bytes)
	return send(api.BackupFrame{Summary: &res, Complete: done})
}

// walkBackupFiles reads directories in bounded batches rather than sorting an
// entire directory into memory.
func walkBackupFiles(ctx context.Context, root string, visit func(string) error) error {
	d, err := os.Open(root)
	if err != nil {
		return err
	}
	defer d.Close()
	for {
		entries, err := d.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			p := filepath.Join(root, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("backup contains a symlink; remove it and retry")
			}
			if entry.IsDir() {
				if err := walkBackupFiles(ctx, p, visit); err != nil {
					return err
				}
			} else if err := visit(p); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
