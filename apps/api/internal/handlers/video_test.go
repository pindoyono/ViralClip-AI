package handlers

import (
	"encoding/json"
	"fmt"
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
)

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
	h := NewVideoHandler(db, "/tmp/storage", "http://localhost/storage")

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

	// Verify status updated
	var updated models.Video
	db.First(&updated, "id = ?", video.ID)
	assert.Equal(t, models.VideoStatusProcessing, updated.Status)
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
