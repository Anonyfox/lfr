// Package blob stores raw messages as immutable files named by their SHA-256,
// so a message delivered to several mailboxes is kept once and a crash never
// leaves a half-written file behind.
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store is a directory of content-addressed files.
type Store struct{ dir string }

// Open uses dir, creating it when missing.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func valid(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func (s *Store) path(hash string) string {
	return filepath.Join(s.dir, hash[:2], hash+".eml")
}

// Put writes data and returns its hash. Writing the same content twice is a no-op.
func (s *Store) Put(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	p := s.path(hash)
	if _, err := os.Stat(p); err == nil {
		return hash, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, p)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	// The rename is durable only once the directory is synced.
	if d, err := os.Open(filepath.Dir(p)); err == nil {
		err = d.Sync()
		d.Close()
		if err != nil {
			return "", err
		}
	}
	return hash, nil
}

// Get reads a whole blob.
func (s *Store) Get(hash string) ([]byte, error) {
	if !valid(hash) {
		return nil, fmt.Errorf("blob: invalid hash %q", hash)
	}
	return os.ReadFile(s.path(hash))
}

// Open streams a blob.
func (s *Store) Open(hash string) (io.ReadCloser, error) {
	if !valid(hash) {
		return nil, fmt.Errorf("blob: invalid hash %q", hash)
	}
	return os.Open(s.path(hash))
}

// Remove deletes a blob; removing a missing blob is not an error.
func (s *Store) Remove(hash string) error {
	if !valid(hash) {
		return fmt.Errorf("blob: invalid hash %q", hash)
	}
	err := os.Remove(s.path(hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Walk calls fn for every stored hash.
func (s *Store) Walk(fn func(hash string) error) error {
	return filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		if filepath.Ext(name) == ".eml" && valid(name[:len(name)-4]) {
			return fn(name[:len(name)-4])
		}
		return nil
	})
}
