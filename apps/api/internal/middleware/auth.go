package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims defines the claims stored in the JWT token.
type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Tier   string `json:"tier"`
	jwt.RegisteredClaims
}

// Protected returns a JWT authentication middleware.
func Protected(jwtSecret string) fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey: jwtware.SigningKey{
			JWTAlg: jwtware.HS256,
			Key:    []byte(jwtSecret),
		},
		ContextKey: "user_token",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "unauthorized",
					"message": "Invalid or expired token",
				},
			})
		},
		Filter: func(c *fiber.Ctx) bool {
			// Skip JWT check for paths starting with /auth
			return strings.HasPrefix(c.Path(), "/api/v1/auth") ||
				c.Path() == "/health" ||
				c.Path() == "/metrics"
		},
	})
}

// GetUserID extracts the user ID from the JWT claims in the context.
func GetUserID(c *fiber.Ctx) string {
	token, ok := c.Locals("user_token").(*jwt.Token)
	if !ok || token == nil {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	userID, _ := claims["user_id"].(string)
	return userID
}

// GetUserEmail extracts the email from the JWT claims.
func GetUserEmail(c *fiber.Ctx) string {
	token, ok := c.Locals("user_token").(*jwt.Token)
	if !ok || token == nil {
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	email, _ := claims["email"].(string)
	return email
}

// GetUserTier extracts the subscription tier from the JWT claims.
func GetUserTier(c *fiber.Ctx) string {
	token, ok := c.Locals("user_token").(*jwt.Token)
	if !ok || token == nil {
		return "free"
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "free"
	}
	tier, _ := claims["tier"].(string)
	if tier == "" {
		return "free"
	}
	return tier
}

// RequireTier returns a middleware that restricts access to users with the specified tier or higher.
func RequireTier(minTier string) fiber.Handler {
	tierOrder := map[string]int{
		"free":       0,
		"starter":    1,
		"pro":        2,
		"enterprise": 3,
	}

	return func(c *fiber.Ctx) error {
		userTier := GetUserTier(c)
		userTierLevel, ok := tierOrder[userTier]
		if !ok {
			userTierLevel = 0
		}
		minTierLevel, ok := tierOrder[minTier]
		if !ok {
			minTierLevel = 0
		}

		if userTierLevel < minTierLevel {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"success": false,
				"error": fiber.Map{
					"code":    "insufficient_tier",
					"message": "Your subscription tier does not allow this action. Please upgrade.",
					"required_tier": minTier,
				},
			})
		}

		return c.Next()
	}
}
