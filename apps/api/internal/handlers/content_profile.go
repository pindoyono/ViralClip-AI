package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// ContentProfileHandler handles content profile CRUD.
type ContentProfileHandler struct {
	db *gorm.DB
}

// NewContentProfileHandler creates a new ContentProfileHandler.
func NewContentProfileHandler(db *gorm.DB) *ContentProfileHandler {
	return &ContentProfileHandler{db: db}
}

// List returns all content profiles for the authenticated user.
func (h *ContentProfileHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var profiles []models.ContentProfile
	if err := h.db.Where("user_id = ?", userID).
		Order("is_default DESC, created_at ASC").
		Find(&profiles).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch content profiles")
		return utils.InternalError(c, "Failed to fetch content profiles")
	}

	responses := make([]dto.ContentProfileResponse, len(profiles))
	for i, p := range profiles {
		responses[i] = toContentProfileResponse(p)
	}
	return utils.Success(c, responses)
}

// Create creates a new content profile.
func (h *ContentProfileHandler) Create(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var req dto.CreateContentProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}
	if req.Name == "" {
		return utils.BadRequest(c, "Name is required")
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return utils.InternalError(c, "Invalid user ID")
	}

	// If this profile is marked as default, clear the old default.
	if req.IsDefault {
		h.db.Model(&models.ContentProfile{}).
			Where("user_id = ? AND is_default = true", userID).
			Update("is_default", false)
	}

	profile := models.ContentProfile{
		Base:        models.Base{ID: uuid.New()},
		UserID:      uid,
		Name:        req.Name,
		Platform:    req.Platform,
		Niche:       req.Niche,
		ToneStyle:   req.ToneStyle,
		AudienceAge: req.AudienceAge,
		Keywords:    req.Keywords,
		IsDefault:   req.IsDefault,
	}

	if err := h.db.Create(&profile).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create content profile")
		return utils.InternalError(c, "Failed to create content profile")
	}

	return utils.Created(c, toContentProfileResponse(profile))
}

// Update modifies an existing content profile.
func (h *ContentProfileHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	profileID := c.Params("id")
	var profile models.ContentProfile
	if err := h.db.Where("id = ? AND user_id = ?", profileID, userID).First(&profile).Error; err != nil {
		return utils.NotFound(c, "Content profile not found")
	}

	var req dto.UpdateContentProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Niche != nil {
		updates["niche"] = *req.Niche
	}
	if req.ToneStyle != nil {
		updates["tone_style"] = *req.ToneStyle
	}
	if req.AudienceAge != nil {
		updates["audience_age"] = *req.AudienceAge
	}
	if req.Keywords != nil {
		updates["keywords"] = *req.Keywords
	}
	if req.IsDefault != nil && *req.IsDefault {
		// Clear previous default before setting new one.
		h.db.Model(&models.ContentProfile{}).
			Where("user_id = ? AND is_default = true AND id != ?", userID, profileID).
			Update("is_default", false)
		updates["is_default"] = true
	}

	if len(updates) > 0 {
		if err := h.db.Model(&profile).Updates(updates).Error; err != nil {
			log.Error().Err(err).Msg("Failed to update content profile")
			return utils.InternalError(c, "Failed to update content profile")
		}
	}

	h.db.First(&profile, "id = ?", profileID)
	return utils.Success(c, toContentProfileResponse(profile))
}

// Delete removes a content profile.
func (h *ContentProfileHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	profileID := c.Params("id")
	var profile models.ContentProfile
	if err := h.db.Where("id = ? AND user_id = ?", profileID, userID).First(&profile).Error; err != nil {
		return utils.NotFound(c, "Content profile not found")
	}

	if err := h.db.Delete(&profile).Error; err != nil {
		log.Error().Err(err).Msg("Failed to delete content profile")
		return utils.InternalError(c, "Failed to delete content profile")
	}

	return utils.SuccessMessage(c, "Content profile deleted successfully")
}

func toContentProfileResponse(p models.ContentProfile) dto.ContentProfileResponse {
	return dto.ContentProfileResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		Name:        p.Name,
		Platform:    p.Platform,
		Niche:       p.Niche,
		ToneStyle:   p.ToneStyle,
		AudienceAge: p.AudienceAge,
		Keywords:    p.Keywords,
		IsDefault:   p.IsDefault,
		CreatedAt:   p.CreatedAt,
	}
}
