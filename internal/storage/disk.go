package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Disk stores originals and resized PNG cache on the filesystem.
type Disk struct {
	root string
}

// NewDisk creates directories under root.
func NewDisk(root string) (*Disk, error) {
	d := &Disk{root: root}
	if err := os.MkdirAll(d.OriginalsDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(d.CacheDir(), 0o755); err != nil {
		return nil, err
	}
	return d, nil
}

// OriginalsDir is where uploaded files live.
func (d *Disk) OriginalsDir() string {
	return filepath.Join(d.root, "originals")
}

// CacheDir is where resized PNGs live.
func (d *Disk) CacheDir() string {
	return filepath.Join(d.root, "cache")
}

// OriginalPath returns a path for a mark original.
func (d *Disk) OriginalPath(id, ext string) string {
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}
	return filepath.Join(d.OriginalsDir(), id+ext)
}

// SaveOriginal writes bytes to OriginalPath.
func (d *Disk) SaveOriginal(id, ext string, data []byte) (string, error) {
	path := d.OriginalPath(id, ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ReadOriginal reads an original file.
func (d *Disk) ReadOriginal(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// RemoveOriginal deletes an original if present.
func (d *Disk) RemoveOriginal(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CacheFile is the PNG path for a kind/slug/size.
func (d *Disk) CacheFile(kind, slug string, size int) string {
	return filepath.Join(d.CacheDir(), kind, slug, strconv.Itoa(size)+".png")
}

// ReadCache returns cached PNG bytes, or os.ErrNotExist.
func (d *Disk) ReadCache(kind, slug string, size int) ([]byte, error) {
	return os.ReadFile(d.CacheFile(kind, slug, size))
}

// WriteCache stores a resized PNG.
func (d *Disk) WriteCache(kind, slug string, size int, data []byte) error {
	path := d.CacheFile(kind, slug, size)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PurgeCache removes all cached sizes for a mark.
func (d *Disk) PurgeCache(kind, slug string) error {
	dir := filepath.Join(d.CacheDir(), kind, slug)
	err := os.RemoveAll(dir)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RelOriginal stores a portable relative path when possible.
func RelOriginal(dataDir, abs string) string {
	rel, err := filepath.Rel(dataDir, abs)
	if err != nil {
		return abs
	}
	return rel
}

// AbsOriginal resolves a stored path against dataDir.
func AbsOriginal(dataDir, stored string) string {
	if filepath.IsAbs(stored) {
		return stored
	}
	return filepath.Join(dataDir, stored)
}

// EnsureRoot documents the expected layout.
func EnsureRoot(root string) error {
	if root == "" {
		return fmt.Errorf("data dir is required")
	}
	return os.MkdirAll(root, 0o755)
}
