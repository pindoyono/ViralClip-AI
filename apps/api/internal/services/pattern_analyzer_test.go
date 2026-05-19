package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
)

func TestHookPerformanceTracker_Collect(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	videoID, clipID := seedVideoAndClip(t, db, userID, "Hook Clip", 40, 0.6)

	require.NoError(t, db.Create(&models.HookDetection{
		Base:     models.Base{ID: uuid.New()},
		VideoID:  videoID,
		UserID:   userID,
		Start:    0,
		End:      4,
		HookType: "storytelling",
		Score:    90,
	}).Error)

	require.NoError(t, db.Create(&models.ClipAnalytics{
		Base:           models.Base{ID: uuid.New()},
		ClipID:         clipID,
		Platform:       models.PlatformTikTok,
		RecordedAt:     time.Now().UTC(),
		Views:          1000,
		Likes:          100,
		Comments:       25,
		WatchTime:      20,
		CTR:            0.2,
		SubscriberGain: 40,
	}).Error)

	tracker := services.NewHookPerformanceTracker(db)
	records, err := tracker.Collect(context.Background(), userID.String(), "tiktok")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, "storytelling", r.HookType)
	assert.Equal(t, int64(1000), r.Views)
	assert.Equal(t, 20.0, r.WatchTime)
	assert.InDelta(t, 0.5, r.Retention, 0.001)
	assert.Equal(t, int64(100), r.Likes)
	assert.Equal(t, int64(25), r.Comments)
	assert.Equal(t, 0.2, r.CTR)
	assert.Equal(t, int64(40), r.SubscriberGain)
	assert.Greater(t, r.CPS, 0.0)
}

func TestPatternAnalyzer_Analyze(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)

	video1, clip1 := seedVideoAndClip(t, db, userID, "Story Clip", 30, 0.8)
	video2, clip2 := seedVideoAndClip(t, db, userID, "CTA Clip", 30, 0.4)

	require.NoError(t, db.Create(&models.HookDetection{
		Base:     models.Base{ID: uuid.New()},
		VideoID:  video1,
		UserID:   userID,
		Start:    0,
		End:      3,
		HookType: "storytelling",
		Score:    92,
	}).Error)
	require.NoError(t, db.Create(&models.HookDetection{
		Base:     models.Base{ID: uuid.New()},
		VideoID:  video2,
		UserID:   userID,
		Start:    0,
		End:      3,
		HookType: "cta",
		Score:    70,
	}).Error)

	require.NoError(t, db.Create(&models.ClipAnalytics{
		Base:           models.Base{ID: uuid.New()},
		ClipID:         clip1,
		Platform:       models.PlatformTikTok,
		RecordedAt:     time.Now().UTC(),
		Views:          5000,
		Likes:          550,
		Comments:       120,
		WatchTime:      24,
		CTR:            0.35,
		SubscriberGain: 180,
	}).Error)
	require.NoError(t, db.Create(&models.ClipAnalytics{
		Base:           models.Base{ID: uuid.New()},
		ClipID:         clip2,
		Platform:       models.PlatformTikTok,
		RecordedAt:     time.Now().UTC(),
		Views:          500,
		Likes:          30,
		Comments:       10,
		WatchTime:      8,
		CTR:            0.04,
		SubscriberGain: 5,
	}).Error)

	analyzer := services.NewPatternAnalyzer(services.NewHookPerformanceTracker(db))
	patterns, err := analyzer.Analyze(context.Background(), userID.String(), "tiktok")
	require.NoError(t, err)
	require.Len(t, patterns, 2)

	assert.Equal(t, "storytelling", patterns[0].HookType)
	assert.Greater(t, patterns[0].AvgCPS, patterns[1].AvgCPS)
	assert.Greater(t, patterns[0].Improvement, 0.0)
}
