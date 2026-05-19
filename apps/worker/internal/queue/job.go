// Package queue implements the Redis-backed job queue used by ViralClip AI workers.
//
// Each queue uses a Redis List. Producers call Push (RPUSH) and consumers
// call BlockingPop (BLPOP) so workers sleep in Redis instead of hammering
// the database.
//
// Dead-letter queues use explicit Redis keys such as "transcript_dlq" and
// "clip_dlq". Jobs that exceed their MaxRetries limit are moved there
// automatically by the consumer helpers.
//
// Job status is tracked in a Redis hash key "job:{id}" with a configurable
// TTL so callers can inspect a job's current state without querying the DB.
package queue

import (
	"fmt"
	"time"
)

// Queue name constants shared between the API publisher and the worker consumers.
const (
	QueueTranscript = "transcript_queue"
	QueueClip       = "clip_queue"
	QueueSubtitle   = "subtitle_queue"
	QueueUpload     = "upload_queue"
	QueueAnalytics  = "analytics_queue"

	QueueTranscriptDLQ = "transcript_dlq"
	QueueClipDLQ       = "clip_dlq"
	QueueSubtitleDLQ   = "subtitle_dlq"
	QueueUploadDLQ     = "upload_dlq"
	QueueAnalyticsDLQ  = "analytics_dlq"
)

var deadLetterQueueNames = map[string]string{
	QueueTranscript: QueueTranscriptDLQ,
	QueueClip:       QueueClipDLQ,
	QueueSubtitle:   QueueSubtitleDLQ,
	QueueUpload:     QueueUploadDLQ,
	QueueAnalytics:  QueueAnalyticsDLQ,
}

// JobStatus represents the lifecycle state of a queued job.
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusDone       JobStatus = "done"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDead       JobStatus = "dead"
)

// JobType identifies the kind of work a Job requests.
type JobType string

const (
	JobTypeTranscript JobType = "transcript"
	JobTypeClip       JobType = "clip"
	JobTypeSubtitle   JobType = "subtitle"
	JobTypeUpload     JobType = "upload"
	JobTypeAnalytics  JobType = "analytics"
)

// Job is the unit of work pushed onto a queue.
type Job struct {
	// ID uniquely identifies the job. When empty, callers should set it before
	// publishing (typically to the video ID so status look-ups are simple).
	ID string `json:"id"`

	// Type describes the work to perform.
	Type JobType `json:"type"`

	// VideoID is the source video this job belongs to.
	VideoID string `json:"video_id"`

	// UserID is the owner of the video.
	UserID string `json:"user_id"`

	// StoragePath is the key/path of the file in the storage backend.
	StoragePath string `json:"storage_path"`

	// StorageURL is the publicly accessible URL of the file (may be empty).
	StorageURL string `json:"storage_url"`

	// RetryCount is the number of times this job has been retried.
	RetryCount int `json:"retry_count"`

	// MaxRetries is the maximum number of attempts before the job is sent to
	// the dead-letter queue.
	MaxRetries int `json:"max_retries"`

	// CreatedAt is set once when the job is first enqueued.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is updated on each retry attempt.
	UpdatedAt time.Time `json:"updated_at"`

	// Metadata holds arbitrary key/value pairs for job-specific data that
	// does not fit the standard fields.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeadLetterQueueName returns the explicit dead-letter queue name for a given
// main queue.
func DeadLetterQueueName(queueName string) (string, error) {
	if dlqName, ok := deadLetterQueueNames[queueName]; ok {
		return dlqName, nil
	}

	return "", fmt.Errorf("unknown queue %q", queueName)
}

// statusKey returns the Redis key used to track a job's status.
func statusKey(jobID string) string {
	return "job:" + jobID
}
