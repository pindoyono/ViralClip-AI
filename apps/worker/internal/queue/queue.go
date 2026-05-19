package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultJobTTL is how long job status entries are kept in Redis.
const DefaultJobTTL = 24 * time.Hour

// QueueClient provides push/pop operations on Redis-backed job queues.
type QueueClient struct {
	rdb    *redis.Client
	jobTTL time.Duration
}

// NewQueueClient creates a QueueClient using the provided Redis client.
// jobTTL controls how long job status records persist; pass 0 to use the
// DefaultJobTTL.
func NewQueueClient(rdb *redis.Client, jobTTL time.Duration) *QueueClient {
	if jobTTL == 0 {
		jobTTL = DefaultJobTTL
	}
	return &QueueClient{rdb: rdb, jobTTL: jobTTL}
}

// Push enqueues a job at the tail of queueName.
// The job's UpdatedAt field is set to the current time before encoding.
func (q *QueueClient) Push(ctx context.Context, queueName string, job *Job) error {
	job.UpdatedAt = time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = job.UpdatedAt
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue push: marshal job: %w", err)
	}

	if err := q.rdb.RPush(ctx, queueName, data).Err(); err != nil {
		return fmt.Errorf("queue push %q: %w", queueName, err)
	}

	return q.TrackStatus(ctx, job.ID, string(JobStatusQueued), q.jobTTL)
}

// BlockingPop waits up to timeout for a job at the head of queueName.
// It returns (nil, nil) when the timeout elapses with no job available.
func (q *QueueClient) BlockingPop(ctx context.Context, queueName string, timeout time.Duration) (*Job, error) {
	result, err := q.rdb.BLPop(ctx, timeout, queueName).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("queue pop %q: %w", queueName, err)
	}

	// BLPop returns [key, value]; value is at index 1.
	if len(result) < 2 {
		return nil, nil
	}

	var job Job
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("queue pop: unmarshal job: %w", err)
	}

	return &job, nil
}

// PushDead moves a job to its dead-letter queue so it can be inspected or
// replayed later without clogging the main queue.
func (q *QueueClient) PushDead(ctx context.Context, queueName string, job *Job) error {
	job.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("queue dead-letter: marshal job: %w", err)
	}

	dlq, err := DeadLetterQueueName(queueName)
	if err != nil {
		return fmt.Errorf("queue dead-letter name %q: %w", queueName, err)
	}
	if err := q.rdb.RPush(ctx, dlq, data).Err(); err != nil {
		return fmt.Errorf("queue dead-letter push %q: %w", dlq, err)
	}

	return q.TrackStatus(ctx, job.ID, string(JobStatusDead), q.jobTTL)
}

// TrackStatus stores the status of a job in Redis with an expiry.
func (q *QueueClient) TrackStatus(ctx context.Context, jobID, status string, ttl time.Duration) error {
	if jobID == "" {
		return nil
	}

	key := statusKey(jobID)
	if err := q.rdb.Set(ctx, key, status, ttl).Err(); err != nil {
		return fmt.Errorf("track status for job %q: %w", jobID, err)
	}

	return nil
}

// GetStatus retrieves the last-known status for a job.
// It returns an empty string when the key has expired or does not exist.
func (q *QueueClient) GetStatus(ctx context.Context, jobID string) (string, error) {
	val, err := q.rdb.Get(ctx, statusKey(jobID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get status for job %q: %w", jobID, err)
	}

	return val, nil
}

// QueueLength returns the number of jobs waiting in queueName.
func (q *QueueClient) QueueLength(ctx context.Context, queueName string) (int64, error) {
	n, err := q.rdb.LLen(ctx, queueName).Result()
	if err != nil {
		return 0, fmt.Errorf("queue length %q: %w", queueName, err)
	}

	return n, nil
}

// DeadQueueLength returns the number of jobs in the dead-letter queue for
// queueName.
func (q *QueueClient) DeadQueueLength(ctx context.Context, queueName string) (int64, error) {
	dlq, err := DeadLetterQueueName(queueName)
	if err != nil {
		return 0, fmt.Errorf("queue dead-letter name %q: %w", queueName, err)
	}
	return q.QueueLength(ctx, dlq)
}

// Metrics returns a snapshot of queue lengths for all known queues.
func (q *QueueClient) Metrics(ctx context.Context) (map[string]int64, error) {
	queues := []string{
		QueueTranscript,
		QueueClip,
		QueueSubtitle,
		QueueUpload,
		QueueAnalytics,
	}

	metrics := make(map[string]int64, len(queues)*2)

	for _, name := range queues {
		n, err := q.QueueLength(ctx, name)
		if err != nil {
			return nil, err
		}
		metrics[name] = n

		dlqName, err := DeadLetterQueueName(name)
		if err != nil {
			return nil, err
		}

		d, err := q.QueueLength(ctx, dlqName)
		if err != nil {
			return nil, err
		}
		metrics[dlqName] = d
	}

	return metrics, nil
}
