package handlers

import (
	"encoding/json"
	"net/http"
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
)

func setupAnalyticsLearningDB(t *testing.T) *gorm.DB {
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

func setupAnalyticsLearningApp(db *gorm.DB, userID string) *fiber.App {
	app := fiber.New()
	h := NewAnalyticsHandler(db)

	app.Use(func(c *fiber.Ctx) error {
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"exp":     time.Now().Add(30 * time.Minute).Unix(),
			})
			signed, _ := tok.SignedString([]byte("test-secret"))
			parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
				return []byte("test-secret"), nil
			})
			c.Locals("user_token", parsed)
		}
		return c.Next()
	})

	app.Get("/analytics/top-clips", h.TopClips)
	app.Get("/analytics/worst-clips", h.WorstClips)
	app.Get("/analytics/hook-patterns", h.HookPatterns)
	app.Get("/analytics/recommendations", h.LearningRecommendations)
	return app
}

func seedLearningData(t *testing.T, db *gorm.DB, userID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()

	vid := models.Video{
		Base:   models.Base{ID: uuid.New()},
		UserID: userID,
		Title:  "Test Video",
		Status: models.VideoStatusCompleted,
	}
	require.NoError(t, db.Create(&vid).Error)

	clip := models.Clip{
		Base:       models.Base{ID: uuid.New()},
		VideoID:    vid.ID,
		UserID:     userID,
		Title:      "Test Clip",
		Duration:   30,
		ViralScore: 0.8,
		Status:     models.ClipStatusReady,
		Hashtags:   "[]",
	}
	require.NoError(t, db.Create(&clip).Error)

	analytics := models.ClipAnalytics{
		Base:       models.Base{ID: uuid.New()},
		ClipID:     clip.ID,
		Platform:   models.PlatformTikTok,
		RecordedAt: time.Now().UTC(),
		Views:      1000,
		Likes:      100,
		Comments:   20,
		WatchTime:  15,
	}
	require.NoError(t, db.Create(&analytics).Error)

	return vid.ID, clip.ID
}

func TestAnalyticsTopClips_Empty(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New().String()
	app := setupAnalyticsLearningApp(db, userID)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/top-clips", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].([]any)
	assert.Empty(t, data)
}

func TestAnalyticsTopClips_WithData(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New()
	app := setupAnalyticsLearningApp(db, userID.String())
	_, clipID := seedLearningData(t, db, userID)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/top-clips", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].([]any)
	require.Len(t, data, 1)
	assert.Equal(t, clipID.String(), data[0].(map[string]any)["clip_id"].(string))
	cps := data[0].(map[string]any)["cps"].(float64)
	assert.Greater(t, cps, 0.0)
}

func TestAnalyticsWorstClips_WithData(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New()
	app := setupAnalyticsLearningApp(db, userID.String())

	// Seed two clips with different performance
	vid := models.Video{Base: models.Base{ID: uuid.New()}, UserID: userID, Title: "v", Status: models.VideoStatusCompleted}
	require.NoError(t, db.Create(&vid).Error)

	clip1 := models.Clip{Base: models.Base{ID: uuid.New()}, VideoID: vid.ID, UserID: userID, Title: "Weak", Duration: 30, ViralScore: 0.1, Status: models.ClipStatusReady, Hashtags: "[]"}
	clip2 := models.Clip{Base: models.Base{ID: uuid.New()}, VideoID: vid.ID, UserID: userID, Title: "Strong", Duration: 30, ViralScore: 0.9, Status: models.ClipStatusReady, Hashtags: "[]"}
	require.NoError(t, db.Create(&clip1).Error)
	require.NoError(t, db.Create(&clip2).Error)

	require.NoError(t, db.Create(&models.ClipAnalytics{Base: models.Base{ID: uuid.New()}, ClipID: clip1.ID, Platform: "tiktok", RecordedAt: time.Now(), Views: 10, Likes: 1, WatchTime: 3}).Error)
	require.NoError(t, db.Create(&models.ClipAnalytics{Base: models.Base{ID: uuid.New()}, ClipID: clip2.ID, Platform: "tiktok", RecordedAt: time.Now(), Views: 5000, Likes: 500, WatchTime: 25}).Error)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/worst-clips", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].([]any)
	require.Len(t, data, 2)
	// First item should be lowest CPS
	assert.Equal(t, clip1.ID.String(), data[0].(map[string]any)["clip_id"].(string))
}

func TestAnalyticsHookPatterns_NoHookData(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New().String()
	app := setupAnalyticsLearningApp(db, userID)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/hook-patterns", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
}

func TestAnalyticsRecommendations_NoProfiles(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New().String()
	app := setupAnalyticsLearningApp(db, userID)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/recommendations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].([]any)
	assert.Empty(t, data)
}

func TestAnalyticsRecommendations_WithProfile(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	userID := uuid.New()
	app := setupAnalyticsLearningApp(db, userID.String())

	// Seed a user + profile in the DB
	u := models.User{Base: models.Base{ID: userID}, Email: "rec@test.com", PasswordHash: "x", Name: "Test", IsActive: true, IsEmailVerified: true}
	require.NoError(t, db.Create(&u).Error)
	p := models.ContentProfile{Base: models.Base{ID: uuid.New()}, UserID: userID, Name: "Gaming Core", Platform: "tiktok", Niche: "gaming"}
	require.NoError(t, db.Create(&p).Error)

	req, _ := http.NewRequest(http.MethodGet, "/analytics/recommendations", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
}

func TestAnalyticsEndpoints_Unauthenticated(t *testing.T) {
	db := setupAnalyticsLearningDB(t)
	app := setupAnalyticsLearningApp(db, "") // no user injected

	for _, path := range []string{
		"/analytics/top-clips",
		"/analytics/worst-clips",
		"/analytics/hook-patterns",
		"/analytics/recommendations",
	} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode, "path: "+path)
	}
}
