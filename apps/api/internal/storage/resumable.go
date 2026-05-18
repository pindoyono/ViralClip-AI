package storage

// ResumableUploadService and UploadProgressTracker implement Google Drive
// resumable uploads using the Drive v3 resumable upload protocol:
//
//  1. Initiate: POST {uploadEndpoint}?uploadType=resumable → Location header
//  2. Upload chunks: PUT {location} with Content-Range header
//  3. Query status: PUT {location} with Content-Range: bytes */{total}
//
// This allows files of any size (2 GiB+) to be uploaded reliably.
// Interrupted uploads can be resumed within the session's lifetime (≤ 1 week
// per the Drive API docs) using the stored upload URI.
//
// Retry policy: transient errors (408, 429, 5xx) are retried up to
// MaxRetries times with exponential backoff and jitter.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// DefaultUploadEndpoint is the Google Drive resumable upload endpoint.
	DefaultUploadEndpoint = "https://www.googleapis.com/upload/drive/v3/files"

	// DefaultChunkSize is 8 MiB.  Must be a multiple of 256 KiB per the
	// Drive API requirements.
	DefaultChunkSize = 8 * 1024 * 1024

	// minChunkAlignment is the minimum chunk-size alignment enforced by Drive.
	minChunkAlignment = 256 * 1024

	// DefaultMaxRetries is the maximum number of per-chunk retry attempts.
	DefaultMaxRetries = 5

	// DefaultRetryBaseDelay is the initial back-off delay.
	DefaultRetryBaseDelay = time.Second

	// resumableThreshold is the file-size threshold above which resumable
	// upload is used automatically.  Files whose size is unknown (FileSize == 0)
	// always use the simple upload path.
	resumableThreshold = 32 * 1024 * 1024 // 32 MiB

	// resumeHTTPIncomplete is the Drive "Resume Incomplete" status code.
	resumeHTTPIncomplete = 308
)

// ---------------------------------------------------------------------------
// ResumableUploadConfig
// ---------------------------------------------------------------------------

// ResumableUploadConfig controls the behaviour of ResumableUploadService.
type ResumableUploadConfig struct {
	// ChunkSize is the number of bytes per upload chunk.
	// Must be a multiple of 256 KiB (minChunkAlignment).  Default: 8 MiB.
	ChunkSize int

	// MaxRetries is the maximum number of retries per chunk on transient
	// errors before the upload is abandoned.  Default: 5.
	MaxRetries int

	// RetryBaseDelay is the initial back-off delay.  Subsequent retries use
	// exponential back-off with jitter.  Default: 1 s.
	RetryBaseDelay time.Duration

	// UploadEndpoint is the Drive resumable upload URL.
	// Override in tests to point at a mock HTTP server.
	// Default: "https://www.googleapis.com/upload/drive/v3/files".
	UploadEndpoint string
}

// withDefaults returns cfg with zero-value fields replaced by sensible defaults.
func (cfg ResumableUploadConfig) withDefaults() ResumableUploadConfig {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = DefaultChunkSize
	}
	// Round down to a multiple of minChunkAlignment.
	cfg.ChunkSize = (cfg.ChunkSize / minChunkAlignment) * minChunkAlignment
	if cfg.ChunkSize < minChunkAlignment {
		cfg.ChunkSize = minChunkAlignment
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	if cfg.RetryBaseDelay <= 0 {
		cfg.RetryBaseDelay = DefaultRetryBaseDelay
	}
	if cfg.UploadEndpoint == "" {
		cfg.UploadEndpoint = DefaultUploadEndpoint
	}
	return cfg
}

// ---------------------------------------------------------------------------
// UploadProgressTracker
// ---------------------------------------------------------------------------

// uploadSession is the internal representation of an active or completed
// upload, extending the public UploadProgress with the Drive upload URI.
type uploadSession struct {
	UploadProgress
	// uploadURI is the Drive resumable upload URI returned by the initiate
	// request.  Stored to allow interrupted uploads to be resumed.
	uploadURI string
}

// UploadProgressTracker maintains in-memory progress for resumable uploads.
// It is safe for concurrent use.
type UploadProgressTracker struct {
	mu      sync.RWMutex
	uploads map[string]*uploadSession
}

// NewUploadProgressTracker creates an UploadProgressTracker.
func NewUploadProgressTracker() *UploadProgressTracker {
	return &UploadProgressTracker{
		uploads: make(map[string]*uploadSession),
	}
}

// start registers a new upload session.
func (t *UploadProgressTracker) start(uploadID string, totalBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.uploads[uploadID] = &uploadSession{
		UploadProgress: UploadProgress{
			UploadID:   uploadID,
			TotalBytes: totalBytes,
			Status:     UploadStatusUploading,
			StartedAt:  time.Now().UTC(),
		},
	}
}

// setUploadURI stores the Drive upload URI so the upload can be resumed.
func (t *UploadProgressTracker) setUploadURI(uploadID, uri string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.uploads[uploadID]; ok {
		s.uploadURI = uri
	}
}

// getUploadURI returns the stored Drive upload URI, or "" if not found.
func (t *UploadProgressTracker) getUploadURI(uploadID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if s, ok := t.uploads[uploadID]; ok {
		return s.uploadURI
	}
	return ""
}

// update records the number of bytes confirmed received by Drive.
func (t *UploadProgressTracker) update(uploadID string, uploadedBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.uploads[uploadID]; ok {
		s.UploadedBytes = uploadedBytes
	}
}

// complete marks the upload as successfully finished.
func (t *UploadProgressTracker) complete(uploadID string, totalBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.uploads[uploadID]; ok {
		now := time.Now().UTC()
		s.Status = UploadStatusCompleted
		s.CompletedAt = &now
		s.UploadedBytes = totalBytes
	}
}

// fail marks the upload as permanently failed.
func (t *UploadProgressTracker) fail(uploadID, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.uploads[uploadID]; ok {
		s.Status = UploadStatusFailed
		s.Error = errMsg
	}
}

// SimulateStart is an exported helper for tests to seed an upload session.
// It has the same behaviour as the internal start method.
func (t *UploadProgressTracker) SimulateStart(uploadID string, totalBytes int64) {
	t.start(uploadID, totalBytes)
}

// SimulateUpdate is an exported helper for tests.
func (t *UploadProgressTracker) SimulateUpdate(uploadID string, uploadedBytes int64) {
	t.update(uploadID, uploadedBytes)
}

// SimulateComplete is an exported helper for tests.
func (t *UploadProgressTracker) SimulateComplete(uploadID string, totalBytes int64) {
	t.complete(uploadID, totalBytes)
}

// SimulateFail is an exported helper for tests.
func (t *UploadProgressTracker) SimulateFail(uploadID, errMsg string) {
	t.fail(uploadID, errMsg)
}

// Get returns a copy of the upload progress for uploadID.
// Returns (zero, false) when the ID is not tracked.
func (t *UploadProgressTracker) Get(uploadID string) (UploadProgress, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.uploads[uploadID]
	if !ok {
		return UploadProgress{}, false
	}
	return s.UploadProgress, true
}

// Delete removes the progress entry for uploadID.
func (t *UploadProgressTracker) Delete(uploadID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.uploads, uploadID)
}

// ---------------------------------------------------------------------------
// driveFileMeta – minimal JSON body for Drive file creation
// ---------------------------------------------------------------------------

type driveFileMeta struct {
	Name    string   `json:"name"`
	Parents []string `json:"parents,omitempty"`
}

// driveFileResponse – fields we need from Drive's upload completion response.
type driveFileResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        string `json:"size"`
	CreatedTime string `json:"createdTime"`
	MimeType    string `json:"mimeType"`
}

// ---------------------------------------------------------------------------
// ResumableUploadService
// ---------------------------------------------------------------------------

// ResumableUploadService uploads large files to Google Drive using the
// resumable upload protocol.  It supports 2 GiB+ files, per-chunk retry with
// exponential back-off, and progress tracking via UploadProgressTracker.
type ResumableUploadService struct {
	httpClient *http.Client
	tracker    *UploadProgressTracker
	cfg        ResumableUploadConfig
	logger     zerolog.Logger
}

// NewResumableUploadService creates a ResumableUploadService using an
// authenticated HTTP client (e.g. obtained from oauth2.NewClient).
func NewResumableUploadService(
	httpClient *http.Client,
	tracker *UploadProgressTracker,
	cfg ResumableUploadConfig,
) *ResumableUploadService {
	return &ResumableUploadService{
		httpClient: httpClient,
		tracker:    tracker,
		cfg:        cfg.withDefaults(),
		logger:     log.With().Str("component", "ResumableUploadService").Logger(),
	}
}

// Tracker returns the UploadProgressTracker used by this service.
// Callers can use it to inspect progress by upload ID.
func (s *ResumableUploadService) Tracker() *UploadProgressTracker {
	return s.tracker
}

// UploadFile uploads the content of r to the Drive folder identified by
// folderID.  It returns the Drive file ID of the created file.
//
// Parameters:
//   - folderID:     destination Drive folder ID
//   - filename:     the name under which the file will appear in Drive
//   - contentType:  MIME type (e.g. "video/mp4")
//   - r:            content reader (must not be nil)
//   - totalSize:    total file size in bytes; pass 0 if unknown
//   - uploadID:     caller-supplied ID for progress tracking; pass "" to disable
func (s *ResumableUploadService) UploadFile(
	ctx context.Context,
	folderID, filename, contentType string,
	r io.Reader,
	totalSize int64,
	uploadID string,
) (string, error) {
	if uploadID != "" {
		s.tracker.start(uploadID, totalSize)
	}

	s.logger.Info().
		Str("upload_id", uploadID).
		Str("filename", filename).
		Int64("total_bytes", totalSize).
		Msg("starting resumable upload")

	uploadURI, err := s.initiateSession(ctx, folderID, filename, contentType, totalSize)
	if err != nil {
		if uploadID != "" {
			s.tracker.fail(uploadID, err.Error())
		}
		return "", fmt.Errorf("resumable upload: initiate session: %w", err)
	}

	if uploadID != "" {
		s.tracker.setUploadURI(uploadID, uploadURI)
	}

	fileID, uploadedBytes, err := s.uploadChunks(ctx, uploadURI, r, totalSize, uploadID)
	if err != nil {
		if uploadID != "" {
			s.tracker.fail(uploadID, err.Error())
		}
		return "", fmt.Errorf("resumable upload: %w", err)
	}

	if uploadID != "" {
		s.tracker.complete(uploadID, uploadedBytes)
	}

	s.logger.Info().
		Str("upload_id", uploadID).
		Str("file_id", fileID).
		Int64("uploaded_bytes", uploadedBytes).
		Msg("resumable upload completed")

	return fileID, nil
}

// initiateSession POSTs the file metadata to the Drive upload endpoint and
// returns the upload URI from the Location response header.
func (s *ResumableUploadService) initiateSession(
	ctx context.Context,
	folderID, filename, contentType string,
	totalSize int64,
) (string, error) {
	meta := driveFileMeta{
		Name:    filename,
		Parents: []string{folderID},
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	url := s.cfg.UploadEndpoint + "?uploadType=resumable"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create initiate request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", contentType)
	if totalSize > 0 {
		req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(totalSize, 10))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("initiate request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("initiate session: status %d: %s", resp.StatusCode, string(body))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("initiate session: missing Location header")
	}

	s.logger.Debug().
		Str("filename", filename).
		Str("upload_uri", location).
		Msg("resumable upload session initiated")

	return location, nil
}

// uploadChunks reads r in chunks and PUTs each to uploadURI.
// Returns the Drive file ID, total bytes uploaded, and any error.
func (s *ResumableUploadService) uploadChunks(
	ctx context.Context,
	uploadURI string,
	r io.Reader,
	totalSize int64,
	uploadID string,
) (fileID string, uploadedBytes int64, err error) {
	chunkSize := s.cfg.ChunkSize
	offset := int64(0)

	for {
		if err := ctx.Err(); err != nil {
			return "", offset, fmt.Errorf("context cancelled at offset %d: %w", offset, err)
		}

		chunk := make([]byte, chunkSize)
		n, readErr := io.ReadFull(r, chunk)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return "", offset, fmt.Errorf("read chunk at offset %d: %w", offset, readErr)
		}

		if n == 0 {
			// No bytes read – we're done (should not happen in normal flow).
			break
		}

		chunk = chunk[:n]
		isLast := readErr == io.ErrUnexpectedEOF || readErr == io.EOF || int64(n) < int64(chunkSize)

		// Compute Content-Range.
		rangeEnd := offset + int64(n) - 1
		var contentRange string
		if totalSize > 0 {
			if isLast {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, offset+int64(n))
			} else {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, totalSize)
			}
		} else {
			// Unknown total size.
			if isLast {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, offset+int64(n))
			} else {
				contentRange = fmt.Sprintf("bytes %d-%d/*", offset, rangeEnd)
			}
		}

		var chunkFileID string
		chunkFileID, err = s.uploadChunkWithRetry(ctx, uploadURI, chunk, offset, contentRange, isLast)
		if err != nil {
			return "", offset, err
		}

		offset += int64(n)
		if uploadID != "" {
			s.tracker.update(uploadID, offset)
		}

		s.logger.Debug().
			Str("upload_id", uploadID).
			Int64("offset", offset).
			Int64("total", totalSize).
			Bool("is_last", isLast).
			Msg("chunk uploaded")

		// Drive signals completion by returning a non-empty file ID (200/201).
		// This can happen even when isLast==false (e.g. file size is an exact
		// multiple of chunk size) – return immediately in that case.
		if chunkFileID != "" || isLast {
			return chunkFileID, offset, nil
		}
	}

	return "", offset, nil
}

// uploadChunkWithRetry PUTs a single chunk to uploadURI with exponential
// back-off retry on transient errors.  Returns the Drive file ID when the
// upload is complete (i.e., the last chunk), or "" for intermediate chunks.
func (s *ResumableUploadService) uploadChunkWithRetry(
	ctx context.Context,
	uploadURI string,
	chunk []byte,
	offset int64,
	contentRange string,
	isLast bool,
) (fileID string, err error) {
	for attempt := 0; attempt <= s.cfg.MaxRetries; attempt++ {
		if err = ctx.Err(); err != nil {
			return "", err
		}

		if attempt > 0 {
			delay := s.backoffDelay(attempt)
			s.logger.Warn().
				Int("attempt", attempt).
				Dur("delay", delay).
				Str("content_range", contentRange).
				Msg("retrying chunk upload")

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		fileID, retriable, err := s.putChunk(ctx, uploadURI, chunk, contentRange, isLast)
		if err == nil {
			return fileID, nil
		}

		if !retriable {
			return "", fmt.Errorf("chunk upload failed (non-retriable): %w", err)
		}

		s.logger.Warn().
			Err(err).
			Int("attempt", attempt).
			Str("content_range", contentRange).
			Msg("transient chunk upload error")
	}

	return "", fmt.Errorf("chunk upload failed after %d retries for range %s: %w",
		s.cfg.MaxRetries, contentRange, err)
}

// putChunk performs a single PUT request for one chunk.
// Returns (fileID, isRetriable, error).
func (s *ResumableUploadService) putChunk(
	ctx context.Context,
	uploadURI string,
	chunk []byte,
	contentRange string,
	isLast bool,
) (fileID string, retriable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURI, bytes.NewReader(chunk))
	if err != nil {
		return "", false, fmt.Errorf("create chunk request: %w", err)
	}

	req.Header.Set("Content-Length", strconv.Itoa(len(chunk)))
	req.Header.Set("Content-Range", contentRange)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		// Network errors are always retriable.
		return "", true, fmt.Errorf("PUT chunk: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch {
	case resp.StatusCode == resumeHTTPIncomplete:
		// 308 – chunk received, more data expected.
		return "", false, nil

	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		// Upload complete.
		if !isLast {
			// Drive accepted early – treat as complete.
		}
		var result driveFileResponse
		if decErr := json.NewDecoder(resp.Body).Decode(&result); decErr != nil {
			return "", false, fmt.Errorf("decode completion response: %w", decErr)
		}
		return result.ID, false, nil

	case resp.StatusCode == http.StatusRequestTimeout,
		resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode >= 500:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", true, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))

	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", false, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
}

// QueryUploadStatus checks how many bytes Drive has received for an upload.
// Returns (bytesReceived, nil) on success.  A return of -1 means Drive has
// not received any bytes yet.
func (s *ResumableUploadService) QueryUploadStatus(ctx context.Context, uploadURI string, totalSize int64) (int64, error) {
	var rangeHeader string
	if totalSize > 0 {
		rangeHeader = fmt.Sprintf("bytes */%d", totalSize)
	} else {
		rangeHeader = "bytes */*"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURI, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("query status: create request: %w", err)
	}
	req.Header.Set("Content-Range", rangeHeader)
	req.Header.Set("Content-Length", "0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("query status: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		// Already complete.
		return totalSize, nil
	}

	if resp.StatusCode != resumeHTTPIncomplete {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return 0, fmt.Errorf("query status: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	rangeVal := resp.Header.Get("Range")
	if rangeVal == "" {
		// No bytes received yet.
		return -1, nil
	}

	// Range: bytes=0-{N}  or  0-{N}
	rangeVal = strings.TrimPrefix(rangeVal, "bytes=")
	parts := strings.SplitN(rangeVal, "-", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("query status: malformed Range header: %q", rangeVal)
	}

	lastByte, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("query status: parse Range end: %w", err)
	}

	return lastByte + 1, nil // Drive reports last byte index, we return bytes received
}

// ResumeUpload resumes an interrupted upload identified by uploadID.
// It queries Drive for the current offset and continues uploading from there.
// The reader r must be positioned at the start of the file; this method
// will discard already-uploaded bytes before continuing.
func (s *ResumableUploadService) ResumeUpload(
	ctx context.Context,
	uploadID string,
	r io.Reader,
	totalSize int64,
) (string, error) {
	uploadURI := s.tracker.getUploadURI(uploadID)
	if uploadURI == "" {
		return "", fmt.Errorf("resume upload: no upload URI found for id %q", uploadID)
	}

	bytesReceived, err := s.QueryUploadStatus(ctx, uploadURI, totalSize)
	if err != nil {
		return "", fmt.Errorf("resume upload: query status: %w", err)
	}

	if bytesReceived < 0 {
		bytesReceived = 0
	}

	// Discard already-uploaded bytes.
	if bytesReceived > 0 {
		if _, err := io.CopyN(io.Discard, r, bytesReceived); err != nil {
			return "", fmt.Errorf("resume upload: skip %d bytes: %w", bytesReceived, err)
		}
	}

	s.logger.Info().
		Str("upload_id", uploadID).
		Int64("resuming_from", bytesReceived).
		Int64("total", totalSize).
		Msg("resuming interrupted upload")

	// Continue uploading from the current offset.
	// We rebuild a reader that pretends we're at bytesReceived.
	// Update the tracker so progress resumes from the right offset.
	if uploadID != "" {
		s.tracker.update(uploadID, bytesReceived)
	}

	fileID, uploadedBytes, err := s.uploadChunksFromOffset(ctx, uploadURI, r, bytesReceived, totalSize, uploadID)
	if err != nil {
		s.tracker.fail(uploadID, err.Error())
		return "", err
	}

	s.tracker.complete(uploadID, uploadedBytes)
	return fileID, nil
}

// uploadChunksFromOffset is like uploadChunks but starts at startOffset.
func (s *ResumableUploadService) uploadChunksFromOffset(
	ctx context.Context,
	uploadURI string,
	r io.Reader,
	startOffset, totalSize int64,
	uploadID string,
) (fileID string, uploadedBytes int64, err error) {
	chunkSize := s.cfg.ChunkSize
	offset := startOffset

	for {
		if err := ctx.Err(); err != nil {
			return "", offset, fmt.Errorf("context cancelled at offset %d: %w", offset, err)
		}

		chunk := make([]byte, chunkSize)
		n, readErr := io.ReadFull(r, chunk)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return "", offset, fmt.Errorf("read chunk at offset %d: %w", offset, readErr)
		}
		if n == 0 {
			break
		}

		chunk = chunk[:n]
		isLast := readErr == io.ErrUnexpectedEOF || readErr == io.EOF || int64(n) < int64(chunkSize)

		rangeEnd := offset + int64(n) - 1
		var contentRange string
		if totalSize > 0 {
			if isLast {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, offset+int64(n))
			} else {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, totalSize)
			}
		} else {
			if isLast {
				contentRange = fmt.Sprintf("bytes %d-%d/%d", offset, rangeEnd, offset+int64(n))
			} else {
				contentRange = fmt.Sprintf("bytes %d-%d/*", offset, rangeEnd)
			}
		}

		var chunkFileID string
		chunkFileID, err = s.uploadChunkWithRetry(ctx, uploadURI, chunk, offset, contentRange, isLast)
		if err != nil {
			return "", offset, err
		}

		offset += int64(n)
		if uploadID != "" {
			s.tracker.update(uploadID, offset)
		}

		// Drive signals completion by returning a non-empty file ID.
		if chunkFileID != "" || isLast {
			return chunkFileID, offset, nil
		}
	}

	return "", offset, nil
}

// ---------------------------------------------------------------------------
// Back-off helper
// ---------------------------------------------------------------------------

// backoffDelay returns the delay for attempt n using exponential back-off with
// ±30 % jitter, capped at 64 s.
func (s *ResumableUploadService) backoffDelay(attempt int) time.Duration {
	base := float64(s.cfg.RetryBaseDelay) * math.Pow(2, float64(attempt-1))
	cap := float64(64 * time.Second)
	if base > cap {
		base = cap
	}
	// Add ±30 % jitter.
	jitter := base * 0.3 * (rand.Float64()*2 - 1) //nolint:gosec // non-crypto use
	d := time.Duration(base + jitter)
	if d < 0 {
		d = s.cfg.RetryBaseDelay
	}
	return d
}
