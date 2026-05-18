package storage_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
)

func TestLocalStorageService_Upload(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	content := []byte("hello video")
	info, err := svc.Upload(context.Background(), "videos/user1/vid.mp4", bytes.NewReader(content), storage.UploadOptions{
		ContentType: "video/mp4",
		UserID:      "user1",
		Folder:      "uploads",
		Filename:    "vid.mp4",
	})

	require.NoError(t, err)
	assert.Equal(t, "videos/user1/vid.mp4", info.Key)
	assert.Equal(t, "http://localhost/storage/videos/user1/vid.mp4", info.URL)
	assert.Equal(t, int64(len(content)), info.Size)
	assert.Equal(t, "video/mp4", info.ContentType)
	assert.False(t, info.CreatedAt.IsZero())

	// Verify file exists on disk with correct content.
	diskPath := filepath.Join(dir, "videos", "user1", "vid.mp4")
	data, err := os.ReadFile(diskPath)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestLocalStorageService_Upload_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	_, err := svc.Upload(context.Background(), "a/b/c/d/file.txt", bytes.NewReader([]byte("data")), storage.UploadOptions{})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, "a", "b", "c", "d", "file.txt"))
	assert.NoError(t, err)
}

func TestLocalStorageService_Download(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	content := []byte("transcript data")
	_, err := svc.Upload(context.Background(), "transcripts/t1.json", bytes.NewReader(content), storage.UploadOptions{})
	require.NoError(t, err)

	rc, err := svc.Download(context.Background(), "transcripts/t1.json")
	require.NoError(t, err)
	defer rc.Close()

	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestLocalStorageService_Download_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	_, err := svc.Download(context.Background(), "nonexistent/file.mp4")
	assert.Error(t, err)
}

func TestLocalStorageService_Delete(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	content := []byte("data")
	_, err := svc.Upload(context.Background(), "temp/tmp.bin", bytes.NewReader(content), storage.UploadOptions{})
	require.NoError(t, err)

	err = svc.Delete(context.Background(), "temp/tmp.bin")
	require.NoError(t, err)

	diskPath := filepath.Join(dir, "temp", "tmp.bin")
	_, err = os.Stat(diskPath)
	assert.True(t, os.IsNotExist(err), "file should be deleted")
}

func TestLocalStorageService_Delete_NonExistent(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	// Deleting a non-existent file should not return an error.
	err := svc.Delete(context.Background(), "no/such/file.mp4")
	assert.NoError(t, err)
}

func TestLocalStorageService_GetURL(t *testing.T) {
	svc := storage.NewLocalStorageService("/storage", "http://cdn.example.com")

	url, err := svc.GetURL(context.Background(), "thumbnails/abc.jpg")
	require.NoError(t, err)
	assert.Equal(t, "http://cdn.example.com/thumbnails/abc.jpg", url)
}

func TestLocalStorageService_GetURL_TrailingSlash(t *testing.T) {
	svc := storage.NewLocalStorageService("/storage", "http://cdn.example.com/")

	url, err := svc.GetURL(context.Background(), "thumbnails/abc.jpg")
	require.NoError(t, err)
	// Should not produce double slashes.
	assert.Equal(t, "http://cdn.example.com/thumbnails/abc.jpg", url)
}

func TestLocalStorageService_Upload_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	svc := storage.NewLocalStorageService(dir, "http://localhost/storage")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err := svc.Upload(ctx, "test/file.mp4", bytes.NewReader([]byte("data")), storage.UploadOptions{})
	assert.Error(t, err)
}

// TestLocalStorageService_ImplementsInterface verifies compile-time interface satisfaction.
func TestLocalStorageService_ImplementsInterface(t *testing.T) {
	var _ storage.StorageService = storage.NewLocalStorageService("", "")
}
