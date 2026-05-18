// Package queue provides a lightweight job publisher for the ViralClip AI API.
//
// The API uses Redis Lists as queues (RPUSH) to dispatch video processing jobs
// to the worker service. Queue name constants are kept in sync with the worker
// module's queue package.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue name constants – must match the worker's queue package.
const (
	QueueTranscript = "transcript_queue"
	QueueClip       = "clip_queue"
	QueueSubtitle   = "subtitle_queue"
	QueueUpload     = "upload_queue"
	QueueAnalytics  = "analytics_queue"
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
// This struct mirrors the worker's queue.Job and must remain compatible.
type Job struct {
	ID          string            `json:"id"`
	Type        JobType           `json:"type"`
	VideoID     string            `json:"video_id"`
	UserID      string            `json:"user_id"`
	StoragePath string            `json:"storage_path"`
	StorageURL  string            `json:"storage_url"`
	RetryCount  int               `json:"retry_count"`
	MaxRetries  int               `json:"max_retries"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// VideoPublisher is the interface the VideoHandler uses to enqueue transcript
// jobs. It is satisfied by *Publisher and can be mocked in tests.
type VideoPublisher interface {
	PublishTranscriptJob(ctx context.Context, jobID, videoID, userID, storagePath, storageURL string) error
}

// Publisher enqueues jobs onto Redis-backed queues.
type Publisher struct {
	rdb        *redis.Client
	maxRetries int
}

// NewPublisher creates a Publisher. maxRetries sets the MaxRetries field on
// every job it enqueues.
func NewPublisher(rdb *redis.Client, maxRetries int) *Publisher {
	return &Publisher{rdb: rdb, maxRetries: maxRetries}
}

// PublishTranscriptJob enqueues a transcript job for the given video.
// jobID should be the video ID so callers can poll job status by video ID.
func (p *Publisher) PublishTranscriptJob(ctx context.Context, jobID, videoID, userID, storagePath, storageURL string) error {
	now := time.Now().UTC()
	job := &Job{
		ID:          jobID,
		Type:        JobTypeTranscript,
		VideoID:     videoID,
		UserID:      userID,
		StoragePath: storagePath,
		StorageURL:  storageURL,
		MaxRetries:  p.maxRetries,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return p.push(ctx, QueueTranscript, job)
}

// PublishAnalyticsJob enqueues an analytics sync job.
func (p *Publisher) PublishAnalyticsJob(ctx context.Context, jobID, videoID, userID string, metadata map[string]string) error {
	now := time.Now().UTC()
	job := &Job{
		ID:         jobID,
		Type:       JobTypeAnalytics,
		VideoID:    videoID,
		UserID:     userID,
		MaxRetries: p.maxRetries,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   metadata,
	}

	return p.push(ctx, QueueAnalytics, job)
}

// push serialises job as JSON and appends it to the Redis list queueName.
func (p *Publisher) push(ctx context.Context, queueName string, job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue publisher: marshal job: %w", err)
	}

	if err := p.rdb.RPush(ctx, queueName, data).Err(); err != nil {
		return fmt.Errorf("queue publisher: push to %q: %w", queueName, err)
	}

	return nil
}
