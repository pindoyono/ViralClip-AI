package workers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupRecoveryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&FailedJobRecord{}))
	return db
}

func seedFailedJob(t *testing.T, db *gorm.DB, jobID, queueName, status string, retryCount, maxRetries int, lastRetryAt *time.Time) FailedJobRecord {
	t.Helper()
	job := queue.Job{
		ID:         jobID,
		Type:       queue.JobTypeTranscript,
		VideoID:    "vid-" + jobID,
		RetryCount: retryCount,
		MaxRetries: maxRetries,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	payload, err := json.Marshal(job)
	require.NoError(t, err)

	now := time.Now().UTC()
	rec := FailedJobRecord{
		ID:          newUUID(),
		JobID:       jobID,
		QueueName:   queueName,
		Payload:     string(payload),
		RetryCount:  retryCount,
		MaxRetries:  maxRetries,
		Status:      status,
		LastRetryAt: lastRetryAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, db.Create(&rec).Error)
	return rec
}

// ---------------------------------------------------------------------------
// computeBackoff
// ---------------------------------------------------------------------------

func TestComputeBackoff(t *testing.T) {
	base := 30 * time.Second
	max := time.Hour

	assert.Equal(t, 30*time.Second, computeBackoff(0, base, max))
	assert.Equal(t, 60*time.Second, computeBackoff(1, base, max))
	assert.Equal(t, 120*time.Second, computeBackoff(2, base, max))
	assert.Equal(t, 240*time.Second, computeBackoff(3, base, max))

	// Ensure cap is respected.
	assert.Equal(t, max, computeBackoff(100, base, max))
}

// ---------------------------------------------------------------------------
// RecoveryWorker
// ---------------------------------------------------------------------------

func TestNewRecoveryWorker(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t) // reuse helper from queue_workers_test.go
	w := NewRecoveryWorker(db, cli, 3)
	assert.NotNil(t, w)
}

func TestRecoveryWorker_RequeuesEligibleJob(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	seedFailedJob(t, db, "job-requeue", queue.QueueTranscript, "pending", 0, 3, nil)

	w := NewRecoveryWorker(db, cli, 5)
	w.processRecoveryBatch(context.Background())

	// Job should have been pushed back to transcript_queue in Redis.
	n, err := cli.QueueLength(context.Background(), queue.QueueTranscript)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// DB record should be updated to recovering.
	var rec FailedJobRecord
	require.NoError(t, db.Where("job_id = ?", "job-requeue").First(&rec).Error)
	assert.Equal(t, "recovering", rec.Status)
	assert.Equal(t, 1, rec.RetryCount)
	assert.NotNil(t, rec.LastRetryAt)
}

func TestRecoveryWorker_ExhaustsJobAtMaxRetries(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	seedFailedJob(t, db, "job-exhaust", queue.QueueClip, "recovering", 3, 3, nil)

	w := NewRecoveryWorker(db, cli, 3)
	w.processRecoveryBatch(context.Background())

	// Nothing should be pushed to the queue.
	n, err := cli.QueueLength(context.Background(), queue.QueueClip)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var rec FailedJobRecord
	require.NoError(t, db.Where("job_id = ?", "job-exhaust").First(&rec).Error)
	assert.Equal(t, "exhausted", rec.Status)
}

func TestRecoveryWorker_RespectsBackoffDelay(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	// last_retry_at is 5 seconds ago — back-off for retry 1 is 60s, so not yet due.
	recent := time.Now().UTC().Add(-5 * time.Second)
	seedFailedJob(t, db, "job-backoff", queue.QueueUpload, "recovering", 1, 5, &recent)

	w := NewRecoveryWorker(db, cli, 5)
	w.processRecoveryBatch(context.Background())

	// Should not be re-queued yet.
	n, err := cli.QueueLength(context.Background(), queue.QueueUpload)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var rec FailedJobRecord
	require.NoError(t, db.Where("job_id = ?", "job-backoff").First(&rec).Error)
	assert.Equal(t, "recovering", rec.Status) // unchanged
}

func TestRecoveryWorker_RequeuedAfterBackoffExpiry(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	// last_retry_at far enough in the past that back-off has elapsed.
	old := time.Now().UTC().Add(-2 * time.Hour)
	seedFailedJob(t, db, "job-due", queue.QueueAnalytics, "recovering", 1, 5, &old)

	w := NewRecoveryWorker(db, cli, 5)
	w.processRecoveryBatch(context.Background())

	n, err := cli.QueueLength(context.Background(), queue.QueueAnalytics)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var rec FailedJobRecord
	require.NoError(t, db.Where("job_id = ?", "job-due").First(&rec).Error)
	assert.Equal(t, "recovering", rec.Status)
	assert.Equal(t, 2, rec.RetryCount)
}

func TestRecoveryWorker_MaxRetriesOverride(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	// Job's own MaxRetries is 10, but worker is configured with maxRetries=1.
	seedFailedJob(t, db, "job-override", queue.QueueSubtitle, "pending", 1, 10, nil)

	w := NewRecoveryWorker(db, cli, 1)
	w.processRecoveryBatch(context.Background())

	// Worker-level cap should exhaust the job.
	n, err := cli.QueueLength(context.Background(), queue.QueueSubtitle)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	var rec FailedJobRecord
	require.NoError(t, db.Where("job_id = ?", "job-override").First(&rec).Error)
	assert.Equal(t, "exhausted", rec.Status)
}

func TestRecoveryWorker_Start_StopsOnCancel(t *testing.T) {
	db := setupRecoveryDB(t)
	cli, _ := newTestQueueClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	w := NewRecoveryWorker(db, cli, 3)

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RecoveryWorker.Start did not exit after context cancellation")
	}
}
