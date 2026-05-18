package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/websocket"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const testJWTSecret = "status-test-secret-32bytes-long!!"

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, mr
}

func signTestJWT(userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"email":   "test@example.com",
		"tier":    "free",
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
	})
	signed, _ := tok.SignedString([]byte(testJWTSecret))
	return signed
}

// setupStatusApp builds a minimal Fiber app with the StatusHandler wired in.
// It injects the authenticated user via X-Test-User-ID header (same pattern as
// other handler tests in this package).
func setupStatusApp(db *gorm.DB, rdb *redis.Client) (*fiber.App, *StatusHandler) {
	hub := websocket.NewHub()
	go hub.Run()

	h := NewStatusHandler(db, rdb, hub, testJWTSecret)

	app := fiber.New()

	// Inject a fake JWT claim for tests.
	app.Use(func(c *fiber.Ctx) error {
		userID := c.Get("X-Test-User-ID")
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"email":   "test@example.com",
				"tier":    "free",
				"exp":     time.Now().Add(15 * time.Minute).Unix(),
			})
			signed, _ := tok.SignedString([]byte(testJWTSecret))
			parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
				return []byte(testJWTSecret), nil
			})
			c.Locals("user_token", parsed)
		}
		return c.Next()
	})

	app.Get("/api/v1/videos/:id/job-status", h.GetJobStatus)

	return app, h
}

// parseStatusResponse decodes the JSON body into a dto.JobStatusResponse.
func parseStatusResponse(t *testing.T, resp *http.Response) dto.JobStatusResponse {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var wrapper struct {
		Data dto.JobStatusResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &wrapper))
	return wrapper.Data
}

// ---------------------------------------------------------------------------
// Unit tests: buildJobStatusResponse
// ---------------------------------------------------------------------------

func TestBuildJobStatusResponse_Pending(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "pending", "")
	assert.Equal(t, "pending", r.VideoStatus)
	assert.Equal(t, dto.PipelineStageTranscript, r.CurrentStage)
	for _, s := range r.Stages {
		assert.Equal(t, dto.StageStatusPending, s.Status)
	}
}

func TestBuildJobStatusResponse_Processing_TranscriptStage(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "processing", "transcript:processing")
	assert.Equal(t, dto.PipelineStageTranscript, r.CurrentStage)
	assert.Equal(t, dto.StageStatusProcessing, r.Stages[0].Status) // transcript
	assert.Equal(t, dto.StageStatusPending, r.Stages[1].Status)    // clip
}

func TestBuildJobStatusResponse_Processing_ClipStage(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "processing", "clip:processing")
	assert.Equal(t, dto.PipelineStageClip, r.CurrentStage)
	assert.Equal(t, dto.StageStatusDone, r.Stages[0].Status)       // transcript done
	assert.Equal(t, dto.StageStatusProcessing, r.Stages[1].Status) // clip
}

func TestBuildJobStatusResponse_Processing_SubtitleStage(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "processing", "subtitle:processing")
	assert.Equal(t, dto.PipelineStageSubtitle, r.CurrentStage)
	assert.Equal(t, dto.StageStatusProcessing, r.Stages[2].Status)
}

func TestBuildJobStatusResponse_Processing_UploadStage(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "processing", "upload:processing")
	assert.Equal(t, dto.PipelineStageUpload, r.CurrentStage)
	assert.Equal(t, dto.StageStatusProcessing, r.Stages[3].Status)
}

func TestBuildJobStatusResponse_Completed(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "completed", "upload:done")
	assert.Equal(t, dto.PipelineStageCompleted, r.CurrentStage)
	for _, s := range r.Stages {
		assert.Equal(t, dto.StageStatusDone, s.Status)
	}
}

func TestBuildJobStatusResponse_Failed(t *testing.T) {
	r := buildJobStatusResponse("vid-1", "failed", "clip:processing")
	assert.Equal(t, dto.PipelineStageClip, r.CurrentStage)
	assert.Equal(t, dto.StageStatusDone, r.Stages[0].Status)   // transcript
	assert.Equal(t, dto.StageStatusFailed, r.Stages[1].Status) // clip
}

// ---------------------------------------------------------------------------
// HTTP integration tests: GetJobStatus
// ---------------------------------------------------------------------------

func TestGetJobStatus_Unauthenticated(t *testing.T) {
	db := setupSubtitleTestDB(t)
	app, _ := setupStatusApp(db, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", uuid.New()), nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetJobStatus_InvalidVideoID(t *testing.T) {
	db := setupSubtitleTestDB(t)
	app, _ := setupStatusApp(db, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/not-a-uuid/job-status", nil)
	req.Header.Set("X-Test-User-ID", "user-1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetJobStatus_VideoNotFound(t *testing.T) {
	db := setupSubtitleTestDB(t)
	app, _ := setupStatusApp(db, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", uuid.New()), nil)
	req.Header.Set("X-Test-User-ID", "user-1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetJobStatus_VideoOwnedByOtherUser(t *testing.T) {
	db := setupSubtitleTestDB(t)
	ownerID := uuid.New()
	videoID := uuid.New()

	db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: ownerID,
		Title:  "Other User's Video",
		Status: models.VideoStatusPending,
	})

	app, _ := setupStatusApp(db, nil)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", videoID), nil)
	req.Header.Set("X-Test-User-ID", uuid.New().String()) // different user
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetJobStatus_PendingVideo_NoRedis(t *testing.T) {
	db := setupSubtitleTestDB(t)
	userID := uuid.New()
	videoID := uuid.New()

	db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: userID,
		Title:  "Test Video",
		Status: models.VideoStatusPending,
	})

	app, _ := setupStatusApp(db, nil) // nil Redis → job_status = ""

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", videoID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseStatusResponse(t, resp)
	assert.Equal(t, videoID.String(), data.VideoID)
	assert.Equal(t, "pending", data.VideoStatus)
	assert.Equal(t, dto.PipelineStageTranscript, data.CurrentStage)
	assert.Len(t, data.Stages, 4)
}

func TestGetJobStatus_ProcessingVideo_WithRedis(t *testing.T) {
	db := setupSubtitleTestDB(t)
	rdb, mr := newTestRedis(t)
	userID := uuid.New()
	videoID := uuid.New()

	db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: userID,
		Title:  "Test Video",
		Status: models.VideoStatusProcessing,
	})

	// Simulate what the worker writes.
	mr.Set("job:"+videoID.String(), "clip:processing")

	app, _ := setupStatusApp(db, rdb)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", videoID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseStatusResponse(t, resp)
	assert.Equal(t, "processing", data.VideoStatus)
	assert.Equal(t, "clip:processing", data.JobStatus)
	assert.Equal(t, dto.PipelineStageClip, data.CurrentStage)
	assert.Equal(t, dto.StageStatusDone, data.Stages[0].Status)       // transcript done
	assert.Equal(t, dto.StageStatusProcessing, data.Stages[1].Status) // clip active
}

func TestGetJobStatus_CompletedVideo(t *testing.T) {
	db := setupSubtitleTestDB(t)
	rdb, mr := newTestRedis(t)
	userID := uuid.New()
	videoID := uuid.New()

	db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: userID,
		Title:  "Done Video",
		Status: models.VideoStatusCompleted,
	})

	mr.Set("job:"+videoID.String(), "upload:done")

	app, _ := setupStatusApp(db, rdb)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%s/job-status", videoID), nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data := parseStatusResponse(t, resp)
	assert.Equal(t, "completed", data.VideoStatus)
	assert.Equal(t, dto.PipelineStageCompleted, data.CurrentStage)
	for _, s := range data.Stages {
		assert.Equal(t, dto.StageStatusDone, s.Status)
	}
}

// ---------------------------------------------------------------------------
// WSUpgrade tests
// ---------------------------------------------------------------------------

func TestWSUpgrade_MissingToken(t *testing.T) {
	db := setupSubtitleTestDB(t)
	hub := websocket.NewHub()
	go hub.Run()
	h := NewStatusHandler(db, nil, hub, testJWTSecret)
	app := fiber.New()
	app.Get("/ws", h.WSUpgrade)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWSUpgrade_InvalidToken(t *testing.T) {
	db := setupSubtitleTestDB(t)
	hub := websocket.NewHub()
	go hub.Run()
	h := NewStatusHandler(db, nil, hub, testJWTSecret)
	app := fiber.New()
	app.Get("/ws", h.WSUpgrade)

	req := httptest.NewRequest(http.MethodGet, "/ws?token=not.a.valid.jwt", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestWSUpgrade_ValidToken_NotWebSocketRequest(t *testing.T) {
	db := setupSubtitleTestDB(t)
	hub := websocket.NewHub()
	go hub.Run()
	h := NewStatusHandler(db, nil, hub, testJWTSecret)
	app := fiber.New()
	app.Get("/ws", h.WSUpgrade)

	token := signTestJWT("user-123")
	req := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// No WS upgrade headers → 426 Upgrade Required
	assert.Equal(t, http.StatusUpgradeRequired, resp.StatusCode)
}
