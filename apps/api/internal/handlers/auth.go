package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
	"gorm.io/gorm"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	db         *gorm.DB
	jwtManager *utils.JWTManager
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(db *gorm.DB, jwtManager *utils.JWTManager) *AuthHandler {
	return &AuthHandler{db: db, jwtManager: jwtManager}
}

// Register handles user registration.
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	// Check if user already exists
	var existing models.User
	if result := h.db.Where("email = ?", req.Email).First(&existing); result.Error == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":    "email_taken",
				"message": "An account with this email already exists",
			},
		})
	}

	// Hash password
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		log.Error().Err(err).Msg("Failed to hash password")
		return utils.InternalError(c, "Failed to create account")
	}

	user := models.User{
		Base:         models.Base{ID: uuid.New()},
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: hash,
		Tier:         models.TierFree,
		IsActive:     true,
	}

	if err := h.db.Create(&user).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create user")
		return utils.InternalError(c, "Failed to create account")
	}

	// Generate tokens
	accessToken, expiresAt, err := h.jwtManager.GenerateAccessToken(user.ID, user.Email, string(user.Tier))
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate access token")
		return utils.InternalError(c, "Failed to generate authentication token")
	}

	refreshToken, err := h.jwtManager.GenerateRefreshToken()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate refresh token")
		return utils.InternalError(c, "Failed to generate refresh token")
	}

	// Store refresh token
	h.db.Model(&user).Updates(map[string]interface{}{
		"refresh_token": refreshToken,
		"last_login_at": time.Now(),
	})

	log.Info().Str("user_id", user.ID.String()).Str("email", user.Email).Msg("User registered")

	return utils.Created(c, dto.AuthResponse{
		User: dto.UserResponse{
			ID:              user.ID,
			Name:            user.Name,
			Email:           user.Email,
			AvatarURL:       user.AvatarURL,
			IsEmailVerified: user.IsEmailVerified,
			Tier:            user.Tier,
			CreatedAt:       user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	})
}

// Login handles user login.
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req dto.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return utils.Unauthorized(c, "Invalid email or password")
	}

	if !user.IsActive {
		return utils.Unauthorized(c, "Account is disabled")
	}

	if !utils.CheckPasswordHash(req.Password, user.PasswordHash) {
		return utils.Unauthorized(c, "Invalid email or password")
	}

	accessToken, expiresAt, err := h.jwtManager.GenerateAccessToken(user.ID, user.Email, string(user.Tier))
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate access token")
		return utils.InternalError(c, "Failed to generate authentication token")
	}

	refreshToken, err := h.jwtManager.GenerateRefreshToken()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate refresh token")
		return utils.InternalError(c, "Failed to generate refresh token")
	}

	now := time.Now()
	h.db.Model(&user).Updates(map[string]interface{}{
		"refresh_token": refreshToken,
		"last_login_at": now,
	})

	log.Info().Str("user_id", user.ID.String()).Msg("User logged in")

	return utils.Success(c, dto.AuthResponse{
		User: dto.UserResponse{
			ID:              user.ID,
			Name:            user.Name,
			Email:           user.Email,
			AvatarURL:       user.AvatarURL,
			IsEmailVerified: user.IsEmailVerified,
			Tier:            user.Tier,
			CreatedAt:       user.CreatedAt,
			LastLoginAt:     &now,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	})
}

// Logout handles user logout by invalidating the refresh token.
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	h.db.Model(&models.User{}).Where("id = ?", userID).Update("refresh_token", "")
	log.Info().Str("user_id", userID).Msg("User logged out")

	return utils.SuccessMessage(c, "Logged out successfully")
}

// RefreshToken issues a new access token using the provided refresh token.
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	var req dto.RefreshTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	var user models.User
	if err := h.db.Where("refresh_token = ? AND is_active = true", req.RefreshToken).First(&user).Error; err != nil {
		return utils.Unauthorized(c, "Invalid or expired refresh token")
	}

	accessToken, expiresAt, err := h.jwtManager.GenerateAccessToken(user.ID, user.Email, string(user.Tier))
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate access token")
		return utils.InternalError(c, "Failed to refresh token")
	}

	return utils.Success(c, fiber.Map{
		"access_token": accessToken,
		"expires_at":   expiresAt,
	})
}

// Me returns the currently authenticated user.
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return utils.NotFound(c, "User not found")
	}

	return utils.Success(c, dto.UserResponse{
		ID:              user.ID,
		Name:            user.Name,
		Email:           user.Email,
		AvatarURL:       user.AvatarURL,
		IsEmailVerified: user.IsEmailVerified,
		Tier:            user.Tier,
		CreatedAt:       user.CreatedAt,
		LastLoginAt:     user.LastLoginAt,
	})
}
