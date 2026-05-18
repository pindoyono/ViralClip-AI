package storage_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
)

// newMockDriveService creates a GoogleDriveStorageService wired to a fake
// HTTP server so tests run without real Google credentials.
func newMockDriveService(t *testing.T, handler http.Handler) (*storage.GoogleDriveStorageService, *httptest.Server) {
	t.Helper()

	mockServer := httptest.NewServer(handler)
	t.Cleanup(mockServer.Close)

	// Build a service using the mock server URL and a static token source.
	staticToken := &oauth2.Token{
		AccessToken: "mock-access-token",
		Expiry:      time.Now().Add(time.Hour),
	}
	tokenSrc := oauth2.StaticTokenSource(staticToken)

	svc, err := drive.NewService(
		context.Background(),
		option.WithTokenSource(tokenSrc),
		option.WithEndpoint(mockServer.URL+"/"),
	)
	require.NoError(t, err)

	gdSvc := storage.NewGoogleDriveStorageServiceWithClient(svc)
	return gdSvc, mockServer
}

// ---------------------------------------------------------------------------
// Tests for factory validation
// ---------------------------------------------------------------------------

func TestNewGoogleDriveStorageService_MissingCredentials(t *testing.T) {
	_, err := storage.NewGoogleDriveStorageService(context.Background(), storage.GoogleDriveConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ClientID")
}

func TestNewGoogleDriveStorageService_MissingRefreshToken(t *testing.T) {
	_, err := storage.NewGoogleDriveStorageService(context.Background(), storage.GoogleDriveConfig{
		ClientID:     "cid",
		ClientSecret: "csecret",
	})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Tests for GetURL
// ---------------------------------------------------------------------------

func TestGoogleDriveStorageService_GetURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// respond to any drive API call with empty list so folder lookup succeeds
		json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}})
	})
	svc, _ := newMockDriveService(t, handler)

	url, err := svc.GetURL(context.Background(), "some-drive-file-id")
	require.NoError(t, err)
	assert.Equal(t, "https://drive.google.com/file/d/some-drive-file-id/view", url)
}

// ---------------------------------------------------------------------------
// Tests for Delete
// ---------------------------------------------------------------------------

func TestGoogleDriveStorageService_Delete_Success(t *testing.T) {
	deleted := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "file-to-delete") {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	svc, _ := newMockDriveService(t, handler)

	err := svc.Delete(context.Background(), "file-to-delete")
	require.NoError(t, err)
	assert.True(t, deleted, "expected DELETE request to be made")
}

func TestGoogleDriveStorageService_Delete_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    404,
				"message": "File not found",
			},
		})
	})
	svc, _ := newMockDriveService(t, handler)

	err := svc.Delete(context.Background(), "missing-file-id")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Tests for Upload
// ---------------------------------------------------------------------------

func TestGoogleDriveStorageService_Upload_Success(t *testing.T) {
	uploadCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "files"):
			// Folder listing – return empty list so folders get created.
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "files"):
			if strings.Contains(r.URL.RawQuery, "uploadType") {
				// Actual file upload.
				uploadCalled = true
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":          "new-drive-file-id",
					"name":        "video.mp4",
					"size":        "1024",
					"createdTime": "2024-01-01T00:00:00Z",
					"mimeType":    "video/mp4",
				})
			} else {
				// Folder creation.
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id": "folder-id",
				})
			}
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	})
	svc, _ := newMockDriveService(t, handler)

	info, err := svc.Upload(
		context.Background(),
		"videos/user1/abc.mp4",
		strings.NewReader("fake video content"),
		storage.UploadOptions{
			ContentType: "video/mp4",
			UserID:      "user1",
			Folder:      "uploads",
			Filename:    "abc.mp4",
		},
	)
	require.NoError(t, err)
	assert.True(t, uploadCalled, "expected upload request to be made")
	assert.Equal(t, "new-drive-file-id", info.Key)
	assert.Contains(t, info.URL, "new-drive-file-id")
}

// ---------------------------------------------------------------------------
// Compile-time interface check
// ---------------------------------------------------------------------------

func TestGoogleDriveStorageService_ImplementsInterface(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"files": []interface{}{}})
	})
	svc, _ := newMockDriveService(t, handler)

	var _ storage.StorageService = svc
}
