package workers

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// deadLetterQueues lists the original queue names whose dead-letter variants
// this worker monitors. The DLQ key is "{name}:dead" (see queue.deadLetterKey).
var deadLetterQueues = []string{
	queue.QueueTranscript,
	queue.QueueClip,
	queue.QueueSubtitle,
	queue.QueueUpload,
	queue.QueueAnalytics,
}

// DeadLetterWorker reads from all dead-letter queues, persists failed job
// metadata to the database, and signals them as ready for recovery.
//
// It does NOT retry jobs itself — that is the responsibility of RecoveryWorker.
type DeadLetterWorker struct {
	db       *gorm.DB
	queueCli *queue.QueueClient
}

// NewDeadLetterWorker creates a DeadLetterWorker.
func NewDeadLetterWorker(db *gorm.DB, qCli *queue.QueueClient) *DeadLetterWorker {
	return &DeadLetterWorker{db: db, queueCli: qCli}
}

// Start launches one consumer goroutine per monitored DLQ and blocks until
// ctx is cancelled.
func (w *DeadLetterWorker) Start(ctx context.Context) {
	log.Info().Msg("DeadLetterWorker: started")
	var wg sync.WaitGroup
	for _, q := range deadLetterQueues {
		wg.Add(1)
		go func(origQueue string) {
			defer wg.Done()
			w.consumeDLQ(ctx, origQueue)
		}(q)
	}
	wg.Wait()
	log.Info().Msg("DeadLetterWorker: stopped")
}

// consumeDLQ pops jobs from "{origQueue}:dead" and persists them.
func (w *DeadLetterWorker) consumeDLQ(ctx context.Context, origQueue string) {
	dlqName := origQueue + ":dead"
	log.Info().Str("dlq", dlqName).Msg("DeadLetterWorker: monitoring DLQ")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, dlqName, popTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error().Err(err).Str("dlq", dlqName).Msg("DeadLetterWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.persistFailedJob(ctx, origQueue, job)
	}
}

// persistFailedJob serialises the job and writes a FailedJobRecord to the DB.
func (w *DeadLetterWorker) persistFailedJob(ctx context.Context, queueName string, job *queue.Job) {
	payload, err := json.Marshal(job)
	if err != nil {
		log.Error().Err(err).
			Str("job_id", job.ID).
			Msg("DeadLetterWorker: failed to marshal job payload")
		return
	}

	errMsg := ""
	if job.Metadata != nil {
		errMsg = job.Metadata["error"]
	}

	now := time.Now().UTC()
	rec := FailedJobRecord{
		ID:           newUUID(),
		JobID:        job.ID,
		QueueName:    queueName,
		Payload:      string(payload),
		ErrorMessage: errMsg,
		RetryCount:   job.RetryCount,
		MaxRetries:   job.MaxRetries,
		Status:       "pending",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := w.db.WithContext(ctx).Create(&rec).Error; err != nil {
		log.Error().Err(err).
			Str("job_id", job.ID).
			Str("queue", queueName).
			Msg("DeadLetterWorker: failed to persist failed job to DB")
		return
	}

	log.Warn().
		Str("job_id", job.ID).
		Str("queue", queueName).
		Str("failed_job_id", rec.ID).
		Int("retry_count", job.RetryCount).
		Int("max_retries", job.MaxRetries).
		Str("error", errMsg).
		Msg("DeadLetterWorker: job persisted to failed_jobs")
}
