package queue_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// newTestClient spins up an in-process miniredis server and returns a QueueClient
// wired to it. The server is closed automatically when the test ends.
func newTestClient(t *testing.T) *queue.QueueClient {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return queue.NewQueueClient(rdb, time.Minute)
}

// --- Push & BlockingPop ---

func TestPushAndPop_RoundTrip(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	job := &queue.Job{
		ID:          "job-1",
		Type:        queue.JobTypeTranscript,
		VideoID:     "vid-1",
		UserID:      "user-1",
		StoragePath: "/storage/v.mp4",
		MaxRetries:  3,
	}

	require.NoError(t, q.Push(ctx, queue.QueueTranscript, job))

	got, err := q.BlockingPop(ctx, queue.QueueTranscript, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, job.VideoID, got.VideoID)
	assert.Equal(t, job.StoragePath, got.StoragePath)
	assert.Equal(t, queue.JobTypeTranscript, got.Type)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestBlockingPop_EmptyQueue_ReturnsNil(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	got, err := q.BlockingPop(ctx, queue.QueueTranscript, 50*time.Millisecond)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestPush_SetsCreatedAt(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	before := time.Now().UTC().Truncate(time.Second)
	job := &queue.Job{ID: "j2", Type: queue.JobTypeClip, VideoID: "v2", UserID: "u1", MaxRetries: 1}
	require.NoError(t, q.Push(ctx, queue.QueueClip, job))

	got, err := q.BlockingPop(ctx, queue.QueueClip, 100*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.False(t, got.CreatedAt.IsZero())
	assert.True(t, got.CreatedAt.After(before) || got.CreatedAt.Equal(before))
}

// --- Dead-letter queue ---

func TestPushDead_JobAppearsInDLQ(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	job := &queue.Job{ID: "dead-1", VideoID: "vid-dead", UserID: "u1", RetryCount: 5, MaxRetries: 3}
	require.NoError(t, q.PushDead(ctx, queue.QueueTranscript, job))

	n, err := q.DeadQueueLength(ctx, queue.QueueTranscript)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// Main queue is empty
	mainLen, err := q.QueueLength(ctx, queue.QueueTranscript)
	require.NoError(t, err)
	assert.Equal(t, int64(0), mainLen)
}

// --- Status tracking ---

func TestTrackStatus_SetAndGet(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	require.NoError(t, q.TrackStatus(ctx, "job-status-1", string(queue.JobStatusProcessing), time.Minute))

	status, err := q.GetStatus(ctx, "job-status-1")
	require.NoError(t, err)
	assert.Equal(t, string(queue.JobStatusProcessing), status)
}

func TestGetStatus_ExpiredOrMissing_ReturnsEmpty(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	status, err := q.GetStatus(ctx, "nonexistent-job")
	require.NoError(t, err)
	assert.Empty(t, status)
}

func TestTrackStatus_EmptyID_IsNoOp(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	// Should not return an error even for empty job IDs.
	require.NoError(t, q.TrackStatus(ctx, "", string(queue.JobStatusQueued), time.Minute))
}

func TestPush_SetsStatusQueued(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	job := &queue.Job{ID: "j-status", VideoID: "v", UserID: "u", MaxRetries: 1}
	require.NoError(t, q.Push(ctx, queue.QueueSubtitle, job))

	status, err := q.GetStatus(ctx, "j-status")
	require.NoError(t, err)
	assert.Equal(t, string(queue.JobStatusQueued), status)
}

func TestPushDead_SetsStatusDead(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	job := &queue.Job{ID: "j-dead", VideoID: "v", UserID: "u", MaxRetries: 1}
	require.NoError(t, q.PushDead(ctx, queue.QueueClip, job))

	status, err := q.GetStatus(ctx, "j-dead")
	require.NoError(t, err)
	assert.Equal(t, string(queue.JobStatusDead), status)
}

// --- Queue length & metrics ---

func TestQueueLength(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		job := &queue.Job{ID: fmt.Sprintf("j%d", i), VideoID: "v", UserID: "u", MaxRetries: 1}
		require.NoError(t, q.Push(ctx, queue.QueueUpload, job))
	}

	n, err := q.QueueLength(ctx, queue.QueueUpload)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
}

func TestMetrics_AllQueues(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	// Push one job to QueueAnalytics and one dead to QueueClip.
	require.NoError(t, q.Push(ctx, queue.QueueAnalytics, &queue.Job{ID: "a1", VideoID: "v", UserID: "u", MaxRetries: 1}))
	require.NoError(t, q.PushDead(ctx, queue.QueueClip, &queue.Job{ID: "c1", VideoID: "v", UserID: "u", MaxRetries: 1}))

	metrics, err := q.Metrics(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(1), metrics[queue.QueueAnalytics])
	assert.Equal(t, int64(1), metrics[queue.QueueClip+":dead"])
	// All other queues should be 0.
	assert.Equal(t, int64(0), metrics[queue.QueueTranscript])
}

// --- FIFO ordering ---

func TestPush_FIFOOrder(t *testing.T) {
	q := newTestClient(t)
	ctx := context.Background()

	ids := []string{"first", "second", "third"}
	for _, id := range ids {
		require.NoError(t, q.Push(ctx, queue.QueueTranscript, &queue.Job{ID: id, VideoID: "v", UserID: "u", MaxRetries: 1}))
	}

	for _, expected := range ids {
		got, err := q.BlockingPop(ctx, queue.QueueTranscript, 100*time.Millisecond)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, expected, got.ID)
	}
}
