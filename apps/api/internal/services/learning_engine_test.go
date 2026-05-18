package services_test

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
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func setupLearningDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.ContentProfile{},
		&models.Video{},
		&models.Clip{},
		&models.ClipAnalytics{},
		&models.HookDetection{},
	))
	return db
}

func seedUser(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	u := models.User{
		Base:            models.Base{ID: uuid.New()},
		Email:           "test@example.com",
		PasswordHash:    "x",
		Name:            "Test",
		IsEmailVerified: true,
		IsActive:        true,
	}
	require.NoError(t, db.Create(&u).Error)
	return u.ID
}

func seedVideoAndClip(t *testing.T, db *gorm.DB, userID uuid.UUID, title string, duration, viralScore float64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	vid := models.Video{
		Base:   models.Base{ID: uuid.New()},
		UserID: userID,
		Title:  title,
		Status: models.VideoStatusCompleted,
	}
	require.NoError(t, db.Create(&vid).Error)

	clip := models.Clip{
		Base:       models.Base{ID: uuid.New()},
		VideoID:    vid.ID,
		UserID:     userID,
		Title:      title,
		Duration:   duration,
		ViralScore: viralScore,
		Status:     models.ClipStatusReady,
		Hashtags:   "[]",
	}
	require.NoError(t, db.Create(&clip).Error)
	return vid.ID, clip.ID
}

func seedAnalytics(t *testing.T, db *gorm.DB, clipID uuid.UUID, platform string, views, likes, comments int64, watchTime float64) {
	t.Helper()
	a := models.ClipAnalytics{
		Base:       models.Base{ID: uuid.New()},
		ClipID:     clipID,
		Platform:   models.SocialPlatform(platform),
		RecordedAt: time.Now().UTC(),
		Views:      views,
		Likes:      likes,
		Comments:   comments,
		WatchTime:  watchTime,
	}
	require.NoError(t, db.Create(&a).Error)
}

// ---------------------------------------------------------------------------
// ComputeCPS unit tests
// ---------------------------------------------------------------------------

func TestComputeCPS_AllZero(t *testing.T) {
	score := services.ComputeCPS(services.CPSMetrics{})
	assert.Equal(t, 0.0, score)
}

func TestComputeCPS_MaxValues(t *testing.T) {
	m := services.CPSMetrics{
		Views:          10000,
		WatchTime:      60,
		Duration:       60,
		Likes:          10000,
		Comments:       10000,
		Shares:         10000,
		Saves:          10000,
		CTR:            1.0,
		SubscriberGain: 1000,
	}
	score := services.ComputeCPS(m)
	assert.Equal(t, 100.0, score, "max metrics should yield 100 CPS")
}

func TestComputeCPS_WatchTimeWeighted(t *testing.T) {
	// Only watch time contributed (30% weight, half of 60s = 0.5 normalised → 15 pts).
	m := services.CPSMetrics{WatchTime: 30}
	score := services.ComputeCPS(m)
	assert.InDelta(t, 15.0, score, 0.1)
}

func TestComputeCPS_RetentionComponent(t *testing.T) {
	// Full retention (WatchTime == Duration) → 25% weight alone → 25 pts.
	m := services.CPSMetrics{WatchTime: 45, Duration: 45}
	score := services.ComputeCPS(m)
	// watch-time: 45/60*30 = 22.5; retention: 1*25 = 25 → total = 47.5
	assert.InDelta(t, 47.5, score, 0.1)
}

func TestComputeCPS_EngagementComponent(t *testing.T) {
	m := services.CPSMetrics{
		Views: 1000,
		Likes: 500,
	}
	score := services.ComputeCPS(m)
	// engagement norm = 500/1000 = 0.5 → 0.5*20 = 10
	assert.InDelta(t, 10.0, score, 0.1)
}

// ---------------------------------------------------------------------------
// LearningEngine integration tests
// ---------------------------------------------------------------------------

func TestLearningEngine_TopClips_Empty(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	clips, err := engine.TopClips(context.Background(), userID.String(), "", 10)
	require.NoError(t, err)
	assert.Empty(t, clips)
}

func TestLearningEngine_TopClips_Ordered(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	_, clip1 := seedVideoAndClip(t, db, userID, "Low Views", 30, 0.5)
	_, clip2 := seedVideoAndClip(t, db, userID, "High Views", 30, 0.9)

	seedAnalytics(t, db, clip1, "tiktok", 100, 10, 2, 10)
	seedAnalytics(t, db, clip2, "tiktok", 5000, 500, 100, 25)

	clips, err := engine.TopClips(context.Background(), userID.String(), "", 10)
	require.NoError(t, err)
	require.Len(t, clips, 2)
	// clip2 should rank first (higher CPS)
	assert.Equal(t, clip2, clips[0].ClipID)
}

func TestLearningEngine_WorstClips_Ordered(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	_, clip1 := seedVideoAndClip(t, db, userID, "Low Views", 30, 0.5)
	_, clip2 := seedVideoAndClip(t, db, userID, "High Views", 30, 0.9)

	seedAnalytics(t, db, clip1, "tiktok", 100, 10, 2, 10)
	seedAnalytics(t, db, clip2, "tiktok", 5000, 500, 100, 25)

	clips, err := engine.WorstClips(context.Background(), userID.String(), "", 10)
	require.NoError(t, err)
	require.Len(t, clips, 2)
	// clip1 should rank first (lower CPS)
	assert.Equal(t, clip1, clips[0].ClipID)
}

func TestLearningEngine_TopClips_PlatformFilter(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	_, clip1 := seedVideoAndClip(t, db, userID, "TikTok Clip", 30, 0.8)
	_, clip2 := seedVideoAndClip(t, db, userID, "YouTube Clip", 30, 0.7)

	seedAnalytics(t, db, clip1, "tiktok", 1000, 100, 20, 15)
	seedAnalytics(t, db, clip2, "youtube", 2000, 200, 30, 20)

	tiktokClips, err := engine.TopClips(context.Background(), userID.String(), "tiktok", 10)
	require.NoError(t, err)
	require.Len(t, tiktokClips, 1)
	assert.Equal(t, clip1, tiktokClips[0].ClipID)
}

func TestLearningEngine_HookPatterns_NoData(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	patterns, err := engine.HookPatterns(context.Background(), userID.String(), "")
	require.NoError(t, err)
	assert.Empty(t, patterns)
}

func TestLearningEngine_Recommendations_NoProfiles(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	recs, err := engine.Recommendations(context.Background(), userID.String())
	require.NoError(t, err)
	assert.Empty(t, recs)
}

func TestLearningEngine_Recommendations_WithProfile(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)

	// Create a content profile.
	profile := models.ContentProfile{
		Base:     models.Base{ID: uuid.New()},
		UserID:   userID,
		Name:     "Gaming Core",
		Platform: "tiktok",
		Niche:    "gaming",
	}
	require.NoError(t, db.Create(&profile).Error)

	engine := services.NewLearningEngine(db)
	recs, err := engine.Recommendations(context.Background(), userID.String())
	require.NoError(t, err)
	// With no analytics data recommendations may be empty but should not error.
	assert.NotNil(t, recs)
}

func TestLearningEngine_TopClips_LimitRespected(t *testing.T) {
	db := setupLearningDB(t)
	userID := seedUser(t, db)
	engine := services.NewLearningEngine(db)

	for i := 0; i < 5; i++ {
		_, clipID := seedVideoAndClip(t, db, userID, "clip", 30, 0.5)
		seedAnalytics(t, db, clipID, "tiktok", int64(100+i*100), 10, 2, 10)
	}

	clips, err := engine.TopClips(context.Background(), userID.String(), "", 3)
	require.NoError(t, err)
	assert.Len(t, clips, 3)
}
