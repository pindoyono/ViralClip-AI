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
	require.NoError(t, db.AutoMigrate(&Video{}, &Clip{}, &SocialAccount{}, &ScheduledPost{}, &PublishingLog{}))
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
	assert.Equal(t, 3, w.maxRetries)
}

func TestNewSchedulerWorker(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewSchedulerWorker(db, nil)
	assert.NotNil(t, w)
}

func TestSchedulerWorker_EnqueueDuePosts(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewSchedulerWorker(db, nil)

	now := time.Now().UTC().Add(-1 * time.Minute)
	post := ScheduledPost{
		ID:              "sched-1",
		ClipID:          "clip-1",
		UserID:          "user-1",
		SocialAccountID: "acc-1",
		Platform:        "tiktok",
		ScheduledAt:     now,
		PublishAt:       &now,
		Status:          "scheduled",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	require.NoError(t, db.Create(&post).Error)

	w.EnqueueDuePosts(context.Background())

	var updated ScheduledPost
	require.NoError(t, db.First(&updated, "id = ?", "sched-1").Error)
	assert.Equal(t, "publishing", updated.Status)
	assert.Equal(t, 0, updated.UploadProgress)

	var logs []PublishingLog
	require.NoError(t, db.Where("post_id = ?", "sched-1").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "publishing", logs[0].Status)
	assert.Equal(t, "post queued for publishing", logs[0].Message)
}

func TestPublishingWorker_ProcessScheduledPosts_Success(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewPublishingWorker(db, nil)

	exp := time.Now().UTC().Add(1 * time.Hour)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-1",
		UserID:         "user-1",
		Platform:       "tiktok",
		AccessToken:    "token-ok",
		RefreshToken:   "refresh-ok",
		TokenExpiresAt: &exp,
		IsActive:       true,
	}).Error)

	require.NoError(t, db.Create(&Clip{
		ID:          "clip-1",
		VideoID:     "video-1",
		UserID:      "user-1",
		Status:      "ready",
		StorageURL:  "https://storage.example.com/clip-1.mp4",
		StoragePath: "/storage/clip-1.mp4",
	}).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Create(&ScheduledPost{
		ID:              "post-1",
		ClipID:          "clip-1",
		UserID:          "user-1",
		SocialAccountID: "acc-1",
		Platform:        "tiktok",
		ScheduledAt:     now,
		PublishAt:       &now,
		Status:          "publishing",
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	w.ProcessScheduledPosts(context.Background())

	var updated ScheduledPost
	require.NoError(t, db.First(&updated, "id = ?", "post-1").Error)
	assert.Equal(t, "published", updated.Status)
	assert.NotEmpty(t, updated.PlatformPostID)

	var logs []PublishingLog
	require.NoError(t, db.Where("post_id = ?", "post-1").Find(&logs).Error)
	assert.GreaterOrEqual(t, len(logs), 2)
}

func TestPublishingWorker_ProcessScheduledPosts_RefreshesExpiredToken(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewPublishingWorker(db, nil)

	expired := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-expired",
		UserID:         "user-1",
		Platform:       "youtube",
		AccessToken:    "old-token",
		RefreshToken:   "refresh-xyz",
		TokenExpiresAt: &expired,
		IsActive:       true,
	}).Error)

	require.NoError(t, db.Create(&Clip{
		ID:          "clip-1",
		VideoID:     "video-1",
		UserID:      "user-1",
		Status:      "ready",
		StorageURL:  "https://storage.example.com/clip-1.mp4",
		StoragePath: "/storage/clip-1.mp4",
	}).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Create(&ScheduledPost{
		ID:              "post-expired",
		ClipID:          "clip-1",
		UserID:          "user-1",
		SocialAccountID: "acc-expired",
		Platform:        "youtube",
		ScheduledAt:     now,
		PublishAt:       &now,
		Status:          "publishing",
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	w.ProcessScheduledPosts(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-expired").Error)
	assert.Contains(t, account.AccessToken, "refreshed_")

	var post ScheduledPost
	require.NoError(t, db.First(&post, "id = ?", "post-expired").Error)
	assert.Equal(t, "published", post.Status)
}

func TestPublishingWorker_ProcessScheduledPosts_RetriesWhenNoToken(t *testing.T) {
	db := setupWorkerDB(t)
	w := NewPublishingWorker(db, nil)

	require.NoError(t, db.Create(&SocialAccount{
		ID:           "acc-missing-token",
		UserID:       "user-1",
		Platform:     "instagram",
		AccessToken:  "",
		RefreshToken: "",
		IsActive:     true,
	}).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Create(&ScheduledPost{
		ID:              "post-retry",
		ClipID:          "clip-1",
		UserID:          "user-1",
		SocialAccountID: "acc-missing-token",
		Platform:        "instagram",
		ScheduledAt:     now,
		PublishAt:       &now,
		Status:          "publishing",
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	w.ProcessScheduledPosts(context.Background())

	var post ScheduledPost
	require.NoError(t, db.First(&post, "id = ?", "post-retry").Error)
	assert.Equal(t, "scheduled", post.Status)
	assert.Equal(t, 1, post.RetryCount)
	assert.NotEmpty(t, post.ErrorMessage)
}

func TestSchedulerAndPublishingWorkers_EndToEnd(t *testing.T) {
	db := setupWorkerDB(t)
	scheduler := NewSchedulerWorker(db, nil)
	publisher := NewPublishingWorker(db, nil)

	exp := time.Now().UTC().Add(1 * time.Hour)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-e2e",
		UserID:         "user-1",
		Platform:       "tiktok",
		AccessToken:    "token-ok",
		RefreshToken:   "refresh-ok",
		TokenExpiresAt: &exp,
		IsActive:       true,
	}).Error)
	require.NoError(t, db.Create(&Clip{
		ID:          "clip-e2e",
		VideoID:     "video-e2e",
		UserID:      "user-1",
		Status:      "ready",
		StorageURL:  "https://storage.example.com/clip-e2e.mp4",
		StoragePath: "/storage/clip-e2e.mp4",
	}).Error)

	dueAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&ScheduledPost{
		ID:              "post-e2e",
		ClipID:          "clip-e2e",
		UserID:          "user-1",
		SocialAccountID: "acc-e2e",
		Platform:        "tiktok",
		ScheduledAt:     dueAt,
		PublishAt:       &dueAt,
		Status:          "scheduled",
		CreatedAt:       dueAt,
		UpdatedAt:       dueAt,
	}).Error)

	scheduler.EnqueueDuePosts(context.Background())
	publisher.ProcessScheduledPosts(context.Background())

	var post ScheduledPost
	require.NoError(t, db.First(&post, "id = ?", "post-e2e").Error)
	assert.Equal(t, "published", post.Status)
	assert.NotNil(t, post.PublishedAt)
	assert.NotEmpty(t, post.PlatformPostID)
	assert.NotEmpty(t, post.PlatformPostURL)
	assert.Equal(t, 100, post.UploadProgress)

	var logs []PublishingLog
	require.NoError(t, db.Where("post_id = ?", "post-e2e").Order("created_at ASC").Find(&logs).Error)
	require.Len(t, logs, 3)
	assert.Equal(t, "post queued for publishing", logs[0].Message)
	assert.Equal(t, "publishing started", logs[1].Message)
	assert.Equal(t, "post published successfully", logs[2].Message)
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
