package marks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ArminDashti/mark-api/internal/models"
	"github.com/ArminDashti/mark-api/internal/render"
	"github.com/ArminDashti/mark-api/internal/storage"
	"github.com/google/uuid"
)

var slugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const maxUploadBytes = 8 << 20

// Input is metadata for create/update.
type Input struct {
	Kind string
	Slug string
	Name string
}

// File is an uploaded original.
type File struct {
	Bytes []byte
	MIME  string
	Name  string
}

// Service coordinates marks, disk, and rendering.
type Service struct {
	store   *Store
	disk    *storage.Disk
	dataDir string
}

// NewService creates a Service.
func NewService(store *Store, disk *storage.Disk, dataDir string) *Service {
	return &Service{store: store, disk: disk, dataDir: dataDir}
}

// List returns marks for a kind (or all), optionally matching name or slug.
func (s *Service) List(ctx context.Context, kind, q string) ([]models.Mark, error) {
	kind, err := optionalKind(kind)
	if err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if len(q) > 64 {
		return nil, fmt.Errorf("search query is too long")
	}
	return s.store.List(ctx, kind, q)
}

// Get returns a mark by id.
func (s *Service) Get(ctx context.Context, id string) (*models.Mark, error) {
	return s.store.GetByID(ctx, id)
}

// Create stores an original and a mark row.
func (s *Service) Create(ctx context.Context, in Input, file File) (*models.Mark, error) {
	in, err := normalizeInput(in)
	if err != nil {
		return nil, err
	}
	if len(file.Bytes) == 0 {
		return nil, fmt.Errorf("file is required")
	}
	if len(file.Bytes) > maxUploadBytes {
		return nil, fmt.Errorf("file is too large")
	}

	mime := detectMIME(file.MIME, file.Name, file.Bytes)
	info, err := render.Inspect(file.Bytes, mime)
	if err != nil {
		return nil, fmt.Errorf("could not read image: %w", err)
	}

	id := uuid.NewString()
	ext := extForMIME(info.MIME, file.Name)
	abs, err := s.disk.SaveOriginal(id, ext, file.Bytes)
	if err != nil {
		return nil, err
	}

	m := &models.Mark{
		ID:           id,
		Kind:         in.Kind,
		Slug:         in.Slug,
		Name:         in.Name,
		OriginalPath: storage.RelOriginal(s.dataDir, abs),
		OriginalMIME: info.MIME,
		Width:        info.Width,
		Height:       info.Height,
		HasAlpha:     info.HasAlpha,
	}
	created, err := s.store.Insert(ctx, m)
	if err != nil {
		_ = s.disk.RemoveOriginal(abs)
		return nil, err
	}
	return created, nil
}

// Update changes metadata and optionally replaces the original.
func (s *Service) Update(ctx context.Context, id string, in Input, file *File) (*models.Mark, error) {
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	in, err = normalizeInput(in)
	if err != nil {
		return nil, err
	}

	oldKind, oldSlug := existing.Kind, existing.Slug
	oldPath := existing.OriginalPath

	existing.Kind = in.Kind
	existing.Slug = in.Slug
	existing.Name = in.Name

	var newAbs string
	if file != nil && len(file.Bytes) > 0 {
		if len(file.Bytes) > maxUploadBytes {
			return nil, fmt.Errorf("file is too large")
		}
		mime := detectMIME(file.MIME, file.Name, file.Bytes)
		info, err := render.Inspect(file.Bytes, mime)
		if err != nil {
			return nil, fmt.Errorf("could not read image: %w", err)
		}
		ext := extForMIME(info.MIME, file.Name)
		newAbs, err = s.disk.SaveOriginal(existing.ID, ext, file.Bytes)
		if err != nil {
			return nil, err
		}
		existing.OriginalPath = storage.RelOriginal(s.dataDir, newAbs)
		existing.OriginalMIME = info.MIME
		existing.Width = info.Width
		existing.Height = info.Height
		existing.HasAlpha = info.HasAlpha
	}

	updated, err := s.store.Update(ctx, existing)
	if err != nil {
		if newAbs != "" {
			_ = s.disk.RemoveOriginal(newAbs)
		}
		return nil, err
	}

	_ = s.disk.PurgeCache(oldKind, oldSlug)
	if oldKind != updated.Kind || oldSlug != updated.Slug {
		_ = s.disk.PurgeCache(updated.Kind, updated.Slug)
	}
	if newAbs != "" {
		absOld := storage.AbsOriginal(s.dataDir, oldPath)
		if absOld != newAbs {
			_ = s.disk.RemoveOriginal(absOld)
		}
	}
	return updated, nil
}

// Delete removes the row, original, and cache.
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.disk.PurgeCache(existing.Kind, existing.Slug)
	_ = s.disk.RemoveOriginal(storage.AbsOriginal(s.dataDir, existing.OriginalPath))
	return nil
}

// ServePNG returns a cached or freshly rendered square PNG.
func (s *Service) ServePNG(ctx context.Context, kind, slug string, size int) ([]byte, error) {
	kind, err := requireKind(kind)
	if err != nil {
		return nil, err
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRE.MatchString(slug) {
		return nil, fmt.Errorf("invalid slug")
	}
	size = render.ClampSize(size)

	if cached, err := s.disk.ReadCache(kind, slug, size); err == nil {
		return cached, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	m, err := s.store.GetByKindSlug(ctx, kind, slug)
	if err != nil {
		return nil, err
	}
	raw, err := s.disk.ReadOriginal(storage.AbsOriginal(s.dataDir, m.OriginalPath))
	if err != nil {
		return nil, err
	}
	png, err := render.PNGSquare(raw, m.OriginalMIME, size)
	if err != nil {
		return nil, err
	}
	_ = s.disk.WriteCache(kind, slug, size, png)
	return png, nil
}

func normalizeInput(in Input) (Input, error) {
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, fmt.Errorf("name is required")
	}
	if _, err := requireKind(in.Kind); err != nil {
		return in, err
	}
	if !slugRE.MatchString(in.Slug) {
		return in, fmt.Errorf("slug must be lowercase letters, numbers, and hyphens")
	}
	if len(in.Slug) > 64 {
		return in, fmt.Errorf("slug is too long")
	}
	return in, nil
}

func requireKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "logo" && kind != "icon" {
		return "", fmt.Errorf("kind must be logo or icon")
	}
	return kind, nil
}

func optionalKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "", nil
	}
	return requireKind(kind)
}

func detectMIME(hint, filename string, data []byte) string {
	hint = strings.ToLower(strings.TrimSpace(hint))
	if strings.Contains(hint, "svg") {
		return "image/svg+xml"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	}
	if strings.HasPrefix(hint, "image/") {
		return hint
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	return hint
}

func extForMIME(mime, filename string) string {
	switch mime {
	case "image/svg+xml":
		return ".svg"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" {
		return ext
	}
	return ".bin"
}

// IsNotFound reports ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsConflict reports ErrConflict.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsBadRequest reports validation errors (not not-found/conflict/internal).
func IsBadRequest(err error) bool {
	if err == nil || IsNotFound(err) || IsConflict(err) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "required") ||
		strings.Contains(msg, "must") ||
		strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "too large") ||
		strings.Contains(msg, "too long") ||
		strings.Contains(msg, "could not read image")
}
