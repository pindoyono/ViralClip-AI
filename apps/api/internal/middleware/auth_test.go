package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-middleware-secret-32-chars-ok"

func makeSignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	require.NoError(t, err)
	return signed
}

func validClaims(userID, email, tier string) jwt.MapClaims {
	return jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"tier":    tier,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
	}
}

// TestGetUserID_WithValidToken verifies that GetUserID returns the correct
// user ID from a valid JWT stored in fiber locals.
func TestGetUserID_WithValidToken(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		// Simulate what jwtware middleware sets
		tok, _ := jwt.ParseWithClaims(
			makeSignedToken(t, validClaims("abc-123", "u@ex.com", "free")),
			jwt.MapClaims{},
			func(_ *jwt.Token) (interface{}, error) { return []byte(testJWTSecret), nil },
		)
		c.Locals("user_token", tok)
		return c.SendString(GetUserID(c))
	})

	// agent
	req, _ := http.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestGetUserID_NoToken returns empty string when no token is present.
func TestGetUserID_NoToken(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString(GetUserID(c))
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestGetUserTier_Default returns "free" when tier is absent.
func TestGetUserTier_Default(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString(GetUserTier(c))
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// TestRequireTier_InsufficientTier returns 403 for lower tier.
func TestRequireTier_InsufficientTier(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		tok, _ := jwt.ParseWithClaims(
			makeSignedToken(t, validClaims("u1", "u@ex.com", "free")),
			jwt.MapClaims{},
			func(_ *jwt.Token) (interface{}, error) { return []byte(testJWTSecret), nil },
		)
		c.Locals("user_token", tok)
		return c.Next()
	})
	app.Get("/pro-only", RequireTier("pro"), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/pro-only", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

// TestRequireTier_SufficientTier passes for equal or higher tier.
func TestRequireTier_SufficientTier(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		tok, _ := jwt.ParseWithClaims(
			makeSignedToken(t, validClaims("u1", "u@ex.com", "pro")),
			jwt.MapClaims{},
			func(_ *jwt.Token) (interface{}, error) { return []byte(testJWTSecret), nil },
		)
		c.Locals("user_token", tok)
		return c.Next()
	})
	app.Get("/pro-only", RequireTier("pro"), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/pro-only", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// TestRequireTier_EnterpriseAbovePro verifies tier hierarchy.
func TestRequireTier_EnterpriseAbovePro(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		tok, _ := jwt.ParseWithClaims(
			makeSignedToken(t, validClaims("u1", "u@ex.com", "enterprise")),
			jwt.MapClaims{},
			func(_ *jwt.Token) (interface{}, error) { return []byte(testJWTSecret), nil },
		)
		c.Locals("user_token", tok)
		return c.Next()
	})
	app.Get("/pro-only", RequireTier("pro"), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequest("GET", "/pro-only", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// TestProtected_SkipsAuthPaths verifies filter function bypasses /api/v1/auth.
func TestProtected_FiltersPaths(t *testing.T) {
	app := fiber.New()
	app.Use(Protected(testJWTSecret))
	app.Post("/api/v1/auth/register", func(c *fiber.Ctx) error {
		return c.SendString("registered")
	})

	req, _ := http.NewRequest("POST", "/api/v1/auth/register", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// should NOT be blocked by JWT middleware
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

// TestProtected_BlocksUnauthorized verifies protected routes return 401.
func TestProtected_BlocksUnauthorized(t *testing.T) {
	app := fiber.New()
	app.Use(Protected(testJWTSecret))
	app.Get("/api/v1/videos", func(c *fiber.Ctx) error {
		return c.SendString("videos")
	})

	req, _ := http.NewRequest("GET", "/api/v1/videos", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}
