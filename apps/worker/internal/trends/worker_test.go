package trends

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type stubCollector struct {
	videos []CollectedVideo
}

func (s stubCollector) Collect(ctx context.Context, queries []string) ([]CollectedVideo, error) {
	return s.videos, nil
}

func setupTrendWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ViralOpportunity{}, &ContentProfile{}))
	return db
}

func TestTrendCollectorWorkerRunOnceUpserts(t *testing.T) {
	db := setupTrendWorkerDB(t)
	require.NoError(t, db.Create(&ContentProfile{Niche: "gaming", Keywords: "boss, strategy"}).Error)
	require.NoError(t, db.Create(&ViralOpportunity{SourcePlatform: sourcePlatformYouTube, ExternalVideoID: "video-1", Views: 900}).Error)

	now := time.Now().UTC()
	worker := NewTrendCollectorWorker(db, stubCollector{videos: []CollectedVideo{{
		SourcePlatform:  sourcePlatformYouTube,
		ExternalVideoID: "video-1",
		ChannelID:       "channel-1",
		Title:           "Boss strategy",
		Category:        "Gaming",
		SourceQuery:     "boss",
		Views:           1200,
		Likes:           80,
		Comments:        20,
		SubscriberCount: 500,
		PublishedAt:     now.Add(-2 * time.Hour),
	}}}, NewTrendEngine())
	worker.now = func() time.Time { return now }

	require.NoError(t, worker.RunOnce(t.Context()))

	var stored ViralOpportunity
	require.NoError(t, db.First(&stored, "external_video_id = ?", "video-1").Error)
	assert.Equal(t, int64(1200), stored.Views)
	assert.Equal(t, int64(900), stored.PreviousViews)
	assert.Equal(t, 300.0, stored.GrowthScore)
}
