package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

func setupClipV2HistoricalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Video{}, &models.Clip{}, &models.ClipAnalytics{}))
	return db
}

func TestClipHandlerV2_BuildHistoricalRetentionContext_UsesClipAnalytics(t *testing.T) {
	db := setupClipV2HistoricalDB(t)
	h := &ClipHandlerV2{db: db}

	userID := uuid.New()
	require.NoError(t, db.Create(&models.User{
		Base:            models.Base{ID: userID},
		Email:           "hist@test.com",
		PasswordHash:    "x",
		Name:            "Hist",
		IsActive:        true,
		IsEmailVerified: true,
	}).Error)

	videoID := uuid.New()
	require.NoError(t, db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: userID,
		Title:  "v",
		Status: models.VideoStatusCompleted,
	}).Error)

	shortClipID := uuid.New()
	require.NoError(t, db.Create(&models.Clip{
		Base:       models.Base{ID: shortClipID},
		VideoID:    videoID,
		UserID:     userID,
		Title:      "short",
		Duration:   20,
		Status:     models.ClipStatusReady,
		Hashtags:   "[]",
		ViralScore: 0.3,
	}).Error)
	require.NoError(t, db.Create(&models.ClipAnalytics{
		Base:       models.Base{ID: uuid.New()},
		ClipID:     shortClipID,
		Platform:   models.PlatformTikTok,
		RecordedAt: time.Now().UTC(),
		Views:      100,
		WatchTime:  18, // 0.9 retention
	}).Error)

	longClipID := uuid.New()
	require.NoError(t, db.Create(&models.Clip{
		Base:       models.Base{ID: longClipID},
		VideoID:    videoID,
		UserID:     userID,
		Title:      "long",
		Duration:   40,
		Status:     models.ClipStatusReady,
		Hashtags:   "[]",
		ViralScore: 0.3,
	}).Error)
	require.NoError(t, db.Create(&models.ClipAnalytics{
		Base:       models.Base{ID: uuid.New()},
		ClipID:     longClipID,
		Platform:   models.PlatformTikTok,
		RecordedAt: time.Now().UTC(),
		Views:      100,
		WatchTime:  20, // 0.5 retention
	}).Error)

	ctx := context.Background()
	hist, err := h.buildHistoricalRetentionContext(ctx, userID.String())
	require.NoError(t, err)
	require.NotNil(t, hist)

	assert.Equal(t, int64(2), hist.SampleSize)
	assert.InDelta(t, 0.7, hist.AvgRetention, 0.001)
	assert.InDelta(t, 0.9, hist.ShortRetention, 0.001)
	assert.InDelta(t, 0.5, hist.LongRetention, 0.001)
}

func TestClipHandlerV2_BuildHistoricalRetentionContext_EmptyWhenNoData(t *testing.T) {
	db := setupClipV2HistoricalDB(t)
	h := &ClipHandlerV2{db: db}

	userID := uuid.New()
	require.NoError(t, db.Create(&models.User{
		Base:            models.Base{ID: userID},
		Email:           "empty@test.com",
		PasswordHash:    "x",
		Name:            "Empty",
		IsActive:        true,
		IsEmailVerified: true,
	}).Error)

	hist, err := h.buildHistoricalRetentionContext(context.Background(), userID.String())
	require.NoError(t, err)
	assert.Nil(t, hist)
}
