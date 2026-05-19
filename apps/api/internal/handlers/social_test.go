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

func setupSocialDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SocialAccount{}, &models.User{}))
	return db
}

func setupSocialApp(db *gorm.DB, userID string) (*fiber.App, *SocialHandler) {
	app := fiber.New()
	h := NewSocialHandler(db, nil)

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

	app.Get("/social/accounts", h.ListAccounts)
	app.Post("/social/accounts", h.ConnectAccount)
	app.Delete("/social/accounts/:id", h.DisconnectAccount)
	return app, h
}

func TestConnectAccount_Success(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New().String()
	app, _ := setupSocialApp(db, userID)

	payload := map[string]interface{}{
		"platform":        "tiktok",
		"username":        "myaccount",
		"display_name":    "My Account",
		"followers_count": 1000,
	}

	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "tiktok", data["platform"])
	assert.Equal(t, "myaccount", data["username"])
	assert.Equal(t, true, data["is_active"])
}

func TestConnectAccount_MissingPlatform(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New().String()
	app, _ := setupSocialApp(db, userID)

	b, _ := json.Marshal(map[string]string{"username": "myaccount"})
	req, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestConnectAccount_MissingUsername(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New().String()
	app, _ := setupSocialApp(db, userID)

	b, _ := json.Marshal(map[string]string{"platform": "tiktok"})
	req, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestConnectAccount_NotAuthenticated(t *testing.T) {
	db := setupSocialDB(t)
	app, _ := setupSocialApp(db, "")

	b, _ := json.Marshal(map[string]string{"platform": "tiktok", "username": "me"})
	req, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestConnectAccount_DuplicateConflict(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New()
	app, _ := setupSocialApp(db, userID.String())

	// Seed an existing account.
	db.Create(&models.SocialAccount{
		Base:           models.Base{ID: uuid.New()},
		UserID:         userID,
		Platform:       "tiktok",
		PlatformUserID: "existing",
		Username:       "existing",
		IsActive:       true,
	})

	b, _ := json.Marshal(map[string]string{"platform": "tiktok", "username": "existing"})
	req, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
}

func TestListAccounts_Empty(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New().String()
	app, _ := setupSocialApp(db, userID)

	req, _ := http.NewRequest("GET", "/social/accounts", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body["data"])
}

func TestListAccounts_ReturnsOwned(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New()
	otherID := uuid.New()

	db.Create(&models.SocialAccount{
		Base: models.Base{ID: uuid.New()}, UserID: userID, Platform: "tiktok",
		PlatformUserID: "me", Username: "me", IsActive: true,
	})
	db.Create(&models.SocialAccount{
		Base: models.Base{ID: uuid.New()}, UserID: otherID, Platform: "instagram",
		PlatformUserID: "other", Username: "other", IsActive: true,
	})

	app, _ := setupSocialApp(db, userID.String())
	req, _ := http.NewRequest("GET", "/social/accounts", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].([]interface{})
	assert.Len(t, data, 1)
	assert.Equal(t, "tiktok", data[0].(map[string]interface{})["platform"])
}

func TestDisconnectAccount_Success(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New()
	accountID := uuid.New()

	db.Create(&models.SocialAccount{
		Base:           models.Base{ID: accountID},
		UserID:         userID,
		Platform:       "tiktok",
		PlatformUserID: "me",
		Username:       "me",
		IsActive:       true,
	})

	app, _ := setupSocialApp(db, userID.String())

	req, _ := http.NewRequest("DELETE", "/social/accounts/"+accountID.String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var count int64
	db.Model(&models.SocialAccount{}).Where("id = ?", accountID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestDisconnectAccount_NotFound(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New().String()
	app, _ := setupSocialApp(db, userID)

	req, _ := http.NewRequest("DELETE", "/social/accounts/"+uuid.New().String(), nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestConnectAccount_AllowsDifferentPlatformSameUser(t *testing.T) {
	db := setupSocialDB(t)
	userID := uuid.New()
	app, _ := setupSocialApp(db, userID.String())

	// Connect TikTok
	b1, _ := json.Marshal(map[string]string{"platform": "tiktok", "username": "me"})
	r1, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b1))
	r1.Header.Set("Content-Type", "application/json")
	resp1, _ := app.Test(r1)
	assert.Equal(t, fiber.StatusCreated, resp1.StatusCode)

	// Connect Instagram (different platform, same user — should succeed)
	b2, _ := json.Marshal(map[string]string{"platform": "instagram", "username": "me"})
	r2, _ := http.NewRequest("POST", "/social/accounts", bytes.NewReader(b2))
	r2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(r2)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp2.StatusCode)
}
