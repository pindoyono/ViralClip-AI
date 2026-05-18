// Package storage provides a pluggable abstraction for file storage backends.
//
// Two implementations are shipped:
//   - LocalStorageService – saves files to the local filesystem and is used
//     for transcripts, subtitles, cache, temp files, metadata and thumbnails.
//   - GoogleDriveStorageService – uploads files to Google Drive and is used
//     for source videos, generated clips and export files.
//
// Callers depend only on the StorageService interface; the concrete
// implementation is selected at startup by NewStorageService.
package storage

import (
	"context"
	"io"
	"time"
)

// FileInfo contains metadata returned after a successful upload.
type FileInfo struct {
	// Key is the storage key / identifier that can be passed back to
	// Download, Delete and GetURL. For local storage it is the relative
	// file path; for Google Drive it is the Drive file ID.
	Key string

	// URL is a directly accessible URL for the file, or an empty string
	// when the backend does not expose public URLs by default.
	URL string

	// Size is the byte count of the stored file.
	Size int64

	// ContentType is the MIME type, e.g. "video/mp4".
	ContentType string

	// CreatedAt is the time the file was stored.
	CreatedAt time.Time
}

// UploadOptions carries per-upload metadata that storage backends may use.
type UploadOptions struct {
	// ContentType is the MIME type of the file, e.g. "video/mp4".
	ContentType string

	// UserID scopes the upload to a specific user (used by Google Drive to
	// determine the destination folder).
	UserID string

	// Folder is an optional sub-folder hint such as "uploads", "clips" or
	// "exports". Backends that do not support folders may ignore this field.
	Folder string

	// Filename is the original filename that may be preserved by the backend.
	Filename string

	// FileSize is the total byte count of the file being uploaded.
	// When FileSize > 0 and the backend supports resumable uploads, the
	// backend will automatically use the resumable upload protocol for files
	// larger than the configured threshold (default 32 MiB).
	FileSize int64

	// UploadID is an optional caller-supplied identifier used to track upload
	// progress via ResumableStorageService.GetUploadProgress. When empty,
	// progress tracking is disabled for this upload.
	UploadID string
}

// StorageService is the interface every storage backend must satisfy.
type StorageService interface {
	// Upload stores the content of r under the given key and returns
	// metadata about the stored file. The caller is responsible for closing r.
	Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (*FileInfo, error)

	// Download opens the file identified by key for reading.
	// The caller must close the returned ReadCloser.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete permanently removes the file identified by key.
	Delete(ctx context.Context, key string) error

	// GetURL returns a directly accessible URL for the file identified by key.
	GetURL(ctx context.Context, key string) (string, error)
}

// UploadStatus describes the current state of a resumable upload.
type UploadStatus string

const (
	// UploadStatusUploading means the upload is in progress.
	UploadStatusUploading UploadStatus = "uploading"
	// UploadStatusCompleted means the upload finished successfully.
	UploadStatusCompleted UploadStatus = "completed"
	// UploadStatusFailed means the upload failed permanently.
	UploadStatusFailed UploadStatus = "failed"
)

// UploadProgress records the state of a resumable upload.
type UploadProgress struct {
	// UploadID is the caller-supplied identifier for this upload.
	UploadID string

	// TotalBytes is the expected total file size in bytes.
	// Zero means the total size is not known.
	TotalBytes int64

	// UploadedBytes is the number of bytes confirmed received by the backend.
	UploadedBytes int64

	// Status is the current upload status.
	Status UploadStatus

	// Error contains the error message when Status == UploadStatusFailed.
	Error string

	// StartedAt is the UTC time when the upload session was initiated.
	StartedAt time.Time

	// CompletedAt is the UTC time when the upload finished (or nil).
	CompletedAt *time.Time
}

// ResumableStorageService extends StorageService with resumable upload support
// for large files (2 GiB+).
type ResumableStorageService interface {
	StorageService

	// GetUploadProgress returns the current progress for the given uploadID.
	// Returns (zero, false) when no upload with that ID is tracked.
	GetUploadProgress(uploadID string) (UploadProgress, bool)
}
