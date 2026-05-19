package workers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/queue"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupDLQDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&FailedJobRecord{}))
	return db
}

func newDLQTestClient(t *testing.T) (*queue.QueueClient, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cli := queue.NewQueueClient(rdb, time.Minute)
	return cli, mr, rdb
}

// pushToDLQ marshals a Job and RPUSH-es it directly onto the dead-letter queue.
func pushToDLQ(t *testing.T, rdb *redis.Client, origQueue string, job *queue.Job) {
	t.Helper()
	data, err := json.Marshal(job)
	require.NoError(t, err)
	dlqName, err := queue.DeadLetterQueueName(origQueue)
	require.NoError(t, err)
	require.NoError(t, rdb.RPush(context.Background(), dlqName, data).Err())
}

// ---------------------------------------------------------------------------
// DeadLetterWorker tests
// ---------------------------------------------------------------------------

func TestNewDeadLetterWorker(t *testing.T) {
	db := setupDLQDB(t)
	cli, _, _ := newDLQTestClient(t)
	w := NewDeadLetterWorker(db, cli)
	assert.NotNil(t, w)
}

func TestDeadLetterWorker_PersistJob(t *testing.T) {
	db := setupDLQDB(t)
	cli, _, rdb := newDLQTestClient(t)

	job := &queue.Job{
		ID:         "job-persist-test",
		Type:       queue.JobTypeTranscript,
		VideoID:    "vid-1",
		UserID:     "usr-1",
		RetryCount: 3,
		MaxRetries: 3,
		Metadata:   map[string]string{"error": "ai service timed out"},
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	pushToDLQ(t, rdb, queue.QueueTranscript, job)

	// Run worker with a context that we cancel after a short time.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w := NewDeadLetterWorker(db, cli)
	// consumeDLQ is tested directly here to avoid timing issues in CI.
	w.consumeDLQ(ctx, queue.QueueTranscript)

	var records []FailedJobRecord
	require.NoError(t, db.Find(&records).Error)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, job.ID, r.JobID)
	assert.Equal(t, queue.QueueTranscript, r.QueueName)
	assert.Equal(t, "ai service timed out", r.ErrorMessage)
	assert.Equal(t, job.RetryCount, r.RetryCount)
	assert.Equal(t, job.MaxRetries, r.MaxRetries)
	assert.Equal(t, "pending", r.Status)
}

func TestDeadLetterWorker_PersistJob_NoError(t *testing.T) {
	db := setupDLQDB(t)
	cli, _, rdb := newDLQTestClient(t)

	// Job without error metadata (e.g. manually pushed).
	job := &queue.Job{
		ID:         "job-no-err",
		Type:       queue.JobTypeClip,
		VideoID:    "vid-2",
		RetryCount: 1,
		MaxRetries: 3,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	pushToDLQ(t, rdb, queue.QueueClip, job)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w := NewDeadLetterWorker(db, cli)
	w.consumeDLQ(ctx, queue.QueueClip)

	var records []FailedJobRecord
	require.NoError(t, db.Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "", records[0].ErrorMessage)
}

func TestDeadLetterWorker_MultipleJobs(t *testing.T) {
	db := setupDLQDB(t)
	cli, _, rdb := newDLQTestClient(t)

	for i := 0; i < 3; i++ {
		job := &queue.Job{
			ID:         "job-multi-" + string(rune('A'+i)),
			Type:       queue.JobTypeUpload,
			VideoID:    "vid-" + string(rune('A'+i)),
			RetryCount: i + 1,
			MaxRetries: 5,
			Metadata:   map[string]string{"error": "timeout"},
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		pushToDLQ(t, rdb, queue.QueueUpload, job)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	w := NewDeadLetterWorker(db, cli)
	w.consumeDLQ(ctx, queue.QueueUpload)

	var records []FailedJobRecord
	require.NoError(t, db.Find(&records).Error)
	assert.Len(t, records, 3)
}

func TestDeadLetterWorker_Start_CancelsOnContextDone(t *testing.T) {
	db := setupDLQDB(t)
	cli, _, _ := newDLQTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	w := NewDeadLetterWorker(db, cli)

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DeadLetterWorker.Start did not exit after context cancellation")
	}
}
