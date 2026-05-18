package workers

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

const (
	// recoveryBaseDelay is the unit of exponential back-off between retries.
	recoveryBaseDelay = 30 * time.Second

	// recoveryMaxDelay caps the computed back-off so jobs are never starved.
	recoveryMaxDelay = time.Hour

	// recoveryPollInterval is how often RecoveryWorker scans for eligible jobs.
	recoveryPollInterval = 30 * time.Second

	// recoveryBatchSize is the maximum number of jobs processed per poll cycle.
	recoveryBatchSize = 50
)

// RecoveryWorker reads pending FailedJobRecords from the database, re-queues
// them with exponential back-off, and permanently exhausts jobs that have
// exceeded MaxRetries.
type RecoveryWorker struct {
	db         *gorm.DB
	queueCli   *queue.QueueClient
	maxRetries int
}

// NewRecoveryWorker creates a RecoveryWorker.
// maxRetries sets the absolute upper bound of recovery attempts; jobs that
// have already exhausted their original MaxRetries field are capped here too.
func NewRecoveryWorker(db *gorm.DB, qCli *queue.QueueClient, maxRetries int) *RecoveryWorker {
	return &RecoveryWorker{db: db, queueCli: qCli, maxRetries: maxRetries}
}

// Start polls the database for recoverable jobs on a fixed interval until ctx
// is cancelled.
func (w *RecoveryWorker) Start(ctx context.Context) {
	log.Info().Msg("RecoveryWorker: started")
	ticker := time.NewTicker(recoveryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("RecoveryWorker: stopped")
			return
		case <-ticker.C:
			w.processRecoveryBatch(ctx)
		}
	}
}

// processRecoveryBatch fetches jobs eligible for retry and re-queues them.
func (w *RecoveryWorker) processRecoveryBatch(ctx context.Context) {
	var jobs []FailedJobRecord

	// Use recoveryBaseDelay as the minimum back-off in the WHERE clause so we
	// avoid fetching jobs that were very recently retried. Jobs with higher
	// retry counts have longer computed back-offs and are skipped inside
	// recoverJob() after the exact delay is evaluated, keeping the SQL simple
	// while still eliminating most unnecessary reads.
	err := w.db.WithContext(ctx).
		Where("status IN ? AND (last_retry_at IS NULL OR last_retry_at <= ?)",
			[]string{"pending", "recovering"}, time.Now().UTC().Add(-recoveryBaseDelay)).
		Order("created_at ASC").
		Limit(recoveryBatchSize).
		Find(&jobs).Error
	if err != nil {
		log.Error().Err(err).Msg("RecoveryWorker: failed to query failed_jobs")
		return
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.recoverJob(ctx, job)
	}
}

// recoverJob decides whether to re-queue a failed job or exhaust it.
func (w *RecoveryWorker) recoverJob(ctx context.Context, rec FailedJobRecord) {
	maxAllowed := rec.MaxRetries
	if w.maxRetries > 0 && w.maxRetries < maxAllowed {
		maxAllowed = w.maxRetries
	}

	if rec.RetryCount >= maxAllowed {
		w.exhaustJob(ctx, rec)
		return
	}

	// Compute next allowed retry time using exponential back-off.
	delay := computeBackoff(rec.RetryCount, recoveryBaseDelay, recoveryMaxDelay)
	if rec.LastRetryAt != nil && time.Since(*rec.LastRetryAt) < delay {
		// Not yet due.
		return
	}

	w.requeue(ctx, rec)
}

// requeue parses the stored payload, pushes the job back onto its original
// queue, and updates the FailedJobRecord.
func (w *RecoveryWorker) requeue(ctx context.Context, rec FailedJobRecord) {
	var job queue.Job
	if err := json.Unmarshal([]byte(rec.Payload), &job); err != nil {
		log.Error().Err(err).
			Str("failed_job_id", rec.ID).
			Msg("RecoveryWorker: failed to unmarshal job payload; exhausting")
		w.exhaustJob(ctx, rec)
		return
	}

	// Reset RetryCount to 0 so the downstream worker processes the job as a
	// fresh attempt. The total recovery attempts are tracked separately in
	// FailedJobRecord.RetryCount, which is incremented below.
	job.RetryCount = 0
	job.UpdatedAt = time.Now().UTC()
	// Remove stale error metadata so it does not pollute subsequent failures.
	delete(job.Metadata, "error")
	delete(job.Metadata, "failed_at")

	if err := w.queueCli.Push(ctx, rec.QueueName, &job); err != nil {
		log.Error().Err(err).
			Str("failed_job_id", rec.ID).
			Str("queue", rec.QueueName).
			Msg("RecoveryWorker: failed to re-queue job")
		return
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":        "recovering",
		"retry_count":   rec.RetryCount + 1,
		"last_retry_at": now,
		"updated_at":    now,
	}
	if err := w.db.WithContext(ctx).
		Model(&FailedJobRecord{}).
		Where("id = ?", rec.ID).
		Updates(updates).Error; err != nil {
		log.Error().Err(err).
			Str("failed_job_id", rec.ID).
			Msg("RecoveryWorker: failed to update failed_job record after re-queue")
	}

	log.Info().
		Str("failed_job_id", rec.ID).
		Str("job_id", rec.JobID).
		Str("queue", rec.QueueName).
		Int("recovery_attempt", rec.RetryCount+1).
		Msg("RecoveryWorker: job re-queued")
}

// exhaustJob marks a FailedJobRecord as permanently failed.
func (w *RecoveryWorker) exhaustJob(ctx context.Context, rec FailedJobRecord) {
	now := time.Now().UTC()
	if err := w.db.WithContext(ctx).
		Model(&FailedJobRecord{}).
		Where("id = ?", rec.ID).
		Updates(map[string]interface{}{
			"status":     "exhausted",
			"updated_at": now,
		}).Error; err != nil {
		log.Error().Err(err).
			Str("failed_job_id", rec.ID).
			Msg("RecoveryWorker: failed to mark job as exhausted")
		return
	}

	log.Warn().
		Str("failed_job_id", rec.ID).
		Str("job_id", rec.JobID).
		Str("queue", rec.QueueName).
		Int("retry_count", rec.RetryCount).
		Msg("RecoveryWorker: job permanently exhausted after max retries")
}

// computeBackoff returns 2^retryCount * base, capped at maxDelay.
func computeBackoff(retryCount int, base, maxDelay time.Duration) time.Duration {
	if retryCount <= 0 {
		return base
	}
	multiplier := math.Pow(2, float64(retryCount))
	// Guard against float64 overflow: if the result would exceed maxDelay
	// (accounting for the float64 precision limit), return maxDelay directly.
	maxFloat := float64(maxDelay)
	if multiplier*float64(base) >= maxFloat {
		return maxDelay
	}
	return time.Duration(multiplier * float64(base))
}
