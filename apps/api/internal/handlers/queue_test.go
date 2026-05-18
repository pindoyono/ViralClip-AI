package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupQueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.FailedJob{}))
	return db
}

func setupQueueApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()
	db := setupQueueTestDB(t)
	rdb, _ := newTestRedis(t) // defined in status_test.go (same package)

	svc := services.NewQueueMetricsService(db, rdb)
	h := NewQueueHandler(svc)

	app := fiber.New()
	app.Get("/queue/status", h.Status)
	app.Get("/queue/failed", h.Failed)
	app.Get("/queue/retry", h.Retry)

	return app, db
}

func seedAPIFailedJob(t *testing.T, db *gorm.DB, queueName, status string) models.FailedJob {
	t.Helper()
	j := models.FailedJob{
		ID:           uuid.New(),
		JobID:        uuid.New().String(),
		QueueName:    queueName,
		Payload:      `{"id":"x"}`,
		ErrorMessage: "error",
		RetryCount:   1,
		MaxRetries:   3,
		Status:       models.FailedJobStatus(status),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	require.NoError(t, db.Create(&j).Error)
	return j
}

// ---------------------------------------------------------------------------
// GET /queue/status
// ---------------------------------------------------------------------------

func TestQueueHandler_Status_OK(t *testing.T) {
	app, _ := setupQueueApp(t)

	req := httptest.NewRequest(http.MethodGet, "/queue/status", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))

	data := body["data"].(map[string]interface{})
	assert.NotNil(t, data["collected_at"])
	assert.NotNil(t, data["queues"])
	assert.NotNil(t, data["failed_jobs"])
}

// ---------------------------------------------------------------------------
// GET /queue/failed
// ---------------------------------------------------------------------------

func TestQueueHandler_Failed_EmptyDB(t *testing.T) {
	app, _ := setupQueueApp(t)

	req := httptest.NewRequest(http.MethodGet, "/queue/failed", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["total"])
}

func TestQueueHandler_Failed_WithRecords(t *testing.T) {
	app, db := setupQueueApp(t)

	seedAPIFailedJob(t, db, "transcript_queue", "pending")
	seedAPIFailedJob(t, db, "clip_queue", "pending")
	seedAPIFailedJob(t, db, "transcript_queue", "exhausted")

	req := httptest.NewRequest(http.MethodGet, "/queue/failed", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(3), data["total"])
}

func TestQueueHandler_Failed_FilterByQueue(t *testing.T) {
	app, db := setupQueueApp(t)

	seedAPIFailedJob(t, db, "transcript_queue", "pending")
	seedAPIFailedJob(t, db, "clip_queue", "pending")

	req := httptest.NewRequest(http.MethodGet, "/queue/failed?queue=transcript_queue", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total"])
}

func TestQueueHandler_Failed_FilterByStatus(t *testing.T) {
	app, db := setupQueueApp(t)

	seedAPIFailedJob(t, db, "transcript_queue", "pending")
	seedAPIFailedJob(t, db, "transcript_queue", "exhausted")

	req := httptest.NewRequest(http.MethodGet, "/queue/failed?status=exhausted", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total"])
}

func TestQueueHandler_Failed_Pagination(t *testing.T) {
	app, db := setupQueueApp(t)

	for i := 0; i < 5; i++ {
		seedAPIFailedJob(t, db, "transcript_queue", "pending")
	}

	req := httptest.NewRequest(http.MethodGet, "/queue/failed?limit=2&offset=0", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, float64(5), data["total"])
	jobs := data["jobs"].([]interface{})
	assert.Len(t, jobs, 2)
}

// ---------------------------------------------------------------------------
// GET /queue/retry
// ---------------------------------------------------------------------------

func TestQueueHandler_Retry_OnlyRetryableJobs(t *testing.T) {
	app, db := setupQueueApp(t)

	seedAPIFailedJob(t, db, "transcript_queue", "pending")
	seedAPIFailedJob(t, db, "clip_queue", "recovering")
	seedAPIFailedJob(t, db, "upload_queue", "exhausted")

	req := httptest.NewRequest(http.MethodGet, "/queue/retry", nil)
	resp, err := app.Test(req, 3000)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	// Only pending and recovering (not exhausted).
	assert.Equal(t, float64(2), data["total"])
}
