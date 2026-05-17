package workers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Video{}, &Clip{}))
	return db
}

// --- VideoProcessingWorker construction ---

func TestNewVideoProcessingWorker(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewVideoProcessingWorker(db, nil, "http://ai-service:8001", 3)

	assert.NotNil(t, w)
	assert.Equal(t, "http://ai-service:8001", w.aiURL)
	assert.Equal(t, 3, w.maxRetries)
	assert.NotNil(t, w.httpClient)
}

func TestNewVideoProcessingWorker_DefaultHTTPTimeout(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewVideoProcessingWorker(db, nil, "http://ai-service:8001", 5)

	assert.Equal(t, 300*time.Second, w.httpClient.Timeout)
}

// --- PublishingWorker ---

func TestNewPublishingWorker(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewPublishingWorker(db, nil)

	assert.NotNil(t, w)
	assert.Equal(t, 60*time.Second, w.httpClient.Timeout)
}

// --- CleanupWorker ---

func TestNewCleanupWorker(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewCleanupWorker(db, nil)

	assert.NotNil(t, w)
}

// --- AnalyticsWorker ---

func TestNewAnalyticsWorker(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewAnalyticsWorker(db, nil)

	assert.NotNil(t, w)
	assert.Equal(t, 30*time.Second, w.httpClient.Timeout)
}

// --- ProcessPendingVideos with no videos ---

func TestProcessPendingVideos_NoPendingVideos(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewVideoProcessingWorker(db, nil, "http://ai:8001", 3)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should not panic with empty DB
	assert.NotPanics(t, func() {
		w.ProcessPendingVideos(ctx)
	})
}

// --- ProcessPendingVideos sends request to AI service ---

func TestProcessPendingVideos_CallsAIService(t *testing.T) {
	db := setupWorkerDB(t)

	// Seed a pending video
	video := Video{
		ID:          "vid-test-123",
		UserID:      "user-abc",
		StoragePath: "/storage/test.mp4",
		Status:      VideoStatusPending,
	}
	db.Create(&video)

	// Set up a mock AI service server
	called := false
	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, "/process/video", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAI.Close()

	w := NewVideoProcessingWorker(db, nil, mockAI.URL, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.ProcessPendingVideos(ctx)

	assert.True(t, called, "expected AI service to be called")

	// Verify video status updated to completed
	var updated Video
	db.First(&updated, "id = ?", video.ID)
	assert.Equal(t, VideoStatusCompleted, updated.Status)
}

// --- markVideoFailed ---

func TestMarkVideoFailed_UpdatesStatus(t *testing.T) {
	db := setupWorkerDB(t)

	video := Video{
		ID:     "vid-fail-test",
		UserID: "user-1",
		Status: VideoStatusProcessing,
	}
	db.Create(&video)

	w := NewVideoProcessingWorker(db, nil, "http://ai:8001", 3)
	w.markVideoFailed(video.ID, "some processing error")

	var updated Video
	db.First(&updated, "id = ?", video.ID)
	assert.Equal(t, VideoStatusFailed, updated.Status)
	assert.Equal(t, "some processing error", updated.ErrorMessage)
}

// --- CleanupOldData ---

func TestCleanupOldData_DoesNotPanic(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewCleanupWorker(db, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NotPanics(t, func() {
		w.CleanupOldData(ctx)
	})
}

// --- SyncAnalytics ---

func TestSyncAnalytics_DoesNotPanic(t *testing.T) {
	db := setupWorkerDB(t)
	// Create scheduled_posts table to avoid error
	db.Exec(`CREATE TABLE IF NOT EXISTS scheduled_posts (
		id TEXT PRIMARY KEY,
		clip_id TEXT,
		platform TEXT,
		platform_post_id TEXT,
		social_account_id TEXT,
		status TEXT
	)`)

	w := NewAnalyticsWorker(db, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.NotPanics(t, func() {
		w.SyncAnalytics(ctx)
	})
}

// --- Video/Clip TableName ---

func TestVideoTableName(t *testing.T) {
	v := Video{}
	assert.Equal(t, "videos", v.TableName())
}

func TestClipTableName(t *testing.T) {
	c := Clip{}
	assert.Equal(t, "clips", c.TableName())
}

// --- Context cancellation ---

func TestProcessPendingVideos_ContextCancelled(t *testing.T) {
	db := setupWorkerDB(t)

	// Seed a pending video
	db.Create(&Video{
		ID:          "vid-ctx-test",
		UserID:      "user-x",
		StoragePath: "/storage/x.mp4",
		Status:      VideoStatusPending,
	})

	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // intentionally slow
	}))
	defer slowServer.Close()

	w := NewVideoProcessingWorker(db, nil, slowServer.URL, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	w.ProcessPendingVideos(ctx)
	elapsed := time.Since(start)

	// Should return quickly due to context cancellation or HTTP timeout
	assert.Less(t, elapsed, 10*time.Second)
}
