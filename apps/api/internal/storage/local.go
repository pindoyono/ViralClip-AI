package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorageService stores files on the local filesystem.
//
// It is the recommended backend for non-media assets such as transcripts,
// subtitles, cache files, temporary files, metadata JSON and thumbnails.
type LocalStorageService struct {
	basePath string
	baseURL  string
}

// NewLocalStorageService creates a new LocalStorageService.
// basePath is the root directory for all stored files (e.g. "./storage").
// baseURL is the public URL prefix used to construct download URLs
// (e.g. "http://localhost:8080/storage").
func NewLocalStorageService(basePath, baseURL string) *LocalStorageService {
	return &LocalStorageService{
		basePath: basePath,
		baseURL:  strings.TrimRight(baseURL, "/"),
	}
}

// Upload writes the content of r to basePath/key, creating parent directories
// as needed. The key should use forward slashes as path separators.
func (s *LocalStorageService) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (*FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dest := filepath.Join(s.basePath, filepath.FromSlash(key))

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return nil, fmt.Errorf("local storage: create directories for %q: %w", dest, err)
	}

	f, err := os.Create(dest) //nolint:gosec // path is constructed from basePath + caller-controlled key
	if err != nil {
		return nil, fmt.Errorf("local storage: create file %q: %w", dest, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return nil, fmt.Errorf("local storage: write file %q: %w", dest, err)
	}

	url, _ := s.GetURL(ctx, key)

	return &FileInfo{
		Key:         key,
		URL:         url,
		Size:        n,
		ContentType: opts.ContentType,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// Download opens the file at basePath/key for reading.
func (s *LocalStorageService) Download(_ context.Context, key string) (io.ReadCloser, error) {
	dest := filepath.Join(s.basePath, filepath.FromSlash(key))

	f, err := os.Open(dest) //nolint:gosec // path is constructed from basePath + caller-controlled key
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local storage: file not found: %q", key)
		}
		return nil, fmt.Errorf("local storage: open %q: %w", dest, err)
	}

	return f, nil
}

// Delete removes the file at basePath/key.
func (s *LocalStorageService) Delete(_ context.Context, key string) error {
	dest := filepath.Join(s.basePath, filepath.FromSlash(key))

	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local storage: delete %q: %w", dest, err)
	}

	return nil
}

// GetURL returns the public URL for the file at the given key.
func (s *LocalStorageService) GetURL(_ context.Context, key string) (string, error) {
	// Normalize slashes so URLs are always forward-slash separated.
	normalized := filepath.ToSlash(key)
	return s.baseURL + "/" + strings.TrimLeft(normalized, "/"), nil
}
