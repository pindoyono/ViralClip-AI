package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

func setupSocialPublishDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.Video{},
		&models.Clip{},
		&models.SocialAccount{},
		&models.ScheduledPost{},
		&models.PublishingLog{},
	))
	return db
}

func setupSocialPublishApp(db *gorm.DB, rdb *redis.Client, userID string) *fiber.App {
	app := fiber.New()
	h := NewSocialHandler(db, rdb)

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

	app.Post("/social/connect", h.Connect)
	app.Post("/social/disconnect", h.Disconnect)
	app.Post("/publish", h.Publish)
	app.Get("/publish/status", h.PublishStatus)
	return app
}

func seedReadyClipAndAccount(t *testing.T, db *gorm.DB, userID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()

	videoID := uuid.New()
	require.NoError(t, db.Create(&models.Video{
		Base:   models.Base{ID: videoID},
		UserID: userID,
		Title:  "video",
		Status: models.VideoStatusCompleted,
	}).Error)

	clipID := uuid.New()
	require.NoError(t, db.Create(&models.Clip{
		Base:      models.Base{ID: clipID},
		VideoID:   videoID,
		UserID:    userID,
		Title:     "clip",
		Status:    models.ClipStatusReady,
		Hashtags:  "[]",
		StartTime: 0,
		EndTime:   10,
		Duration:  10,
	}).Error)

	accountID := uuid.New()
	exp := time.Now().Add(1 * time.Hour)
	require.NoError(t, db.Create(&models.SocialAccount{
		Base:           models.Base{ID: accountID},
		UserID:         userID,
		Platform:       models.PlatformTikTok,
		PlatformUserID: "me",
		Username:       "me",
		AccessToken:    "token-123",
		RefreshToken:   "refresh-123",
		TokenExpiresAt: &exp,
		IsActive:       true,
	}).Error)

	return clipID, accountID
}

func newSocialPublishRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, mr
}

func TestSocialConnectAndDisconnectAlias(t *testing.T) {
	db := setupSocialPublishDB(t)
	userID := uuid.New().String()
	app := setupSocialPublishApp(db, nil, userID)

	connectPayload := map[string]any{
		"platform": "tiktok",
		"username": "alias-user",
	}
	b, _ := json.Marshal(connectPayload)
	req, _ := http.NewRequest(http.MethodPost, "/social/connect", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var connectBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&connectBody))
	accountID := connectBody["data"].(map[string]any)["id"].(string)

	disconnectPayload := map[string]any{"account_id": accountID}
	b2, _ := json.Marshal(disconnectPayload)
	req2, _ := http.NewRequest(http.MethodPost, "/social/disconnect", bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp2.StatusCode)
}

func TestPublishAndPublishStatusEndpoints(t *testing.T) {
	db := setupSocialPublishDB(t)
	userID := uuid.New()
	rdb, mr := newSocialPublishRedis(t)
	defer mr.Close()
	app := setupSocialPublishApp(db, rdb, userID.String())
	clipID, accountID := seedReadyClipAndAccount(t, db, userID)

	publishPayload := map[string]any{
		"clip_id":           clipID.String(),
		"social_account_id": accountID.String(),
		"caption":           "ship it",
		"hashtags":          "#viral",
	}
	b, _ := json.Marshal(publishPayload)
	req, _ := http.NewRequest(http.MethodPost, "/publish", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var publishBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&publishBody))
	postID := publishBody["data"].(map[string]any)["id"].(string)

	require.NoError(t, db.Create(&models.PublishingLog{
		Base:    models.Base{ID: uuid.New()},
		PostID:  uuid.MustParse(postID),
		Status:  models.PostStatusPublishing,
		Message: "publishing started",
	}).Error)
	require.NoError(t, rdb.Set(context.Background(), "upload:progress:"+postID, 67, time.Minute).Err())

	statusReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/publish/status?post_id=%s", postID), nil)
	statusResp, err := app.Test(statusReq)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, statusResp.StatusCode)

	var statusBody map[string]any
	require.NoError(t, json.NewDecoder(statusResp.Body).Decode(&statusBody))
	data := statusBody["data"].(map[string]any)
	assert.Equal(t, postID, data["post"].(map[string]any)["id"])
	assert.Equal(t, float64(67), data["post"].(map[string]any)["upload_progress"])
	assert.Len(t, data["logs"].([]any), 1)
}
