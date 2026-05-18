package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
)

// mockStorageService is a StorageService implementation used in tests.
type mockStorageService struct {
	uploadErr   error
	downloadErr error
	deleteErr   error
	uploadedKey string
}

func (m *mockStorageService) Upload(_ context.Context, key string, _ io.Reader, opts storage.UploadOptions) (*storage.FileInfo, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	m.uploadedKey = key
	return &storage.FileInfo{
		Key:         key,
		URL:         "http://localhost/storage/" + key,
		Size:        1024,
		ContentType: opts.ContentType,
		CreatedAt:   time.Now(),
	}, nil
}

func (m *mockStorageService) Download(_ context.Context, key string) (io.ReadCloser, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	return io.NopCloser(nil), nil
}

func (m *mockStorageService) Delete(_ context.Context, key string) error {
	return m.deleteErr
}

func (m *mockStorageService) GetURL(_ context.Context, key string) (string, error) {
	return "http://localhost/storage/" + key, nil
}

func setupVideoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Video{}, &models.Clip{}, &models.User{}))
	return db
}

func setupVideoApp(db *gorm.DB) *fiber.App {
	app := fiber.New()
	h := NewVideoHandler(db, &mockStorageService{})

	// Inject a fake JWT token via middleware for tests
	app.Use(func(c *fiber.Ctx) error {
		userID := c.Get("X-Test-User-ID")
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"email":   "test@example.com",
				"tier":    "free",
				"exp":     time.Now().Add(15 * time.Minute).Unix(),
			})
			signed, _ := tok.SignedString([]byte("test-secret"))
			parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
				return []byte("test-secret"), nil
			})
			c.Locals("user_token", parsed)
		}
		return c.Next()
	})

	app.Get("/videos", h.List)
	app.Get("/videos/:id", h.Get)
	app.Delete("/videos/:id", h.Delete)
	app.Post("/videos/:id/process", h.ProcessVideo)
	return app
}

func seedVideo(db *gorm.DB, userID uuid.UUID, status models.VideoStatus) models.Video {
	video := models.Video{
		Base:             models.Base{ID: uuid.New()},
		UserID:           userID,
		Title:            "Test Video",
		Description:      "Test description",
		OriginalFilename: "test.mp4",
		StoragePath:      "/storage/test.mp4",
		StorageURL:       "http://localhost/storage/test.mp4",
		FileSize:         1024,
		MimeType:         "video/mp4",
		Status:           status,
	}
	db.Create(&video)
	return video
}

// --- List ---

func TestVideoList_Authenticated(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	seedVideo(db, userID, models.VideoStatusPending)
	seedVideo(db, userID, models.VideoStatusCompleted)

	req, _ := http.NewRequest("GET", "/videos", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	videos := data["videos"].([]interface{})
	assert.Len(t, videos, 2)
}

func TestVideoList_Unauthenticated(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	req, _ := http.NewRequest("GET", "/videos", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestVideoList_FilterByStatus(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	seedVideo(db, userID, models.VideoStatusPending)
	seedVideo(db, userID, models.VideoStatusCompleted)

	req, _ := http.NewRequest("GET", "/videos?status=pending", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	videos := data["videos"].([]interface{})
	assert.Len(t, videos, 1)
}

func TestVideoList_OnlyReturnsOwnVideos(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	ownerID := uuid.New()
	otherID := uuid.New()
	seedVideo(db, ownerID, models.VideoStatusPending)
	seedVideo(db, otherID, models.VideoStatusPending)

	req, _ := http.NewRequest("GET", "/videos", nil)
	req.Header.Set("X-Test-User-ID", ownerID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	videos := data["videos"].([]interface{})
	assert.Len(t, videos, 1)
}

// --- Get ---

func TestVideoGet_Found(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusCompleted)

	req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, video.ID.String(), data["id"])
}

func TestVideoGet_NotFound(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()

	req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s", uuid.New()), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestVideoGet_OtherUsersVideo(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	ownerID := uuid.New()
	video := seedVideo(db, ownerID, models.VideoStatusCompleted)

	otherUserID := uuid.New()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s", video.ID), nil)
	req.Header.Set("X-Test-User-ID", otherUserID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- Delete ---

func TestVideoDelete_Success(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusCompleted)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/videos/%s", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify soft-deleted
	var count int64
	db.Unscoped().Model(&models.Video{}).Where("id = ?", video.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestVideoDelete_ProcessingVideoRejected(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusProcessing)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/videos/%s", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestVideoDelete_NotFound(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/videos/%s", uuid.New()), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

// --- ProcessVideo ---

func TestProcessVideo_TriggerSuccess(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusPending)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/videos/%s/process", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify status reset to pending so the worker pipeline can pick it up
	var updated models.Video
	db.First(&updated, "id = ?", video.ID)
	assert.Equal(t, models.VideoStatusPending, updated.Status)
}

func TestProcessVideo_AlreadyProcessing(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusProcessing)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/videos/%s/process", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestProcessVideo_AlreadyCompleted(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusCompleted)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/videos/%s/process", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestProcessVideo_FailedCanRetry(t *testing.T) {
	db := setupVideoTestDB(t)
	app := setupVideoApp(db)

	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusFailed)

	req, _ := http.NewRequest("POST", fmt.Sprintf("/videos/%s/process", video.ID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// mockResumableStorageService – ResumableStorageService backed by a real tracker
// ---------------------------------------------------------------------------

type mockResumableStorageService struct {
mockStorageService
tracker *storage.UploadProgressTracker
}

func newMockResumableStorageService() *mockResumableStorageService {
return &mockResumableStorageService{
tracker: storage.NewUploadProgressTracker(),
}
}

func (m *mockResumableStorageService) GetUploadProgress(uploadID string) (storage.UploadProgress, bool) {
return m.tracker.Get(uploadID)
}

// setupVideoAppWithResumable sets up a Fiber app with a VideoHandler that uses
// a ResumableStorageService, wiring up the GetUploadProgress route too.
func setupVideoAppWithResumable(db *gorm.DB, svc *mockResumableStorageService, userID string) *fiber.App {
app := fiber.New()
h := NewVideoHandler(db, svc)

app.Use(func(c *fiber.Ctx) error {
if userID != "" {
tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
"user_id": userID,
"exp":     time.Now().Add(30 * time.Minute).Unix(),
})
signed, _ := tok.SignedString([]byte("test-secret"))
parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
return []byte("test-secret"), nil
})
c.Locals("user_token", parsed)
}
return c.Next()
})

app.Get("/videos/:id/upload-progress", h.GetUploadProgress)
return app
}

// --- GetUploadProgress ---

func TestGetUploadProgress_Uploading(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
video := seedVideo(db, userID, models.VideoStatusPending)

svc := newMockResumableStorageService()
svc.tracker.SimulateStart(video.ID.String(), 10000)
svc.tracker.SimulateUpdate(video.ID.String(), 6700)

app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", video.ID), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusOK, resp.StatusCode)

var body map[string]interface{}
require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
data := body["data"].(map[string]interface{})
assert.Equal(t, "uploading", data["status"])
assert.InDelta(t, 67, data["progress"].(float64), 1)
assert.Equal(t, float64(6700), data["uploaded_bytes"].(float64))
assert.Equal(t, float64(10000), data["total_bytes"].(float64))
}

func TestGetUploadProgress_Completed(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
video := seedVideo(db, userID, models.VideoStatusPending)

svc := newMockResumableStorageService()
svc.tracker.SimulateStart(video.ID.String(), 5000)
svc.tracker.SimulateComplete(video.ID.String(), 5000)

app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", video.ID), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusOK, resp.StatusCode)

var body map[string]interface{}
require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
data := body["data"].(map[string]interface{})
assert.Equal(t, "completed", data["status"])
assert.Equal(t, float64(100), data["progress"].(float64))
}

func TestGetUploadProgress_Failed(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
video := seedVideo(db, userID, models.VideoStatusPending)

svc := newMockResumableStorageService()
svc.tracker.SimulateStart(video.ID.String(), 5000)
svc.tracker.SimulateFail(video.ID.String(), "network timeout")

app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", video.ID), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusOK, resp.StatusCode)

var body map[string]interface{}
require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
data := body["data"].(map[string]interface{})
assert.Equal(t, "failed", data["status"])
assert.Equal(t, "network timeout", data["error"])
}

func TestGetUploadProgress_NotFound_VideoMissing(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
svc := newMockResumableStorageService()
app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", uuid.New()), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestGetUploadProgress_NoContent_WhenNoTrackingEntry(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
video := seedVideo(db, userID, models.VideoStatusPending)

svc := newMockResumableStorageService()
// No tracker entry seeded – tracker returns (zero, false)
app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", video.ID), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}

func TestGetUploadProgress_NoContent_NonResumableBackend(t *testing.T) {
	db := setupVideoTestDB(t)
	userID := uuid.New()
	video := seedVideo(db, userID, models.VideoStatusPending)

	// Plain StorageService (not Resumable) – handler cannot type-assert to ResumableStorageService.
	h := NewVideoHandler(db, &mockStorageService{})

	app2 := fiber.New()
	app2.Use(func(c *fiber.Ctx) error {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": userID.String(),
			"exp":     time.Now().Add(30 * time.Minute).Unix(),
		})
		signed, _ := tok.SignedString([]byte("test-secret"))
		parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
			return []byte("test-secret"), nil
		})
		c.Locals("user_token", parsed)
		return c.Next()
	})
	app2.Get("/videos/:id/upload-progress", h.GetUploadProgress)

	req2, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", video.ID), nil)
	resp, err := app2.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
}

func TestGetUploadProgress_Unauthenticated(t *testing.T) {
db := setupVideoTestDB(t)
svc := newMockResumableStorageService()
app := setupVideoAppWithResumable(db, svc, "") // no user injected

req, _ := http.NewRequest("GET", fmt.Sprintf("/videos/%s/upload-progress", uuid.New()), nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestGetUploadProgress_InvalidID(t *testing.T) {
db := setupVideoTestDB(t)
userID := uuid.New()
svc := newMockResumableStorageService()
app := setupVideoAppWithResumable(db, svc, userID.String())

req, _ := http.NewRequest("GET", "/videos/not-a-uuid/upload-progress", nil)
resp, err := app.Test(req)
require.NoError(t, err)
assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// --- toUploadProgressResponse ---

func TestToUploadProgressResponse_CalculatesPercent(t *testing.T) {
p := storage.UploadProgress{
UploadID:      "uid-1",
TotalBytes:    200,
UploadedBytes: 100,
Status:        storage.UploadStatusUploading,
}
r := toUploadProgressResponse("uid-1", p)
assert.Equal(t, 50, r.Progress)
assert.Equal(t, "uploading", r.Status)
}

func TestToUploadProgressResponse_CompletedIs100(t *testing.T) {
p := storage.UploadProgress{
UploadID:      "uid-2",
TotalBytes:    0, // unknown size
UploadedBytes: 0,
Status:        storage.UploadStatusCompleted,
}
r := toUploadProgressResponse("uid-2", p)
assert.Equal(t, 100, r.Progress)
}

func TestToUploadProgressResponse_ZeroTotalReturnsZero(t *testing.T) {
p := storage.UploadProgress{
UploadID:      "uid-3",
TotalBytes:    0,
UploadedBytes: 512,
Status:        storage.UploadStatusUploading,
}
r := toUploadProgressResponse("uid-3", p)
assert.Equal(t, 0, r.Progress)
}
