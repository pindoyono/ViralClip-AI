package services

import (
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

func setupViralOpportunityServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.ContentProfile{}, &models.ViralOpportunity{}))
	return db
}

func TestRecommendationEngineRecommend_MatchesProfiles(t *testing.T) {
	engine := NewRecommendationEngine()
	now := time.Now().UTC()
	opportunity := models.ViralOpportunity{
		Base:            models.Base{ID: uuid.New()},
		SourcePlatform:  "youtube",
		ExternalVideoID: "video-1",
		Title:           "Gaming boss fight strategy guide",
		Category:        "Gaming",
		SourceQuery:     "gaming strategy",
		Views:           100000,
		PreviousViews:   75000,
		Likes:           5500,
		Comments:        450,
		SubscriberCount: 20000,
		PublishedAt:     now.Add(-2 * time.Hour),
		LastCollectedAt: now,
		EngagementRate:  0.0595,
		OutlierScore:    5,
		GrowthScore:     25000,
		ViralScore:      5127,
	}
	profiles := []models.ContentProfile{{Name: "Gaming Core", Platform: "youtube", Niche: "gaming", Keywords: "boss fight, strategy"}}

	results := engine.Recommend([]models.ViralOpportunity{opportunity}, profiles, 10)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].MatchedProfiles, "Gaming Core")
	assert.NotEmpty(t, results[0].Reasons)
	assert.Greater(t, results[0].RecommendationScore, opportunity.ViralScore*0.7)
}

func TestViralOpportunityServiceTrending_OrdersRecentResults(t *testing.T) {
	db := setupViralOpportunityServiceDB(t)
	service := NewViralOpportunityService(db, NewRecommendationEngine())
	now := time.Now().UTC()

	require.NoError(t, db.Create(&models.ViralOpportunity{
		Base:            models.Base{ID: uuid.New()},
		SourcePlatform:  "youtube",
		ExternalVideoID: "recent-top",
		Title:           "Recent winner",
		Category:        "Education",
		Views:           200000,
		PublishedAt:     now.Add(-6 * time.Hour),
		LastCollectedAt: now,
		GrowthScore:     20000,
		ViralScore:      300,
	}).Error)
	require.NoError(t, db.Create(&models.ViralOpportunity{
		Base:            models.Base{ID: uuid.New()},
		SourcePlatform:  "youtube",
		ExternalVideoID: "old-video",
		Title:           "Old clip",
		Category:        "Education",
		Views:           500000,
		PublishedAt:     now.Add(-96 * time.Hour),
		LastCollectedAt: now,
		GrowthScore:     50000,
		ViralScore:      999,
	}).Error)

	results, err := service.Trending(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "recent-top", results[0].ExternalVideoID)
}
