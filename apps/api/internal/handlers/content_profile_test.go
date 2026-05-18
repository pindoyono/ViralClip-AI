package handlers

import (
	"bytes"
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

func setupContentProfileDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ContentProfile{}, &models.User{}))
	return db
}

func setupContentProfileApp(db *gorm.DB, userID string) (*fiber.App, *ContentProfileHandler) {
	app := fiber.New()
	h := NewContentProfileHandler(db)

	app.Use(func(c *fiber.Ctx) error {
		if userID != "" {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"user_id": userID,
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

	app.Get("/content-profiles", h.List)
	app.Post("/content-profiles", h.Create)
	app.Patch("/content-profiles/:id", h.Update)
	app.Delete("/content-profiles/:id", h.Delete)
	return app, h
}

func TestContentProfile_List_Empty(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New().String()
	app, _ := setupContentProfileApp(db, userID)

	req, _ := http.NewRequest("GET", "/content-profiles", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	assert.Empty(t, body["data"])
}

func TestContentProfile_Create_Success(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New().String()
	app, _ := setupContentProfileApp(db, userID)

	payload := map[string]interface{}{
		"name":       "Tech Reviews",
		"platform":   "youtube",
		"niche":      "technology",
		"tone_style": "educational",
	}

	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/content-profiles", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Tech Reviews", data["name"])
	assert.Equal(t, "youtube", data["platform"])
}

func TestContentProfile_Create_MissingName(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New().String()
	app, _ := setupContentProfileApp(db, userID)

	b, _ := json.Marshal(map[string]string{"platform": "tiktok"})
	req, _ := http.NewRequest("POST", "/content-profiles", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestContentProfile_Create_NotAuthenticated(t *testing.T) {
	db := setupContentProfileDB(t)
	app, _ := setupContentProfileApp(db, "")

	b, _ := json.Marshal(map[string]string{"name": "X", "platform": "general"})
	req, _ := http.NewRequest("POST", "/content-profiles", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestContentProfile_List_ReturnsOwned(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New()
	otherUserID := uuid.New()

	// Seed profiles for two different users.
	db.Create(&models.ContentProfile{
		Base:     models.Base{ID: uuid.New()},
		UserID:   userID,
		Name:     "My Profile",
		Platform: "tiktok",
	})
	db.Create(&models.ContentProfile{
		Base:     models.Base{ID: uuid.New()},
		UserID:   otherUserID,
		Name:     "Other Profile",
		Platform: "instagram",
	})

	app, _ := setupContentProfileApp(db, userID.String())

	req, _ := http.NewRequest("GET", "/content-profiles", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].([]interface{})
	assert.Len(t, data, 1)
	assert.Equal(t, "My Profile", data[0].(map[string]interface{})["name"])
}

func TestContentProfile_Update_Success(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New()
	profileID := uuid.New()

	db.Create(&models.ContentProfile{
		Base:     models.Base{ID: profileID},
		UserID:   userID,
		Name:     "Old Name",
		Platform: "general",
	})

	app, _ := setupContentProfileApp(db, userID.String())

	newName := "Updated Name"
	b, _ := json.Marshal(map[string]interface{}{"name": newName})
	req, _ := http.NewRequest("PATCH", "/content-profiles/"+profileID.String(), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, newName, data["name"])
}

func TestContentProfile_Update_NotFound(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New().String()
	app, _ := setupContentProfileApp(db, userID)

	b, _ := json.Marshal(map[string]interface{}{"name": "X"})
	req, _ := http.NewRequest("PATCH", "/content-profiles/"+uuid.New().String(), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestContentProfile_Delete_Success(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New()
	profileID := uuid.New()

	db.Create(&models.ContentProfile{
		Base:     models.Base{ID: profileID},
		UserID:   userID,
		Name:     "To Delete",
		Platform: "general",
	})

	app, _ := setupContentProfileApp(db, userID.String())

	req, _ := http.NewRequest("DELETE", "/content-profiles/"+profileID.String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Verify it's gone.
	var count int64
	db.Model(&models.ContentProfile{}).Where("id = ?", profileID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestContentProfile_Delete_NotFound(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New().String()
	app, _ := setupContentProfileApp(db, userID)

	req, _ := http.NewRequest("DELETE", "/content-profiles/"+uuid.New().String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestContentProfile_DefaultFlag_ClearsPrevious(t *testing.T) {
	db := setupContentProfileDB(t)
	userID := uuid.New()

	// Create an existing default profile.
	db.Create(&models.ContentProfile{
		Base:      models.Base{ID: uuid.New()},
		UserID:    userID,
		Name:      "Old Default",
		Platform:  "general",
		IsDefault: true,
	})

	app, _ := setupContentProfileApp(db, userID.String())

	// Create a new profile with is_default: true.
	b, _ := json.Marshal(map[string]interface{}{
		"name":       "New Default",
		"platform":   "tiktok",
		"is_default": true,
	})
	req, _ := http.NewRequest("POST", "/content-profiles", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// Only one default should exist.
	var defaults int64
	db.Model(&models.ContentProfile{}).Where("user_id = ? AND is_default = true", userID).Count(&defaults)
	assert.Equal(t, int64(1), defaults)
}
