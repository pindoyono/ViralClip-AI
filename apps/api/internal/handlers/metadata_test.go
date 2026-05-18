package handlers

import (
	"bytes"
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

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupMetadataTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Clip{}, &models.Video{}, &models.User{}))
	return db
}

// setupMetadataApp builds a Fiber test app wired to a fake AI server.
func setupMetadataApp(db *gorm.DB, aiServer *httptest.Server) *fiber.App {
	cfg := &config.Config{}
	cfg.AI.ServiceURL = aiServer.URL
	cfg.AI.Timeout = 10 * time.Second

	app := fiber.New()

	// Inject a fake JWT via X-Test-User-ID header (same pattern as other tests).
	app.Use(func(c *fiber.Ctx) error {
		userID := c.Get("X-Test-User-ID")
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
				"email":   "test@example.com",
				"tier":    "free",
				"exp":     time.Now().Add(15 * time.Minute).Unix(),
			})
			signed, _ := tok.SignedString([]byte("test-secret"))
			parsed, _ := jwt.Parse(signed, func(t *jwt.Token) (interface{}, error) {
				return []byte("test-secret"), nil
			})
			c.Locals("user_token", parsed)
		}
		return c.Next()
	})

	h := NewMetadataHandler(db, cfg)
	app.Post("/clips/:id/metadata/enhance", h.Enhance)
	return app
}

// seedClip inserts a ready clip record and returns it.
func seedClipWithDetails(db *gorm.DB, userID uuid.UUID) models.Clip {
	clip := models.Clip{
		Base:        models.Base{ID: uuid.New()},
		VideoID:     uuid.New(),
		UserID:      userID,
		Title:       "Original Title",
		Description: "Original description text.",
		HookText:    "Watch this amazing moment",
		AIRationale: "High viral potential due to emotional peak",
		Hashtags:    `["viral","trending"]`,
		Status:      models.ClipStatusReady,
	}
	db.Create(&clip)
	return clip
}

// fakeMetadataAIServer creates a fake httptest.Server that handles /api/v1/metadata.
func fakeMetadataAIServer(t *testing.T, statusCode int, responseBody interface{}) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if responseBody != nil {
			_ = json.NewEncoder(w).Encode(responseBody)
		}
	})
	return httptest.NewServer(mux)
}

// successfulMetadataResponse returns a typical successful AI response.
func successfulMetadataResponse(clipID string) map[string]interface{} {
	return map[string]interface{}{
		"video_id":    clipID,
		"title":       "Enhanced: Amazing Viral Moment",
		"description": "Discover this incredible moment that will change your perspective.",
		"hashtags":    []string{"viral", "trending", "fyp", "amazing"},
		"keywords":    []string{"viral", "social", "trending"},
		"category":    "Entertainment",
		"optimal_post_times": []string{
			"7:00 PM EST on Weekdays",
			"12:00 PM EST on Weekends",
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEnhanceMetadata_Unauthenticated(t *testing.T) {
	db := setupMetadataTestDB(t)
	srv := fakeMetadataAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	app := setupMetadataApp(db, srv)
	req, _ := http.NewRequest("POST", "/clips/"+uuid.New().String()+"/metadata/enhance", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestEnhanceMetadata_ClipNotFound(t *testing.T) {
	db := setupMetadataTestDB(t)
	srv := fakeMetadataAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	app := setupMetadataApp(db, srv)
	userID := uuid.New()
	req, _ := http.NewRequest("POST", "/clips/"+uuid.New().String()+"/metadata/enhance", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestEnhanceMetadata_ClipOwnedByOtherUser(t *testing.T) {
	db := setupMetadataTestDB(t)
	srv := fakeMetadataAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	owner := uuid.New()
	clip := seedClipWithDetails(db, owner)

	app := setupMetadataApp(db, srv)
	otherUser := uuid.New()
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance", nil)
	req.Header.Set("X-Test-User-ID", otherUser.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestEnhanceMetadata_AIServiceError(t *testing.T) {
	db := setupMetadataTestDB(t)
	srv := fakeMetadataAIServer(t, http.StatusInternalServerError, map[string]string{"detail": "openai error"})
	defer srv.Close()

	userID := uuid.New()
	clip := seedClipWithDetails(db, userID)

	app := setupMetadataApp(db, srv)
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance", nil)
	req.Header.Set("X-Test-User-ID", userID.String())
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestEnhanceMetadata_Success_DefaultPlatform(t *testing.T) {
	db := setupMetadataTestDB(t)

	userID := uuid.New()
	clip := seedClipWithDetails(db, userID)

	// Capture the request payload sent to the AI service.
	var captured aiMetadataRequest
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/metadata", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(successfulMetadataResponse(clip.ID.String()))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	app := setupMetadataApp(db, srv)
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance", nil)
	req.Header.Set("X-Test-User-ID", userID.String())

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Default platform should be "tiktok".
	assert.Equal(t, "tiktok", captured.Platform)

	// Transcript context must include the clip's fields.
	assert.Contains(t, captured.Transcript, "Original Title")
	assert.Contains(t, captured.Transcript, "Watch this amazing moment")
}

func TestEnhanceMetadata_Success_UpdatesClipInDB(t *testing.T) {
	db := setupMetadataTestDB(t)
	userID := uuid.New()
	clip := seedClipWithDetails(db, userID)

	aiResp := successfulMetadataResponse(clip.ID.String())
	srv := fakeMetadataAIServer(t, http.StatusOK, aiResp)
	defer srv.Close()

	app := setupMetadataApp(db, srv)
	payload := map[string]string{"platform": "youtube", "niche": "tech", "tone": "educational"}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", userID.String())

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Reload clip from DB to verify fields were updated.
	var updated models.Clip
	require.NoError(t, db.First(&updated, "id = ?", clip.ID).Error)
	assert.Equal(t, "Enhanced: Amazing Viral Moment", updated.Title)
	assert.Equal(t, "Discover this incredible moment that will change your perspective.", updated.Description)

	var hashtags []string
	_ = json.Unmarshal([]byte(updated.Hashtags), &hashtags)
	assert.Contains(t, hashtags, "fyp")
}

func TestEnhanceMetadata_Success_ResponseShape(t *testing.T) {
	db := setupMetadataTestDB(t)
	userID := uuid.New()
	clip := seedClipWithDetails(db, userID)

	aiResp := successfulMetadataResponse(clip.ID.String())
	srv := fakeMetadataAIServer(t, http.StatusOK, aiResp)
	defer srv.Close()

	app := setupMetadataApp(db, srv)
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance", nil)
	req.Header.Set("X-Test-User-ID", userID.String())

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Clip struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"clip"`
			Keywords         []string `json:"keywords"`
			Category         string   `json:"category"`
			OptimalPostTimes []string `json:"optimal_post_times"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.Success)
	assert.Equal(t, "Enhanced: Amazing Viral Moment", body.Data.Clip.Title)
	assert.Equal(t, "Entertainment", body.Data.Category)
	assert.Contains(t, body.Data.Keywords, "viral")
	assert.NotEmpty(t, body.Data.OptimalPostTimes)
}

func TestEnhanceMetadata_InvalidBody(t *testing.T) {
	db := setupMetadataTestDB(t)
	srv := fakeMetadataAIServer(t, http.StatusOK, nil)
	defer srv.Close()

	userID := uuid.New()
	clip := seedClipWithDetails(db, userID)

	app := setupMetadataApp(db, srv)
	req, _ := http.NewRequest("POST", "/clips/"+clip.ID.String()+"/metadata/enhance",
		bytes.NewBufferString("{not valid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", userID.String())

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBuildTranscriptContext_IncludesAllFields(t *testing.T) {
	clip := models.Clip{
		Title:       "My Title",
		HookText:    "Amazing hook",
		Description: "A great clip",
		AIRationale: "High emotion",
		Hashtags:    `["cool","viral"]`,
	}
	result := buildTranscriptContext(clip)
	assert.Contains(t, result, "My Title")
	assert.Contains(t, result, "Amazing hook")
	assert.Contains(t, result, "A great clip")
	assert.Contains(t, result, "High emotion")
	assert.Contains(t, result, "cool")
}
