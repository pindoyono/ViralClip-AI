package services

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupQueueMetricsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.FailedJob{}))
	return db
}

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, mr
}

func seedFailedJobAPI(t *testing.T, db *gorm.DB, queueName, status string) models.FailedJob {
	t.Helper()
	j := models.FailedJob{
		ID:           uuid.New(),
		JobID:        uuid.New().String(),
		QueueName:    queueName,
		Payload:      `{"id":"x"}`,
		ErrorMessage: "some error",
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
// QueueMetricsService tests
// ---------------------------------------------------------------------------

func TestNewQueueMetricsService(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)
	assert.NotNil(t, svc)
}

func TestQueueMetricsService_Status_EmptyQueues(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	report, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.Len(t, report.Queues, len(metricsQueueNames))
	assert.Equal(t, int64(0), report.FailedJobs.TotalFailed)

	for _, q := range report.Queues {
		assert.Equal(t, int64(0), q.PendingJobs)
		assert.Equal(t, int64(0), q.DeadJobs)
	}
}

func TestQueueMetricsService_Status_WithJobs(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, mr := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	// Push some items to queues in miniredis.
	mr.Push(metricsQueueTranscript, "a", "b")
	mr.Push(metricsQueueTranscript+":dead", "c")

	// Seed DB records.
	seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	seedFailedJobAPI(t, db, metricsQueueClip, "recovering")
	seedFailedJobAPI(t, db, metricsQueueClip, "exhausted")

	report, err := svc.Status(context.Background())
	require.NoError(t, err)

	// Check queue sizes.
	var transcriptMetric *QueueSizeMetric
	for i := range report.Queues {
		if report.Queues[i].Name == metricsQueueTranscript {
			transcriptMetric = &report.Queues[i]
			break
		}
	}
	require.NotNil(t, transcriptMetric)
	assert.Equal(t, int64(2), transcriptMetric.PendingJobs)
	assert.Equal(t, int64(1), transcriptMetric.DeadJobs)

	// Check failed job stats.
	assert.Equal(t, int64(1), report.FailedJobs.TotalFailed)
	assert.Equal(t, int64(1), report.FailedJobs.TotalRecovering)
	assert.Equal(t, int64(1), report.FailedJobs.TotalExhausted)
}

func TestQueueMetricsService_ListFailedJobs_FilterByQueue(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	seedFailedJobAPI(t, db, metricsQueueClip, "pending")

	jobs, total, err := svc.ListFailedJobs(context.Background(), metricsQueueTranscript, "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, jobs, 2)
}

func TestQueueMetricsService_ListFailedJobs_FilterByStatus(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	seedFailedJobAPI(t, db, metricsQueueTranscript, "exhausted")
	seedFailedJobAPI(t, db, metricsQueueClip, "recovering")

	jobs, total, err := svc.ListFailedJobs(context.Background(), "", "exhausted", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, jobs, 1)
	assert.Equal(t, models.FailedJobStatusExhausted, jobs[0].Status)
}

func TestQueueMetricsService_ListFailedJobs_Pagination(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	for i := 0; i < 5; i++ {
		seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	}

	jobs, total, err := svc.ListFailedJobs(context.Background(), "", "", 2, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, jobs, 2)

	jobs2, _, err := svc.ListFailedJobs(context.Background(), "", "", 2, 2)
	require.NoError(t, err)
	assert.Len(t, jobs2, 2)
}

func TestQueueMetricsService_ListRetryableJobs(t *testing.T) {
	db := setupQueueMetricsDB(t)
	rdb, _ := newTestRedis(t)
	svc := NewQueueMetricsService(db, rdb)

	// Retryable: pending with retryCount < maxRetries.
	seedFailedJobAPI(t, db, metricsQueueTranscript, "pending")
	// Retryable: recovering.
	seedFailedJobAPI(t, db, metricsQueueClip, "recovering")
	// Not retryable: exhausted.
	seedFailedJobAPI(t, db, metricsQueueUpload, "exhausted")

	jobs, total, err := svc.ListRetryableJobs(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, jobs, 2)

	for _, j := range jobs {
		assert.NotEqual(t, models.FailedJobStatusExhausted, j.Status)
	}
}
