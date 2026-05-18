package workers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

func setupQueueWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Video{}, &Clip{}))
	return db
}

func newTestQueueClient(t *testing.T) (*queue.QueueClient, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return queue.NewQueueClient(rdb, time.Minute), mr
}

func seedVideoForQueue(db *gorm.DB, id string, status VideoStatus) Video {
	v := Video{ID: id, UserID: "user-1", StoragePath: "/storage/" + id + ".mp4", Status: status}
	db.Create(&v)
	return v
}

// ---------------------------------------------------------------------------
// TranscriptWorker
// ---------------------------------------------------------------------------

func TestNewTranscriptWorker(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	w := NewTranscriptWorker(db, qCli, "http://ai:8000", 3)
	assert.NotNil(t, w)
}

func TestTranscriptWorker_ProcessJob_Success(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-transcript-1", VideoStatusPending)

	aiCalled := false
	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalled = true
		assert.Equal(t, "/process/transcript", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer mockAI.Close()

	w := NewTranscriptWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "vid-transcript-1", Type: queue.JobTypeTranscript,
		VideoID: "vid-transcript-1", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	assert.True(t, aiCalled)

	// Job must be forwarded to clip_queue.
	nextJob, err := qCli.BlockingPop(ctx, queue.QueueClip, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, nextJob)
	assert.Equal(t, queue.JobTypeClip, nextJob.Type)
}

func TestTranscriptWorker_ProcessJob_AIFailure_Retries(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-transcript-2", VideoStatusPending)

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockAI.Close()

	w := NewTranscriptWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "vid-transcript-2", Type: queue.JobTypeTranscript,
		VideoID: "vid-transcript-2", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3, RetryCount: 0,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	// Job should be re-queued in transcript_queue (retry count 1 < max 3).
	requeued, err := qCli.BlockingPop(ctx, queue.QueueTranscript, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, requeued)
	assert.Equal(t, 1, requeued.RetryCount)
}

func TestTranscriptWorker_ProcessJob_ExceedsMaxRetries_GoesToDLQ(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-transcript-3", VideoStatusPending)

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockAI.Close()

	w := NewTranscriptWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "vid-transcript-3", Type: queue.JobTypeTranscript,
		VideoID: "vid-transcript-3", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3, RetryCount: 2,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	// Main queue must be empty.
	n, err := qCli.QueueLength(ctx, queue.QueueTranscript)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	// Dead-letter queue must have the job.
	dn, err := qCli.DeadQueueLength(ctx, queue.QueueTranscript)
	require.NoError(t, err)
	assert.Equal(t, int64(1), dn)

	// Video must be marked failed.
	var vid Video
	db.First(&vid, "id = ?", "vid-transcript-3")
	assert.Equal(t, VideoStatusFailed, vid.Status)
}

func TestTranscriptWorker_Start_StopsOnContextCancel(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)

	w := NewTranscriptWorker(db, qCli, "http://ai:8000", 3)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("TranscriptWorker.Start did not stop after context cancel")
	}
}

// ---------------------------------------------------------------------------
// ClipWorker
// ---------------------------------------------------------------------------

func TestNewClipWorker(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	w := NewClipWorker(db, qCli, "http://ai:8000", 3)
	assert.NotNil(t, w)
}

func TestClipWorker_ProcessJob_Success_PushesToSubtitleQueue(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-clip-1", VideoStatusProcessing)

	clipsPayload := map[string]interface{}{
		"video_id": "vid-clip-1",
		"clips": []map[string]interface{}{
			{
				"index":           0,
				"storage_path":    "/clips/vid-clip-1/clip_000.mp4",
				"start_time":      5.0,
				"end_time":        35.0,
				"duration":        30.0,
				"viral_score":     0.88,
				"rationale":       "Great hook",
				"hook_text":       "You won't believe this",
				"suggested_title": "Amazing Clip",
				"hashtags":        []string{"#viral"},
				"suggested_for":   []string{"tiktok"},
			},
		},
		"manifest_path":   "/clips/vid-clip-1_manifest.json",
		"processing_time": 2.5,
	}

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/process/clips", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(clipsPayload)
	}))
	defer mockAI.Close()

	w := NewClipWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "job-clip-1", Type: queue.JobTypeClip,
		VideoID: "vid-clip-1", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	// Job advanced to subtitle_queue.
	next, err := qCli.BlockingPop(ctx, queue.QueueSubtitle, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, queue.JobTypeSubtitle, next.Type)

	// Clip record must be created in DB.
	var clips []Clip
	db.Where("video_id = ?", "vid-clip-1").Find(&clips)
	require.Len(t, clips, 1)
	assert.Equal(t, "Amazing Clip", clips[0].Title)
	assert.Equal(t, "user-1", clips[0].UserID)
	assert.InDelta(t, 0.88, clips[0].ViralScore, 0.001)
	assert.Equal(t, "generating", clips[0].Status)
}

// ---------------------------------------------------------------------------
// SubtitleWorker
// ---------------------------------------------------------------------------

func TestNewSubtitleWorker(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	w := NewSubtitleWorker(db, qCli, "http://ai:8000", 3)
	assert.NotNil(t, w)
}

func TestSubtitleWorker_ProcessJob_Success_PushesToUploadQueue(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-sub-1", VideoStatusProcessing)

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/process/subtitles", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAI.Close()

	w := NewSubtitleWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "job-sub-1", Type: queue.JobTypeSubtitle,
		VideoID: "vid-sub-1", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	next, err := qCli.BlockingPop(ctx, queue.QueueUpload, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, queue.JobTypeUpload, next.Type)
}

// ---------------------------------------------------------------------------
// UploadWorker
// ---------------------------------------------------------------------------

func TestNewUploadWorker(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	w := NewUploadWorker(db, qCli, "http://ai:8000", 3)
	assert.NotNil(t, w)
}

func TestUploadWorker_ProcessJob_Success_MarksVideoCompleted(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-upload-1", VideoStatusProcessing)

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/process/video", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAI.Close()

	w := NewUploadWorker(db, qCli, mockAI.URL, 3)
	job := &queue.Job{
		ID: "job-upload-1", Type: queue.JobTypeUpload,
		VideoID: "vid-upload-1", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	var vid Video
	db.First(&vid, "id = ?", "vid-upload-1")
	assert.Equal(t, VideoStatusCompleted, vid.Status)

	status, err := qCli.GetStatus(ctx, "job-upload-1")
	require.NoError(t, err)
	assert.Equal(t, string(queue.JobStatusDone), status)
}

func TestUploadWorker_ProcessJob_AIFailure_MarksVideoFailed(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-upload-fail", VideoStatusProcessing)

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockAI.Close()

	w := NewUploadWorker(db, qCli, mockAI.URL, 3)
	// RetryCount already at max → goes straight to DLQ.
	job := &queue.Job{
		ID: "job-upload-fail", Type: queue.JobTypeUpload,
		VideoID: "vid-upload-fail", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3, RetryCount: 2,
	}

	ctx := context.Background()
	w.processJob(ctx, job)

	var vid Video
	db.First(&vid, "id = ?", "vid-upload-fail")
	assert.Equal(t, VideoStatusFailed, vid.Status)
}

// ---------------------------------------------------------------------------
// QueueAnalyticsWorker
// ---------------------------------------------------------------------------

func TestNewQueueAnalyticsWorker(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	w := NewQueueAnalyticsWorker(db, qCli, 3)
	assert.NotNil(t, w)
}

func TestQueueAnalyticsWorker_ProcessJob_DoesNotPanic(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)

	w := NewQueueAnalyticsWorker(db, qCli, 3)
	job := &queue.Job{
		ID: "job-analytics-1", Type: queue.JobTypeAnalytics,
		VideoID: "vid-1", UserID: "user-1", MaxRetries: 3,
	}

	ctx := context.Background()
	assert.NotPanics(t, func() {
		w.processJob(ctx, job)
	})

	status, err := qCli.GetStatus(ctx, "job-analytics-1")
	require.NoError(t, err)
	assert.Equal(t, string(queue.JobStatusDone), status)
}

func TestQueueAnalyticsWorker_Start_StopsOnContextCancel(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)

	w := NewQueueAnalyticsWorker(db, qCli, 3)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("QueueAnalyticsWorker.Start did not stop after context cancel")
	}
}

// ---------------------------------------------------------------------------
// End-to-end pipeline test
// ---------------------------------------------------------------------------

// TestFullPipeline_TranscriptToUpload tests that a job flows all the way
// through the transcript → clip → subtitle → upload pipeline.
func TestFullPipeline_TranscriptToUpload(t *testing.T) {
	db := setupQueueWorkerDB(t)
	qCli, _ := newTestQueueClient(t)
	seedVideoForQueue(db, "vid-e2e", VideoStatusPending)

	// Mock AI service: /process/clips returns a single clip; all other endpoints return OK.
	clipsPayload := map[string]interface{}{
		"video_id": "vid-e2e",
		"clips": []map[string]interface{}{
			{
				"index":           0,
				"storage_path":    "/clips/vid-e2e/clip_000.mp4",
				"start_time":      0.0,
				"end_time":        30.0,
				"duration":        30.0,
				"viral_score":     0.75,
				"rationale":       "E2E clip",
				"hook_text":       "Watch this",
				"suggested_title": "E2E Test Clip",
				"hashtags":        []string{},
				"suggested_for":   []string{"tiktok"},
			},
		},
		"manifest_path":   "/clips/vid-e2e_manifest.json",
		"processing_time": 1.0,
	}

	mockAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process/clips" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(clipsPayload)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAI.Close()

	ctx := context.Background()

	tw := NewTranscriptWorker(db, qCli, mockAI.URL, 3)
	cw := NewClipWorker(db, qCli, mockAI.URL, 3)
	sw := NewSubtitleWorker(db, qCli, mockAI.URL, 3)
	uw := NewUploadWorker(db, qCli, mockAI.URL, 3)

	job := &queue.Job{
		ID: "vid-e2e", Type: queue.JobTypeTranscript,
		VideoID: "vid-e2e", UserID: "user-1",
		StoragePath: "/storage/v.mp4", MaxRetries: 3,
	}

	tw.processJob(ctx, job)

	clipJob, err := qCli.BlockingPop(ctx, queue.QueueClip, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, clipJob)
	cw.processJob(ctx, clipJob)

	// After ClipWorker: one Clip record in DB.
	var clips []Clip
	db.Where("video_id = ?", "vid-e2e").Find(&clips)
	assert.Len(t, clips, 1, "clip record should be created after ClipWorker")

	subJob, err := qCli.BlockingPop(ctx, queue.QueueSubtitle, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, subJob)
	sw.processJob(ctx, subJob)

	uploadJob, err := qCli.BlockingPop(ctx, queue.QueueUpload, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, uploadJob)
	uw.processJob(ctx, uploadJob)

	var vid Video
	db.First(&vid, "id = ?", "vid-e2e")
	assert.Equal(t, VideoStatusCompleted, vid.Status)
}
