package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
)

func setupViralOpportunityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.ContentProfile{}, &models.ViralOpportunity{}))
	return db
}

func setupViralOpportunityApp(db *gorm.DB, userID string) *fiber.App {
	app := fiber.New()
	service := services.NewViralOpportunityService(db, services.NewRecommendationEngine())
	h := NewViralOpportunityHandler(service)

	app.Use(func(c *fiber.Ctx) error {
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"user_id": userID, "exp": time.Now().Add(15 * time.Minute).Unix()})
			signed, _ := tok.SignedString([]byte("test-secret"))
			parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) { return []byte("test-secret"), nil })
			c.Locals("user_token", parsed)
		}
		return c.Next()
	})

	app.Get("/viral-opportunities", h.List)
	app.Get("/viral-opportunities/trending", h.Trending)
	app.Get("/viral-opportunities/recommendations", h.Recommendations)
	return app
}

func seedViralOpportunityFixtures(t *testing.T, db *gorm.DB, userID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.User{Base: models.Base{ID: userID}, Email: "viral@example.com", PasswordHash: "hash", Name: "Viral User", IsActive: true}).Error)
	require.NoError(t, db.Create(&models.ContentProfile{Base: models.Base{ID: uuid.New()}, UserID: userID, Name: "Gaming", Platform: "youtube", Niche: "gaming", Keywords: "boss, strategy"}).Error)
	require.NoError(t, db.Create(&models.ViralOpportunity{
		Base:            models.Base{ID: uuid.New()},
		SourcePlatform:  "youtube",
		ExternalVideoID: "top-hit",
		ChannelID:       "chan-1",
		Title:           "Gaming boss strategy that exploded",
		Category:        "Gaming",
		SourceQuery:     "gaming strategy",
		Views:           120000,
		PreviousViews:   90000,
		Likes:           5000,
		Comments:        500,
		SubscriberCount: 20000,
		PublishedAt:     now.Add(-3 * time.Hour),
		LastCollectedAt: now,
		ViewVelocity:    40000,
		EngagementRate:  0.0458,
		OutlierScore:    6,
		GrowthScore:     30000,
		ViralScore:      7400,
	}).Error)
	require.NoError(t, db.Create(&models.ViralOpportunity{
		Base:            models.Base{ID: uuid.New()},
		SourcePlatform:  "youtube",
		ExternalVideoID: "older",
		ChannelID:       "chan-2",
		Title:           "Education deep dive",
		Category:        "Education",
		SourceQuery:     "education",
		Views:           50000,
		PreviousViews:   45000,
		Likes:           1500,
		Comments:        120,
		SubscriberCount: 18000,
		PublishedAt:     now.Add(-96 * time.Hour),
		LastCollectedAt: now,
		ViewVelocity:    520,
		EngagementRate:  0.0324,
		OutlierScore:    2.77,
		GrowthScore:     5000,
		ViralScore:      1200,
	}).Error)
}

func TestViralOpportunityHandlerList(t *testing.T) {
	db := setupViralOpportunityDB(t)
	userID := uuid.New()
	seedViralOpportunityFixtures(t, db, userID)
	app := setupViralOpportunityApp(db, userID.String())

	req := httptest.NewRequest(http.MethodGet, "/viral-opportunities?category=gaming", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Data []map[string]interface{} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	assert.True(t, envelope.Success)
	assert.Len(t, envelope.Data.Data, 1)
	assert.Equal(t, "Gaming", envelope.Data.Data[0]["category"])
}

func TestViralOpportunityHandlerTrending(t *testing.T) {
	db := setupViralOpportunityDB(t)
	userID := uuid.New()
	seedViralOpportunityFixtures(t, db, userID)
	app := setupViralOpportunityApp(db, userID.String())

	req := httptest.NewRequest(http.MethodGet, "/viral-opportunities/trending", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	assert.True(t, envelope.Success)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "top-hit", envelope.Data[0]["external_video_id"])
}

func TestViralOpportunityHandlerRecommendations(t *testing.T) {
	db := setupViralOpportunityDB(t)
	userID := uuid.New()
	seedViralOpportunityFixtures(t, db, userID)
	app := setupViralOpportunityApp(db, userID.String())

	req := httptest.NewRequest(http.MethodGet, "/viral-opportunities/recommendations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope struct {
		Success bool `json:"success"`
		Data    []struct {
			Opportunity map[string]interface{} `json:"opportunity"`
			Reasons     []string               `json:"reasons"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	assert.True(t, envelope.Success)
	assert.True(t, envelope.Success)
	require.Len(t, envelope.Data, 1)
	assert.Equal(t, "top-hit", envelope.Data[0].Opportunity["external_video_id"])
	assert.NotEmpty(t, envelope.Data[0].Reasons)
}
