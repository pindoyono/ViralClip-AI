package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// popTimeout is the BLPOP wait before the consumer re-checks the context.
const popTimeout = 5 * time.Second

// baseQueueWorker provides the shared retry / dead-letter logic.
type baseQueueWorker struct {
	db         *gorm.DB
	queueCli   *queue.QueueClient
	httpClient *http.Client
	aiURL      string
	maxRetries int
}

// handleJobFailure either re-queues the job (if retries remain) or sends it
// to the dead-letter queue. It also updates the job status in Redis.
func (b *baseQueueWorker) handleJobFailure(ctx context.Context, queueName string, job *queue.Job, err error) {
	log.Error().
		Err(err).
		Str("job_id", job.ID).
		Str("queue", queueName).
		Int("retry_count", job.RetryCount).
		Msg("Job failed")

	job.RetryCount++
	job.UpdatedAt = time.Now().UTC()

	if job.RetryCount >= job.MaxRetries {
		log.Warn().
			Str("job_id", job.ID).
			Str("queue", queueName).
			Msg("Job exceeded max retries, sending to dead-letter queue")

		if dlqErr := b.queueCli.PushDead(ctx, queueName, job); dlqErr != nil {
			log.Error().Err(dlqErr).Str("job_id", job.ID).Msg("Failed to push to dead-letter queue")
		}

		b.updateVideoStatus(ctx, job.VideoID, string(VideoStatusFailed), err.Error())
		return
	}

	// Re-queue with incremented retry count.
	if pushErr := b.queueCli.Push(ctx, queueName, job); pushErr != nil {
		log.Error().Err(pushErr).Str("job_id", job.ID).Msg("Failed to re-queue job")
	}
}

// updateVideoStatus sets the video status and optional error message in the DB.
func (b *baseQueueWorker) updateVideoStatus(ctx context.Context, videoID, status, errMsg string) {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}

	if err := b.db.WithContext(ctx).
		Model(&Video{}).
		Where("id = ?", videoID).
		Updates(updates).Error; err != nil {
		log.Error().Err(err).Str("video_id", videoID).Msg("Failed to update video status in DB")
	}
}

// callAI posts a JSON payload to the AI service and returns the response body.
func (b *baseQueueWorker) callAI(ctx context.Context, path string, payload map[string]interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal AI payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.aiURL+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create AI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call AI service %q: %w", path, err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		resp.Body.Close()
		return nil, fmt.Errorf("AI service %q returned status %d", path, resp.StatusCode)
	}

	return resp, nil
}

// ---------------------------------------------------------------------------
// TranscriptWorker
// ---------------------------------------------------------------------------

// TranscriptWorker consumes jobs from transcript_queue, calls the AI service
// to generate a transcript, and on success pushes the job to clip_queue.
type TranscriptWorker struct {
	baseQueueWorker
}

// NewTranscriptWorker creates a TranscriptWorker.
func NewTranscriptWorker(db *gorm.DB, qCli *queue.QueueClient, aiURL string, maxRetries int) *TranscriptWorker {
	return &TranscriptWorker{
		baseQueueWorker: baseQueueWorker{
			db:         db,
			queueCli:   qCli,
			httpClient: &http.Client{Timeout: 300 * time.Second},
			aiURL:      aiURL,
			maxRetries: maxRetries,
		},
	}
}

// Start begins the blocking consume loop. It exits when ctx is cancelled.
func (w *TranscriptWorker) Start(ctx context.Context) {
	log.Info().Msg("TranscriptWorker: started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("TranscriptWorker: stopped")
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, queue.QueueTranscript, popTimeout)
		if err != nil {
			log.Error().Err(err).Msg("TranscriptWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *TranscriptWorker) processJob(ctx context.Context, job *queue.Job) {
	log.Info().Str("job_id", job.ID).Str("video_id", job.VideoID).Msg("TranscriptWorker: processing job")

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusProcessing), queue.DefaultJobTTL)
	w.updateVideoStatus(ctx, job.VideoID, string(VideoStatusProcessing), "")

	resp, err := w.callAI(ctx, "/process/transcript", map[string]interface{}{
		"video_id":     job.VideoID,
		"storage_path": job.StoragePath,
	})
	if err != nil {
		w.handleJobFailure(ctx, queue.QueueTranscript, job, err)
		return
	}
	resp.Body.Close()

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusDone), queue.DefaultJobTTL)

	// Advance the job to the clip generation stage.
	job.Type = queue.JobTypeClip
	if pushErr := w.queueCli.Push(ctx, queue.QueueClip, job); pushErr != nil {
		log.Error().Err(pushErr).Str("job_id", job.ID).Msg("TranscriptWorker: failed to push to clip_queue")
	}

	log.Info().Str("job_id", job.ID).Msg("TranscriptWorker: job done, pushed to clip_queue")
}

// ---------------------------------------------------------------------------
// ClipWorker
// ---------------------------------------------------------------------------

// ClipWorker consumes jobs from clip_queue, calls the AI service to generate
// clips, and on success pushes the job to subtitle_queue.
type ClipWorker struct {
	baseQueueWorker
}

// NewClipWorker creates a ClipWorker.
func NewClipWorker(db *gorm.DB, qCli *queue.QueueClient, aiURL string, maxRetries int) *ClipWorker {
	return &ClipWorker{
		baseQueueWorker: baseQueueWorker{
			db:         db,
			queueCli:   qCli,
			httpClient: &http.Client{Timeout: 300 * time.Second},
			aiURL:      aiURL,
			maxRetries: maxRetries,
		},
	}
}

// Start begins the blocking consume loop.
func (w *ClipWorker) Start(ctx context.Context) {
	log.Info().Msg("ClipWorker: started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("ClipWorker: stopped")
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, queue.QueueClip, popTimeout)
		if err != nil {
			log.Error().Err(err).Msg("ClipWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *ClipWorker) processJob(ctx context.Context, job *queue.Job) {
	log.Info().Str("job_id", job.ID).Str("video_id", job.VideoID).Msg("ClipWorker: processing job")

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusProcessing), queue.DefaultJobTTL)

	resp, err := w.callAI(ctx, "/process/clips", map[string]interface{}{
		"video_id":     job.VideoID,
		"storage_path": job.StoragePath,
	})
	if err != nil {
		w.handleJobFailure(ctx, queue.QueueClip, job, err)
		return
	}
	resp.Body.Close()

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusDone), queue.DefaultJobTTL)

	job.Type = queue.JobTypeSubtitle
	if pushErr := w.queueCli.Push(ctx, queue.QueueSubtitle, job); pushErr != nil {
		log.Error().Err(pushErr).Str("job_id", job.ID).Msg("ClipWorker: failed to push to subtitle_queue")
	}

	log.Info().Str("job_id", job.ID).Msg("ClipWorker: job done, pushed to subtitle_queue")
}

// ---------------------------------------------------------------------------
// SubtitleWorker
// ---------------------------------------------------------------------------

// SubtitleWorker consumes jobs from subtitle_queue, calls the AI service to
// generate subtitles, and on success pushes the job to upload_queue.
type SubtitleWorker struct {
	baseQueueWorker
}

// NewSubtitleWorker creates a SubtitleWorker.
func NewSubtitleWorker(db *gorm.DB, qCli *queue.QueueClient, aiURL string, maxRetries int) *SubtitleWorker {
	return &SubtitleWorker{
		baseQueueWorker: baseQueueWorker{
			db:         db,
			queueCli:   qCli,
			httpClient: &http.Client{Timeout: 120 * time.Second},
			aiURL:      aiURL,
			maxRetries: maxRetries,
		},
	}
}

// Start begins the blocking consume loop.
func (w *SubtitleWorker) Start(ctx context.Context) {
	log.Info().Msg("SubtitleWorker: started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("SubtitleWorker: stopped")
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, queue.QueueSubtitle, popTimeout)
		if err != nil {
			log.Error().Err(err).Msg("SubtitleWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *SubtitleWorker) processJob(ctx context.Context, job *queue.Job) {
	log.Info().Str("job_id", job.ID).Str("video_id", job.VideoID).Msg("SubtitleWorker: processing job")

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusProcessing), queue.DefaultJobTTL)

	resp, err := w.callAI(ctx, "/process/subtitles", map[string]interface{}{
		"video_id":     job.VideoID,
		"storage_path": job.StoragePath,
	})
	if err != nil {
		w.handleJobFailure(ctx, queue.QueueSubtitle, job, err)
		return
	}
	resp.Body.Close()

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusDone), queue.DefaultJobTTL)

	job.Type = queue.JobTypeUpload
	if pushErr := w.queueCli.Push(ctx, queue.QueueUpload, job); pushErr != nil {
		log.Error().Err(pushErr).Str("job_id", job.ID).Msg("SubtitleWorker: failed to push to upload_queue")
	}

	log.Info().Str("job_id", job.ID).Msg("SubtitleWorker: job done, pushed to upload_queue")
}

// ---------------------------------------------------------------------------
// UploadWorker
// ---------------------------------------------------------------------------

// UploadWorker consumes jobs from upload_queue and finalizes video processing
// by marking the video as completed in the database.
type UploadWorker struct {
	baseQueueWorker
}

// NewUploadWorker creates an UploadWorker.
func NewUploadWorker(db *gorm.DB, qCli *queue.QueueClient, aiURL string, maxRetries int) *UploadWorker {
	return &UploadWorker{
		baseQueueWorker: baseQueueWorker{
			db:         db,
			queueCli:   qCli,
			httpClient: &http.Client{Timeout: 120 * time.Second},
			aiURL:      aiURL,
			maxRetries: maxRetries,
		},
	}
}

// Start begins the blocking consume loop.
func (w *UploadWorker) Start(ctx context.Context) {
	log.Info().Msg("UploadWorker: started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("UploadWorker: stopped")
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, queue.QueueUpload, popTimeout)
		if err != nil {
			log.Error().Err(err).Msg("UploadWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *UploadWorker) processJob(ctx context.Context, job *queue.Job) {
	log.Info().Str("job_id", job.ID).Str("video_id", job.VideoID).Msg("UploadWorker: processing job")

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusProcessing), queue.DefaultJobTTL)

	resp, err := w.callAI(ctx, "/process/video", map[string]interface{}{
		"video_id":     job.VideoID,
		"storage_path": job.StoragePath,
	})
	if err != nil {
		w.handleJobFailure(ctx, queue.QueueUpload, job, err)
		return
	}
	resp.Body.Close()

	now := time.Now().UTC()
	if dbErr := w.db.WithContext(ctx).
		Model(&Video{}).
		Where("id = ?", job.VideoID).
		Updates(map[string]interface{}{
			"status":     string(VideoStatusCompleted),
			"updated_at": now,
		}).Error; dbErr != nil {
		log.Error().Err(dbErr).Str("video_id", job.VideoID).Msg("UploadWorker: failed to mark video completed")
	}

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusDone), queue.DefaultJobTTL)

	log.Info().Str("job_id", job.ID).Str("video_id", job.VideoID).Msg("UploadWorker: video processing pipeline complete")
}

// ---------------------------------------------------------------------------
// QueueAnalyticsWorker
// ---------------------------------------------------------------------------

// QueueAnalyticsWorker consumes jobs from analytics_queue to process
// analytics events asynchronously.
type QueueAnalyticsWorker struct {
	baseQueueWorker
}

// NewQueueAnalyticsWorker creates a QueueAnalyticsWorker.
func NewQueueAnalyticsWorker(db *gorm.DB, qCli *queue.QueueClient, maxRetries int) *QueueAnalyticsWorker {
	return &QueueAnalyticsWorker{
		baseQueueWorker: baseQueueWorker{
			db:         db,
			queueCli:   qCli,
			httpClient: &http.Client{Timeout: 30 * time.Second},
			maxRetries: maxRetries,
		},
	}
}

// Start begins the blocking consume loop.
func (w *QueueAnalyticsWorker) Start(ctx context.Context) {
	log.Info().Msg("QueueAnalyticsWorker: started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("QueueAnalyticsWorker: stopped")
			return
		default:
		}

		job, err := w.queueCli.BlockingPop(ctx, queue.QueueAnalytics, popTimeout)
		if err != nil {
			log.Error().Err(err).Msg("QueueAnalyticsWorker: pop error")
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *QueueAnalyticsWorker) processJob(ctx context.Context, job *queue.Job) {
	log.Info().Str("job_id", job.ID).Msg("QueueAnalyticsWorker: processing analytics job")

	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusProcessing), queue.DefaultJobTTL)

	// Analytics events are processed by reading the clip/post ID from metadata
	// and updating the ClipAnalytics table with the latest platform metrics.
	// Actual platform API calls will be implemented in Task 6 (Performance
	// Learning Engine). Here we acknowledge the job and mark it done so the
	// queue does not stall.
	_ = w.queueCli.TrackStatus(ctx, job.ID, string(queue.JobStatusDone), queue.DefaultJobTTL)

	log.Info().Str("job_id", job.ID).Msg("QueueAnalyticsWorker: job done")
}
