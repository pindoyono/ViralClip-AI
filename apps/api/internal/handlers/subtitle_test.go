package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// ---------------------------------------------------------------------------
// Helpers shared across subtitle handler tests
// ---------------------------------------------------------------------------

func setupSubtitleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Video{}, &models.Clip{}, &models.User{}))
	return db
}

// setupSubtitleApp builds a Fiber test app backed by a fake AI server.
// The aiServer is a *httptest.Server whose URL is injected into the handler.
func setupSubtitleApp(db *gorm.DB, aiServer *httptest.Server) *fiber.App {
	cfg := &config.Config{}
	cfg.AI.ServiceURL = aiServer.URL
	cfg.AI.Timeout = 10 * time.Second

	app := fiber.New()

	// Inject a fake JWT for tests via X-Test-User-ID header.
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

	h := NewSubtitleHandler(db, cfg)
	app.Post("/videos/:videoId/subtitles/burn", h.BurnSubtitles)
	return app
}

// seedCompletedVideo creates a completed video record owned by userID.
func seedCompletedVideo(db *gorm.DB, userID uuid.UUID) models.Video {
	video := models.Video{
		Base:         models.Base{ID: uuid.New()},
		UserID:       userID,
		Title:        "Subtitle Test Video",
		StoragePath:  "/storage/test.mp4",
		StorageURL:   "http://localhost/storage/test.mp4",
		FileSize:     1024,
		MimeType:     "video/mp4",
		Status:       models.VideoStatusCompleted,
	}
	db.Create(&video)
	return video
}

// seedClipForVideo creates a clip attached to video.
func seedClipForVideo(db *gorm.DB, videoID, userID uuid.UUID) models.Clip {
	clip := models.Clip{
		Base:    models.Base{ID: uuid.New()},
		VideoID: videoID,
		UserID:  userID,
		Title:   "Test Clip",
		Status:  models.ClipStatusReady,
	}
	db.Create(&clip)
	return clip
}

// fakeAIServer creates an httptest.Server that mimics the AI subtitle endpoint.
func fakeAIServer(t *testing.T, statusCode int, responseBody interface{}) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/process/subtitles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if responseBody != nil {
			_ = json.NewEncoder(w).Encode(responseBody)
		}
	})
	return httptest.NewServer(mux)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBurnSubtitles_Unauthenticated(t *testing.T) {
	db := setupSubtitleTestDB(t)
	srv := fakeAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	app := setupSubtitleApp(db, srv)
	req, _ := http.NewRequest("POST", "/videos/"+uuid.New().String()+"/subtitles/burn", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBurnSubtitles_InvalidVideoID(t *testing.T) {
	db := setupSubtitleTestDB(t)
	srv := fakeAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	app := setupSubtitleApp(db, srv)
	userID := uuid.New()
	req, _ := http.NewRequest("POST", "/videos/not-a-uuid/subtitles/burn", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBurnSubtitles_VideoNotFound(t *testing.T) {
	db := setupSubtitleTestDB(t)
	srv := fakeAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	app := setupSubtitleApp(db, srv)
	userID := uuid.New()
	req, _ := http.NewRequest("POST", "/videos/"+uuid.New().String()+"/subtitles/burn", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBurnSubtitles_VideoNotCompleted(t *testing.T) {
	db := setupSubtitleTestDB(t)
	srv := fakeAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	userID := uuid.New()
	video := models.Video{
		Base:        models.Base{ID: uuid.New()},
		UserID:      userID,
		Title:       "Pending Video",
		StoragePath: "/storage/pending.mp4",
		FileSize:    1024,
		MimeType:    "video/mp4",
		Status:      models.VideoStatusPending,
	}
	db.Create(&video)

	app := setupSubtitleApp(db, srv)
	req, _ := http.NewRequest("POST", "/videos/"+video.ID.String()+"/subtitles/burn", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBurnSubtitles_AIServiceError(t *testing.T) {
	db := setupSubtitleTestDB(t)
	srv := fakeAIServer(t, http.StatusInternalServerError, map[string]string{"detail": "ffmpeg failed"})
	defer srv.Close()

	userID := uuid.New()
	video := seedCompletedVideo(db, userID)

	app := setupSubtitleApp(db, srv)
	req, _ := http.NewRequest("POST", "/videos/"+video.ID.String()+"/subtitles/burn", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestBurnSubtitles_Success(t *testing.T) {
	db := setupSubtitleTestDB(t)

	aiResp := map[string]interface{}{
		"video_id":        "will-be-set",
		"clips_processed": 3,
	}
	srv := fakeAIServer(t, http.StatusOK, aiResp)
	defer srv.Close()

	userID := uuid.New()
	video := seedCompletedVideo(db, userID)
	clip1 := seedClipForVideo(db, video.ID, userID)
	clip2 := seedClipForVideo(db, video.ID, userID)

	app := setupSubtitleApp(db, srv)
	req, _ := http.NewRequest("POST", "/videos/"+video.ID.String()+"/subtitles/burn", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Validate response body.
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			VideoID        string `json:"video_id"`
			ClipsProcessed int    `json:"clips_processed"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.Success)
	assert.Equal(t, video.ID.String(), body.Data.VideoID)
	assert.Equal(t, 3, body.Data.ClipsProcessed)

	// Verify clips now have subtitle_path set (has_subtitles = true).
	var updated1, updated2 models.Clip
	db.First(&updated1, "id = ?", clip1.ID)
	db.First(&updated2, "id = ?", clip2.ID)
	assert.NotEmpty(t, updated1.SubtitlePath)
	assert.NotEmpty(t, updated2.SubtitlePath)
}

func TestBurnSubtitles_WithStyleOptions(t *testing.T) {
	db := setupSubtitleTestDB(t)

	// Capture the request payload sent to the AI service.
	var captured map[string]interface{}
	mux := http.NewServeMux()
	mux.HandleFunc("/process/subtitles", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"video_id":        "v1",
			"clips_processed": 1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	userID := uuid.New()
	video := seedCompletedVideo(db, userID)

	app := setupSubtitleApp(db, srv)

	payload := map[string]interface{}{
		"style":         "bold",
		"font_size":     32,
		"primary_color": "&H0000FFFF",
		"outline_color": "&H00000000",
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/videos/%s/subtitles/burn", video.ID),
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", userID.String())

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify style options were forwarded to the AI service.
	assert.Equal(t, "bold", captured["style"])
	assert.EqualValues(t, 32, captured["font_size"])
	assert.Equal(t, "&H0000FFFF", captured["primary_color"])
}

func TestBurnSubtitles_HasSubtitlesFalseByDefault(t *testing.T) {
	db := setupSubtitleTestDB(t)

	userID := uuid.New()
	video := seedCompletedVideo(db, userID)
	clip := seedClipForVideo(db, video.ID, userID)

	// subtitle_path should be empty before burning.
	var fresh models.Clip
	db.First(&fresh, "id = ?", clip.ID)
	assert.Empty(t, fresh.SubtitlePath)
}
