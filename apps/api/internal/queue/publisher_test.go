package queue_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/queue"
)

func newTestPublisher(t *testing.T) (*queue.Publisher, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return queue.NewPublisher(rdb, 3), mr
}

func TestPublishTranscriptJob_PushesJobToTranscriptQueue(t *testing.T) {
	pub, mr := newTestPublisher(t)
	ctx := context.Background()

	err := pub.PublishTranscriptJob(ctx, "job-1", "vid-1", "user-1", "/storage/v.mp4", "http://cdn/v.mp4")
	require.NoError(t, err)

	// Verify job is in the transcript_queue Redis list.
	raw, lpopErr := mr.Lpop(queue.QueueTranscript)
	require.NoError(t, lpopErr)
	require.NotEmpty(t, raw)

	var job queue.Job
	require.NoError(t, json.Unmarshal([]byte(raw), &job))

	assert.Equal(t, "job-1", job.ID)
	assert.Equal(t, "vid-1", job.VideoID)
	assert.Equal(t, "user-1", job.UserID)
	assert.Equal(t, "/storage/v.mp4", job.StoragePath)
	assert.Equal(t, "http://cdn/v.mp4", job.StorageURL)
	assert.Equal(t, queue.JobTypeTranscript, job.Type)
	assert.Equal(t, 3, job.MaxRetries)
	assert.False(t, job.CreatedAt.IsZero())
}

func TestPublishTranscriptJob_QueueNameIsTranscriptQueue(t *testing.T) {
	pub, mr := newTestPublisher(t)
	ctx := context.Background()

	require.NoError(t, pub.PublishTranscriptJob(ctx, "j", "v", "u", "/p", ""))

	// Should NOT appear in any other queue.
	v1, _ := mr.Lpop(queue.QueueClip)
	assert.Empty(t, v1)
	v2, _ := mr.Lpop(queue.QueueSubtitle)
	assert.Empty(t, v2)
	v3, _ := mr.Lpop(queue.QueueUpload)
	assert.Empty(t, v3)
	v4, _ := mr.Lpop(queue.QueueAnalytics)
	assert.Empty(t, v4)

	// Must appear in transcript_queue.
	v5, _ := mr.Lpop(queue.QueueTranscript)
	assert.NotEmpty(t, v5)
}

func TestPublishAnalyticsJob_PushesJobToAnalyticsQueue(t *testing.T) {
	pub, mr := newTestPublisher(t)
	ctx := context.Background()

	meta := map[string]string{"post_id": "p-1", "platform": "youtube"}
	err := pub.PublishAnalyticsJob(ctx, "job-analytics-1", "vid-2", "user-2", meta)
	require.NoError(t, err)

	raw, lpopErr := mr.Lpop(queue.QueueAnalytics)
	require.NoError(t, lpopErr)
	require.NotEmpty(t, raw)

	var job queue.Job
	require.NoError(t, json.Unmarshal([]byte(raw), &job))

	assert.Equal(t, queue.JobTypeAnalytics, job.Type)
	assert.Equal(t, "p-1", job.Metadata["post_id"])
	assert.Equal(t, "youtube", job.Metadata["platform"])
}

func TestPublisher_FIFO_OrderPreserved(t *testing.T) {
	pub, mr := newTestPublisher(t)
	ctx := context.Background()

	ids := []string{"first", "second", "third"}
	for _, id := range ids {
		require.NoError(t, pub.PublishTranscriptJob(ctx, id, "v", "u", "/p", ""))
	}

	for _, expected := range ids {
		raw, lpopErr := mr.Lpop(queue.QueueTranscript)
		require.NoError(t, lpopErr)
		require.NotEmpty(t, raw)
		var job queue.Job
		require.NoError(t, json.Unmarshal([]byte(raw), &job))
		assert.Equal(t, expected, job.ID)
	}
}
