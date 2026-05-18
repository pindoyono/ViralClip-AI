package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
)

// ---------------------------------------------------------------------------
// UploadProgressTracker tests
// ---------------------------------------------------------------------------

func TestUploadProgressTracker_StartAndGet(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()

	_, ok := tracker.Get("unknown")
	assert.False(t, ok, "unknown upload should return false")

	tracker.SimulateStart("upload-1", 1024)
	p, ok := tracker.Get("upload-1")
	require.True(t, ok)
	assert.Equal(t, "upload-1", p.UploadID)
	assert.Equal(t, int64(1024), p.TotalBytes)
	assert.Equal(t, int64(0), p.UploadedBytes)
	assert.Equal(t, storage.UploadStatusUploading, p.Status)
	assert.False(t, p.StartedAt.IsZero())
	assert.Nil(t, p.CompletedAt)
}

func TestUploadProgressTracker_UpdateProgress(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()
	tracker.SimulateStart("upload-2", 2048)

	tracker.SimulateUpdate("upload-2", 512)
	p, ok := tracker.Get("upload-2")
	require.True(t, ok)
	assert.Equal(t, int64(512), p.UploadedBytes)
	assert.Equal(t, storage.UploadStatusUploading, p.Status)
}

func TestUploadProgressTracker_Complete(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()
	tracker.SimulateStart("upload-3", 4096)
	tracker.SimulateComplete("upload-3", 4096)

	p, ok := tracker.Get("upload-3")
	require.True(t, ok)
	assert.Equal(t, storage.UploadStatusCompleted, p.Status)
	assert.NotNil(t, p.CompletedAt)
	assert.Equal(t, int64(4096), p.UploadedBytes)
}

func TestUploadProgressTracker_Fail(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()
	tracker.SimulateStart("upload-4", 1024)
	tracker.SimulateFail("upload-4", "network error")

	p, ok := tracker.Get("upload-4")
	require.True(t, ok)
	assert.Equal(t, storage.UploadStatusFailed, p.Status)
	assert.Equal(t, "network error", p.Error)
}

func TestUploadProgressTracker_Delete(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()
	tracker.SimulateStart("upload-5", 1024)

	tracker.Delete("upload-5")
	_, ok := tracker.Get("upload-5")
	assert.False(t, ok)
}

func TestUploadProgressTracker_ConcurrentSafe(t *testing.T) {
	tracker := storage.NewUploadProgressTracker()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		id := fmt.Sprintf("upload-%d", i)
		go func(id string) {
			defer wg.Done()
			tracker.SimulateStart(id, 1024)
			tracker.SimulateUpdate(id, 512)
			tracker.SimulateComplete(id, 1024)
		}(id)

		go func(id string) {
			defer wg.Done()
			tracker.Get(id)
		}(id)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// ResumableUploadService tests
// ---------------------------------------------------------------------------

// mockResumableServer simulates the Drive resumable upload API.
type mockResumableServer struct {
	t *testing.T

	mu           sync.Mutex
	received     []byte
	totalSize    int64
	uploadID     string
	fileID       string
	// failOnChunk forces a 503 error on the N-th chunk PUT (0-indexed). -1 = never fail.
	failOnChunkN int32
	chunkCount   int32
}

func newMockResumableServer(t *testing.T, totalSize int64, failOnChunkN int) *mockResumableServer {
	t.Helper()
	return &mockResumableServer{
		t:            t,
		totalSize:    totalSize,
		fileID:       "drive-file-id-001",
		uploadID:     "test-upload-id",
		failOnChunkN: int32(failOnChunkN),
	}
}

func (m *mockResumableServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "upload/drive"):
		// Initiate resumable upload session.
		w.Header().Set("Location", fmt.Sprintf("http://%s/upload?uploadType=resumable&upload_id=%s", r.Host, m.uploadID))
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPut && r.URL.Path == "/upload":
		m.handleChunkPUT(w, r)

	// Single-file metadata GET (used by uploadResumable to fetch post-upload metadata).
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "files/"+m.fileID):
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"id":          m.fileID,
			"name":        "video.mp4",
			"size":        strconv.FormatInt(m.totalSize, 10),
			"createdTime": "2024-06-01T10:00:00Z",
			"mimeType":    "video/mp4",
		})

	// Folder listing / creation (needed when Drive service calls resolveFolder).
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "files"):
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}}) //nolint:errcheck

	case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "files"):
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "folder-id"}) //nolint:errcheck

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *mockResumableServer) handleChunkPUT(w http.ResponseWriter, r *http.Request) {
	contentRange := r.Header.Get("Content-Range")

	// Status query: Content-Range: bytes */{total}
	if strings.HasPrefix(contentRange, "bytes */") {
		m.mu.Lock()
		received := len(m.received)
		m.mu.Unlock()

		w.WriteHeader(resumeHTTPIncomplete)
		if received > 0 {
			w.Header().Set("Range", fmt.Sprintf("0-%d", received-1))
		}
		return
	}

	// Simulate transient failure on a specific chunk.
	n := atomic.AddInt32(&m.chunkCount, 1) - 1
	if m.failOnChunkN >= 0 && n == m.failOnChunkN {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.t.Errorf("read chunk body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	m.mu.Lock()
	m.received = append(m.received, body...)
	received := len(m.received)
	m.mu.Unlock()

	if int64(received) >= m.totalSize {
		// Upload complete.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"id":          m.fileID,
			"name":        "video.mp4",
			"size":        strconv.FormatInt(m.totalSize, 10),
			"createdTime": "2024-06-01T10:00:00Z",
			"mimeType":    "video/mp4",
		})
		return
	}

	// More chunks expected – 308.
	w.Header().Set("Range", fmt.Sprintf("0-%d", received-1))
	w.WriteHeader(resumeHTTPIncomplete)
}

// receivedData returns a copy of all bytes received so far.
func (m *mockResumableServer) receivedData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]byte, len(m.received))
	copy(out, m.received)
	return out
}

// resumeHTTPIncomplete mirrors the unexported constant inside the storage package.
const resumeHTTPIncomplete = 308

// newResumableUploadService builds a ResumableUploadService pointed at a mock server.
func newResumableUploadService(t *testing.T, handler http.Handler, chunkSize int) (*storage.ResumableUploadService, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	staticToken := &oauth2.Token{
		AccessToken: "mock-token",
		Expiry:      time.Now().Add(time.Hour),
	}
	httpClient := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(staticToken))

	tracker := storage.NewUploadProgressTracker()
	cfg := storage.ResumableUploadConfig{
		UploadEndpoint: srv.URL + "/upload/drive/v3/files",
		ChunkSize:      chunkSize,
		MaxRetries:     3,
		RetryBaseDelay: time.Millisecond,
	}
	svc := storage.NewResumableUploadService(httpClient, tracker, cfg)
	return svc, srv
}

func TestResumableUploadService_SmallFile(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 512*1024) // 512 KiB – one chunk
	mock := newMockResumableServer(t, int64(len(data)), -1)
	svc, _ := newResumableUploadService(t, mock, 1*1024*1024)

	fileID, err := svc.UploadFile(
		context.Background(),
		"folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "upload-test-1",
	)

	require.NoError(t, err)
	assert.Equal(t, "drive-file-id-001", fileID)
	assert.Equal(t, data, mock.receivedData())
}

func TestResumableUploadService_MultiChunk(t *testing.T) {
	chunkSize := 256 * 1024       // 256 KiB
	data := bytes.Repeat([]byte("v"), chunkSize*5) // 5 chunks
	mock := newMockResumableServer(t, int64(len(data)), -1)
	svc, _ := newResumableUploadService(t, mock, chunkSize)

	fileID, err := svc.UploadFile(
		context.Background(),
		"folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "upload-test-2",
	)

	require.NoError(t, err)
	assert.Equal(t, "drive-file-id-001", fileID)
	assert.Equal(t, data, mock.receivedData())
}

func TestResumableUploadService_RetryOnTransientError(t *testing.T) {
	chunkSize := 256 * 1024
	data := bytes.Repeat([]byte("r"), chunkSize*3)
	// First chunk (index 0) returns 503.
	mock := newMockResumableServer(t, int64(len(data)), 0)
	svc, _ := newResumableUploadService(t, mock, chunkSize)

	fileID, err := svc.UploadFile(
		context.Background(),
		"folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "upload-retry",
	)

	require.NoError(t, err)
	assert.Equal(t, "drive-file-id-001", fileID)
}

func TestResumableUploadService_ProgressTracking(t *testing.T) {
	chunkSize := 256 * 1024
	data := bytes.Repeat([]byte("p"), chunkSize*4)
	mock := newMockResumableServer(t, int64(len(data)), -1)

	svc, _ := newResumableUploadService(t, mock, chunkSize)

	fileID, err := svc.UploadFile(
		context.Background(),
		"folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "progress-upload",
	)

	require.NoError(t, err)
	assert.Equal(t, "drive-file-id-001", fileID)

	p, ok := svc.Tracker().Get("progress-upload")
	require.True(t, ok)
	assert.Equal(t, storage.UploadStatusCompleted, p.Status)
	assert.Equal(t, int64(len(data)), p.UploadedBytes)
	assert.NotNil(t, p.CompletedAt)
}

func TestResumableUploadService_ContextCancellation(t *testing.T) {
	data := bytes.Repeat([]byte("c"), 1*1024*1024) // 1 MiB

	ctx, cancel := context.WithCancel(context.Background())

	// Use a handler that cancels the context as soon as it receives the first chunk PUT.
	var firstChunkReceived atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Location", "http://"+r.Host+"/upload?uploadType=resumable&upload_id=ctx-test")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPut {
			if firstChunkReceived.CompareAndSwap(false, true) {
				// Cancel the context when the first chunk arrives, then return 503
				// so the service tries to retry – but the context is already done.
				cancel()
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	svc, _ := newResumableUploadService(t, handler, 256*1024)

	_, err := svc.UploadFile(ctx, "folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "ctx-upload")

	assert.Error(t, err, "expected error after context cancellation")
}

func TestResumableUploadService_NoUploadID_NoProgressTracked(t *testing.T) {
	data := bytes.Repeat([]byte("n"), 256*1024)
	mock := newMockResumableServer(t, int64(len(data)), -1)
	svc, _ := newResumableUploadService(t, mock, 256*1024)

	// Pass empty uploadID.
	fileID, err := svc.UploadFile(
		context.Background(),
		"folder-id", "video.mp4", "video/mp4",
		bytes.NewReader(data), int64(len(data)), "",
	)

	require.NoError(t, err)
	assert.Equal(t, "drive-file-id-001", fileID)
}

func TestResumableUploadService_QueryUploadStatus(t *testing.T) {
	received := 256 * 1024
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Range", fmt.Sprintf("0-%d", received-1))
		w.WriteHeader(resumeHTTPIncomplete)
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	staticToken := &oauth2.Token{AccessToken: "tok", Expiry: time.Now().Add(time.Hour)}
	httpClient := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(staticToken))
	tracker := storage.NewUploadProgressTracker()
	cfg := storage.ResumableUploadConfig{
		UploadEndpoint: srv.URL + "/upload/drive/v3/files",
		RetryBaseDelay: time.Millisecond,
	}
	svc := storage.NewResumableUploadService(httpClient, tracker, cfg)

	n, err := svc.QueryUploadStatus(context.Background(), srv.URL+"/upload?uploadType=resumable&upload_id=q1", 1*1024*1024)
	require.NoError(t, err)
	assert.Equal(t, int64(received), n)
}

// ---------------------------------------------------------------------------
// GoogleDriveStorageService resumable integration tests
// ---------------------------------------------------------------------------

// buildResumableMockHandler returns an http.Handler that handles both the
// Drive folder API calls and the resumable upload protocol.
func buildResumableMockHandler(t *testing.T, totalSize int64, fileID string) http.Handler {
	t.Helper()
	mock := newMockResumableServer(t, totalSize, -1)
	mock.fileID = fileID
	return mock
}

func TestGoogleDriveStorageService_Upload_LargeFile_UsesResumable(t *testing.T) {
	// 33 MiB > resumableThreshold (32 MiB) → should route to resumable upload.
	totalSize := int64(33 * 1024 * 1024)
	data := bytes.Repeat([]byte("L"), int(totalSize))

	handler := buildResumableMockHandler(t, totalSize, "large-file-id")
	gdSvc, _ := newMockDriveService(t, handler)

	// Override chunk size to 8 MiB for this test.
	info, err := gdSvc.Upload(
		context.Background(),
		"videos/user1/large.mp4",
		bytes.NewReader(data),
		storage.UploadOptions{
			ContentType: "video/mp4",
			UserID:      "user1",
			Folder:      "uploads",
			Filename:    "large.mp4",
			FileSize:    totalSize,
			UploadID:    "large-upload-1",
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "large-file-id", info.Key)
	assert.Contains(t, info.URL, "large-file-id")
}

func TestGoogleDriveStorageService_Upload_SmallFile_SkipsResumable(t *testing.T) {
	// < resumableThreshold → standard upload path.
	uploadCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "files"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}}) //nolint:errcheck
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "files"):
			if strings.Contains(r.URL.RawQuery, "uploadType") {
				uploadCalled = true
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
					"id":          "small-file-id",
					"name":        "small.mp4",
					"size":        "1024",
					"createdTime": "2024-01-01T00:00:00Z",
					"mimeType":    "video/mp4",
				})
			} else {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{"id": "folder-id"}) //nolint:errcheck
			}
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{}) //nolint:errcheck
		}
	})

	gdSvc, _ := newMockDriveService(t, handler)

	info, err := gdSvc.Upload(
		context.Background(),
		"videos/user1/small.mp4",
		strings.NewReader("small video data"),
		storage.UploadOptions{
			ContentType: "video/mp4",
			UserID:      "user1",
			Folder:      "uploads",
			Filename:    "small.mp4",
			FileSize:    1024, // well below threshold
		},
	)

	require.NoError(t, err)
	assert.True(t, uploadCalled)
	assert.Equal(t, "small-file-id", info.Key)
}

func TestGoogleDriveStorageService_GetUploadProgress(t *testing.T) {
	totalSize := int64(33 * 1024 * 1024)
	data := bytes.Repeat([]byte("G"), int(totalSize))

	handler := buildResumableMockHandler(t, totalSize, "progress-file-id")
	gdSvc, _ := newMockDriveService(t, handler)

	_, err := gdSvc.Upload(
		context.Background(),
		"videos/u1/prog.mp4",
		bytes.NewReader(data),
		storage.UploadOptions{
			ContentType: "video/mp4",
			UserID:      "u1",
			Folder:      "uploads",
			Filename:    "prog.mp4",
			FileSize:    totalSize,
			UploadID:    "prog-upload",
		},
	)
	require.NoError(t, err)

	p, ok := gdSvc.GetUploadProgress("prog-upload")
	require.True(t, ok)
	assert.Equal(t, storage.UploadStatusCompleted, p.Status)
	assert.Equal(t, totalSize, p.UploadedBytes)
}

func TestGoogleDriveStorageService_GetUploadProgress_Unknown(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}}) //nolint:errcheck
	})
	gdSvc, _ := newMockDriveService(t, handler)

	_, ok := gdSvc.GetUploadProgress("no-such-upload")
	assert.False(t, ok)
}

func TestGoogleDriveStorageService_ImplementsResumableStorageService(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}}) //nolint:errcheck
	})
	gdSvc, _ := newMockDriveService(t, handler)
	var _ storage.ResumableStorageService = gdSvc
}
