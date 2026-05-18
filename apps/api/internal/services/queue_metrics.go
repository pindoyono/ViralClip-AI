package services

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// Known queue names mirrored from the worker package (kept in sync manually
// to avoid a cross-module import).
const (
	metricsQueueTranscript = "transcript_queue"
	metricsQueueClip       = "clip_queue"
	metricsQueueSubtitle   = "subtitle_queue"
	metricsQueueUpload     = "upload_queue"
	metricsQueueAnalytics  = "analytics_queue"
)

var metricsQueueNames = []string{
	metricsQueueTranscript,
	metricsQueueClip,
	metricsQueueSubtitle,
	metricsQueueUpload,
	metricsQueueAnalytics,
}

// QueueSizeMetric holds the pending and dead-letter counts for one queue.
type QueueSizeMetric struct {
	Name        string `json:"name"`
	PendingJobs int64  `json:"pending_jobs"`
	DeadJobs    int64  `json:"dead_jobs"`
}

// FailedJobStats aggregates database-side failed job counters.
type FailedJobStats struct {
	TotalFailed    int64 `json:"total_failed"`
	TotalRecovering int64 `json:"total_recovering"`
	TotalExhausted int64 `json:"total_exhausted"`
}

// QueueStatusReport is the payload returned by GET /queue/status.
type QueueStatusReport struct {
	CollectedAt time.Time         `json:"collected_at"`
	Queues      []QueueSizeMetric `json:"queues"`
	FailedJobs  FailedJobStats    `json:"failed_jobs"`
}

// QueueMetricsService collects queue and failed-job metrics from Redis and
// the database.
type QueueMetricsService struct {
	db  *gorm.DB
	rdb *redis.Client
}

// NewQueueMetricsService creates a QueueMetricsService.
func NewQueueMetricsService(db *gorm.DB, rdb *redis.Client) *QueueMetricsService {
	return &QueueMetricsService{db: db, rdb: rdb}
}

// Status returns a point-in-time snapshot of queue depths and failed job
// counts.
func (s *QueueMetricsService) Status(ctx context.Context) (*QueueStatusReport, error) {
	queueMetrics, err := s.collectQueueSizes(ctx)
	if err != nil {
		return nil, err
	}

	failedStats, err := s.collectFailedJobStats(ctx)
	if err != nil {
		return nil, err
	}

	return &QueueStatusReport{
		CollectedAt: time.Now().UTC(),
		Queues:      queueMetrics,
		FailedJobs:  *failedStats,
	}, nil
}

// collectQueueSizes fetches LLEN for each main queue and its DLQ from Redis.
func (s *QueueMetricsService) collectQueueSizes(ctx context.Context) ([]QueueSizeMetric, error) {
	result := make([]QueueSizeMetric, 0, len(metricsQueueNames))

	for _, name := range metricsQueueNames {
		pending, err := s.rdb.LLen(ctx, name).Result()
		if err != nil {
			log.Error().Err(err).Str("queue", name).Msg("QueueMetricsService: LLEN failed")
			return nil, err
		}

		dlqName := name + ":dead"
		dead, err := s.rdb.LLen(ctx, dlqName).Result()
		if err != nil {
			log.Error().Err(err).Str("dlq", dlqName).Msg("QueueMetricsService: LLEN DLQ failed")
			return nil, err
		}

		result = append(result, QueueSizeMetric{
			Name:        name,
			PendingJobs: pending,
			DeadJobs:    dead,
		})
	}

	return result, nil
}

// collectFailedJobStats counts FailedJob rows by status from the database.
func (s *QueueMetricsService) collectFailedJobStats(ctx context.Context) (*FailedJobStats, error) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row

	err := s.db.WithContext(ctx).
		Model(&models.FailedJob{}).
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&rows).Error
	if err != nil {
		log.Error().Err(err).Msg("QueueMetricsService: failed to count failed_jobs by status")
		return nil, err
	}

	stats := &FailedJobStats{}
	for _, r := range rows {
		switch r.Status {
		case string(models.FailedJobStatusPending):
			stats.TotalFailed = r.Count
		case string(models.FailedJobStatusRecovering):
			stats.TotalRecovering = r.Count
		case string(models.FailedJobStatusExhausted):
			stats.TotalExhausted = r.Count
		}
	}

	return stats, nil
}

// ListFailedJobs returns paginated FailedJob records filtered by queue name
// and/or status.
func (s *QueueMetricsService) ListFailedJobs(
	ctx context.Context,
	queueName, status string,
	limit, offset int,
) ([]models.FailedJob, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.FailedJob{})

	if queueName != "" {
		query = query.Where("queue_name = ?", queueName)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var jobs []models.FailedJob
	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobs).Error; err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

// ListRetryableJobs returns FailedJob records that are eligible for recovery
// (status pending or recovering, retry_count < max_retries).
func (s *QueueMetricsService) ListRetryableJobs(ctx context.Context, limit, offset int) ([]models.FailedJob, int64, error) {
	query := s.db.WithContext(ctx).
		Model(&models.FailedJob{}).
		Where("status IN ? AND retry_count < max_retries",
			[]string{
				string(models.FailedJobStatusPending),
				string(models.FailedJobStatusRecovering),
			})

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var jobs []models.FailedJob
	if err := query.
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&jobs).Error; err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}
