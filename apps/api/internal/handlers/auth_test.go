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
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// setupAuthTestDB creates an in-memory SQLite database with the User schema.
func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))
	return db
}

func setupAuthApp(db *gorm.DB) (*fiber.App, *AuthHandler) {
	app := fiber.New()
	jwtManager := utils.NewJWTManager("test-secret-32-chars-1234567890ab", 15*time.Minute, 7*24*time.Hour, "test")
	h := NewAuthHandler(db, jwtManager)
	app.Post("/register", h.Register)
	app.Post("/login", h.Login)
	app.Post("/refresh", h.RefreshToken)
	return app, h
}

func postJSON(app *fiber.App, path string, body interface{}) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return app.Test(req)
}

// --- Register ---

func TestRegister_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	resp, err := postJSON(app, "/register", map[string]string{
		"name":     "Alice",
		"email":    "alice@example.com",
		"password": "securepass123",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	assert.NotEmpty(t, data["access_token"])
	assert.NotEmpty(t, data["refresh_token"])
}

func TestRegister_DuplicateEmail(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	payload := map[string]string{
		"name":     "Bob",
		"email":    "bob@example.com",
		"password": "pass1234",
	}

	resp1, err := postJSON(app, "/register", payload)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp1.StatusCode)

	resp2, err := postJSON(app, "/register", payload)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusConflict, resp2.StatusCode)
}

func TestRegister_InvalidJSON(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	req, _ := http.NewRequest("POST", "/register", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// --- Login ---

func TestLogin_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	// Seed a user
	hash, _ := utils.HashPassword("mypassword")
	db.Create(&models.User{
		Base:         models.Base{ID: uuid.New()},
		Name:         "Carol",
		Email:        "carol@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	resp, err := postJSON(app, "/login", map[string]string{
		"email":    "carol@example.com",
		"password": "mypassword",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	assert.NotEmpty(t, data["access_token"])
}

func TestLogin_WrongPassword(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	hash, _ := utils.HashPassword("correctpassword")
	db.Create(&models.User{
		Base:         models.Base{ID: uuid.New()},
		Name:         "Dave",
		Email:        "dave@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	resp, err := postJSON(app, "/login", map[string]string{
		"email":    "dave@example.com",
		"password": "wrongpassword",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestLogin_UnknownEmail(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	resp, err := postJSON(app, "/login", map[string]string{
		"email":    "nobody@example.com",
		"password": "somepass",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestLogin_InactiveUser(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	hash, _ := utils.HashPassword("pass123")
	db.Create(&models.User{
		Base:         models.Base{ID: uuid.New()},
		Name:         "Eve",
		Email:        "eve@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     false, // disabled
	})

	resp, err := postJSON(app, "/login", map[string]string{
		"email":    "eve@example.com",
		"password": "pass123",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

// --- RefreshToken ---

func TestRefreshToken_Success(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	// Create user with a known refresh token
	refreshToken := "test-refresh-token-abc123"
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:         models.Base{ID: uuid.New()},
		Name:         "Frank",
		Email:        "frank@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
		RefreshToken: refreshToken,
	})

	resp, err := postJSON(app, "/refresh", map[string]string{
		"refresh_token": refreshToken,
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.NotEmpty(t, data["access_token"])
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	db := setupAuthTestDB(t)
	app, _ := setupAuthApp(db)

	resp, err := postJSON(app, "/refresh", map[string]string{
		"refresh_token": "invalid-or-expired-token",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

// --- UpdateProfile ---

func setupAuthAppWithMe(db *gorm.DB, userID string) (*fiber.App, *AuthHandler) {
	app := fiber.New()
	jwtManager := utils.NewJWTManager("test-secret-32-chars-1234567890ab", 15*time.Minute, 7*24*time.Hour, "test")
	h := NewAuthHandler(db, jwtManager)

	// Simulate JWT middleware: inject a real parsed token so GetUserID works.
	app.Use(func(c *fiber.Ctx) error {
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

	app.Get("/me", h.Me)
	app.Patch("/me", h.UpdateProfile)
	return app, h
}

func patchJSON(app *fiber.App, path string, body interface{}, token ...string) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if len(token) > 0 {
		req.Header.Set("Authorization", "Bearer "+token[0])
	}
	return app.Test(req)
}

func TestUpdateProfile_UpdateName(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:     models.Base{ID: id},
		Name:     "Old Name",
		Email:    "user@example.com",
		PasswordHash: hash,
		Tier:     models.TierFree,
		IsActive: true,
	})

	app, _ := setupAuthAppWithMe(db, id.String())

	newName := "New Name"
	resp, err := patchJSON(app, "/me", map[string]interface{}{
		"name": newName,
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, newName, data["name"])
}

func TestUpdateProfile_UpdateAvatarURL(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:     models.Base{ID: id},
		Name:     "User",
		Email:    "avatar@example.com",
		PasswordHash: hash,
		Tier:     models.TierFree,
		IsActive: true,
	})

	app, _ := setupAuthAppWithMe(db, id.String())

	resp, err := patchJSON(app, "/me", map[string]interface{}{
		"avatar_url": "https://example.com/avatar.jpg",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "https://example.com/avatar.jpg", data["avatar_url"])
}

func TestUpdateProfile_EmptyBodyIsNoOp(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:     models.Base{ID: id},
		Name:     "Unchanged",
		Email:    "noop@example.com",
		PasswordHash: hash,
		Tier:     models.TierFree,
		IsActive: true,
	})

	app, _ := setupAuthAppWithMe(db, id.String())

	resp, err := patchJSON(app, "/me", map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Unchanged", data["name"])
}

func TestUpdateProfile_NotAuthenticated(t *testing.T) {
	db := setupAuthTestDB(t)

	// App with no user_id in Locals (empty string).
	app, _ := setupAuthAppWithMe(db, "")

	resp, err := patchJSON(app, "/me", map[string]interface{}{"name": "X"})
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestMe_ReturnsCurrentUser(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:     models.Base{ID: id},
		Name:     "Me User",
		Email:    "me@example.com",
		PasswordHash: hash,
		Tier:     models.TierPro,
		IsActive: true,
	})

	app, _ := setupAuthAppWithMe(db, id.String())

	req, _ := http.NewRequest("GET", "/me", nil)
	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Me User", data["name"])
	assert.Equal(t, "me@example.com", data["email"])
	assert.Equal(t, "pro", data["tier"])
}

// --- ChangePassword ---

func setupAuthAppWithPassword(db *gorm.DB, userID string) *fiber.App {
	app := fiber.New()
	jwtManager := utils.NewJWTManager("test-secret-32-chars-1234567890ab", 15*time.Minute, 7*24*time.Hour, "test")
	h := NewAuthHandler(db, jwtManager)

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

	app.Patch("/me/password", h.ChangePassword)
	return app
}

func TestChangePassword_Success(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("oldpassword123")
	db.Create(&models.User{
		Base:         models.Base{ID: id},
		Name:         "User",
		Email:        "pw@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	app := setupAuthAppWithPassword(db, id.String())

	resp, err := patchJSON(app, "/me/password", map[string]string{
		"current_password": "oldpassword123",
		"new_password":     "newpassword456",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["success"].(bool))

	// Verify new password was stored.
	var user models.User
	db.First(&user, "id = ?", id)
	assert.True(t, utils.CheckPasswordHash("newpassword456", user.PasswordHash))
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("correctpassword")
	db.Create(&models.User{
		Base:         models.Base{ID: id},
		Name:         "User",
		Email:        "pw2@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	app := setupAuthAppWithPassword(db, id.String())

	resp, err := patchJSON(app, "/me/password", map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpassword456",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnprocessableEntity, resp.StatusCode)
}

func TestChangePassword_TooShortNewPassword(t *testing.T) {
	db := setupAuthTestDB(t)

	id := uuid.New()
	hash, _ := utils.HashPassword("currentpass")
	db.Create(&models.User{
		Base:         models.Base{ID: id},
		Name:         "User",
		Email:        "pw3@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	app := setupAuthAppWithPassword(db, id.String())

	resp, err := patchJSON(app, "/me/password", map[string]string{
		"current_password": "currentpass",
		"new_password":     "short",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestChangePassword_NotAuthenticated(t *testing.T) {
	db := setupAuthTestDB(t)
	app := setupAuthAppWithPassword(db, "")

	resp, err := patchJSON(app, "/me/password", map[string]string{
		"current_password": "any",
		"new_password":     "newpassword456",
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestChangePassword_MissingFields(t *testing.T) {
	db := setupAuthTestDB(t)
	id := uuid.New()
	hash, _ := utils.HashPassword("pass")
	db.Create(&models.User{
		Base:         models.Base{ID: id},
		Name:         "User",
		Email:        "pw4@example.com",
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	})

	app := setupAuthAppWithPassword(db, id.String())

	resp, err := patchJSON(app, "/me/password", map[string]string{
		"current_password": "pass",
		// new_password missing
	})

	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}
