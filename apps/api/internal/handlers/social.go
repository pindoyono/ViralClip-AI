package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// SocialHandler handles social media account operations.
type SocialHandler struct {
	db *gorm.DB
}

// NewSocialHandler creates a new SocialHandler.
func NewSocialHandler(db *gorm.DB) *SocialHandler {
	return &SocialHandler{db: db}
}

// ListAccounts returns all connected social accounts for the user.
func (h *SocialHandler) ListAccounts(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var accounts []models.SocialAccount
	if err := h.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch social accounts")
		return utils.InternalError(c, "Failed to fetch social accounts")
	}

	responses := make([]dto.SocialAccountResponse, len(accounts))
	for i, a := range accounts {
		responses[i] = dto.SocialAccountResponse{
			ID:             a.ID,
			UserID:         a.UserID,
			Platform:       a.Platform,
			Username:       a.Username,
			DisplayName:    a.DisplayName,
			AvatarURL:      a.AvatarURL,
			IsActive:       a.IsActive,
			FollowersCount: a.FollowersCount,
			ConnectedAt:    a.CreatedAt,
			LastSyncedAt:   a.LastSyncedAt,
		}
	}

	return utils.Success(c, responses)
}

// DisconnectAccount removes a connected social account.
func (h *SocialHandler) DisconnectAccount(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	accountID := c.Params("id")
	var account models.SocialAccount
	if err := h.db.Where("id = ? AND user_id = ?", accountID, userID).First(&account).Error; err != nil {
		return utils.NotFound(c, "Social account not found")
	}

	if err := h.db.Delete(&account).Error; err != nil {
		log.Error().Err(err).Msg("Failed to disconnect social account")
		return utils.InternalError(c, "Failed to disconnect account")
	}

	return utils.SuccessMessage(c, "Account disconnected successfully")
}

// ConnectAccount adds a social media account for the authenticated user.
func (h *SocialHandler) ConnectAccount(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var req dto.ConnectSocialAccountRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}
	if req.Platform == "" || req.Username == "" {
		return utils.BadRequest(c, "platform and username are required")
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return utils.InternalError(c, "Invalid user ID")
	}

	// Prevent duplicate connections for the same platform+username per user.
	var existing models.SocialAccount
	if result := h.db.Where("user_id = ? AND platform = ? AND username = ?", userID, req.Platform, req.Username).First(&existing); result.Error == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":    "already_connected",
				"message": "This account is already connected",
			},
		})
	}

	account := models.SocialAccount{
		Base:           models.Base{ID: uuid.New()},
		UserID:         uid,
		Platform:       models.SocialPlatform(req.Platform),
		PlatformUserID: req.Username, // use username as platform user ID when no OAuth
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		AvatarURL:      req.AvatarURL,
		AccessToken:    req.AccessToken,
		IsActive:       true,
		FollowersCount: req.FollowersCount,
	}

	if err := h.db.Create(&account).Error; err != nil {
		log.Error().Err(err).Msg("Failed to connect social account")
		return utils.InternalError(c, "Failed to connect account")
	}

	return utils.Created(c, dto.SocialAccountResponse{
		ID:             account.ID,
		UserID:         account.UserID,
		Platform:       account.Platform,
		Username:       account.Username,
		DisplayName:    account.DisplayName,
		AvatarURL:      account.AvatarURL,
		IsActive:       account.IsActive,
		FollowersCount: account.FollowersCount,
		ConnectedAt:    account.CreatedAt,
	})
}

// SchedulePost schedules a clip for publishing on a social platform.
func (h *SocialHandler) SchedulePost(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var req dto.CreateScheduledPostRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	// Verify clip belongs to user
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", req.ClipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	if clip.Status != models.ClipStatusReady {
		return utils.BadRequest(c, "Clip is not ready for publishing")
	}

	// Verify social account belongs to user
	var account models.SocialAccount
	if err := h.db.Where("id = ? AND user_id = ? AND is_active = true", req.SocialAccountID, userID).First(&account).Error; err != nil {
		return utils.NotFound(c, "Social account not found or inactive")
	}

	if req.ScheduledAt.Before(time.Now()) {
		return utils.BadRequest(c, "Scheduled time must be in the future")
	}

	post := models.ScheduledPost{
		Base:            models.Base{ID: uuid.New()},
		ClipID:          req.ClipID,
		UserID:          uuid.MustParse(userID),
		SocialAccountID: req.SocialAccountID,
		Platform:        account.Platform,
		ScheduledAt:     req.ScheduledAt,
		Caption:         req.Caption,
		Hashtags:        req.Hashtags,
		Status:          models.PostStatusScheduled,
	}

	if err := h.db.Create(&post).Error; err != nil {
		log.Error().Err(err).Msg("Failed to schedule post")
		return utils.InternalError(c, "Failed to schedule post")
	}

	log.Info().
		Str("post_id", post.ID.String()).
		Str("user_id", userID).
		Str("platform", string(post.Platform)).
		Time("scheduled_at", post.ScheduledAt).
		Msg("Post scheduled")

	return utils.Created(c, dto.ScheduledPostResponse{
		ID:              post.ID,
		ClipID:          post.ClipID,
		UserID:          post.UserID,
		SocialAccountID: post.SocialAccountID,
		Platform:        post.Platform,
		ScheduledAt:     post.ScheduledAt,
		Caption:         post.Caption,
		Hashtags:        post.Hashtags,
		Status:          post.Status,
		CreatedAt:       post.CreatedAt,
	})
}

// ListScheduledPosts returns scheduled posts for the user.
func (h *SocialHandler) ListScheduledPosts(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var pagination dto.PaginationRequest
	if err := c.QueryParser(&pagination); err != nil {
		return utils.BadRequest(c, "Invalid pagination parameters")
	}
	pagination.Normalize()

	var posts []models.ScheduledPost
	var total int64

	query := h.db.Model(&models.ScheduledPost{}).Where("user_id = ?", userID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}

	query.Count(&total)
	if err := query.
		Preload("Clip").
		Order("scheduled_at ASC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit).
		Find(&posts).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch scheduled posts")
		return utils.InternalError(c, "Failed to fetch scheduled posts")
	}

	responses := make([]dto.ScheduledPostResponse, len(posts))
	for i, p := range posts {
		resp := dto.ScheduledPostResponse{
			ID:              p.ID,
			ClipID:          p.ClipID,
			UserID:          p.UserID,
			SocialAccountID: p.SocialAccountID,
			Platform:        p.Platform,
			ScheduledAt:     p.ScheduledAt,
			PublishedAt:     p.PublishedAt,
			Caption:         p.Caption,
			Hashtags:        p.Hashtags,
			PlatformPostURL: p.PlatformPostURL,
			Status:          p.Status,
			ErrorMessage:    p.ErrorMessage,
			CreatedAt:       p.CreatedAt,
		}
		if p.Clip.ID != uuid.Nil {
			clipResp := toClipResponse(p.Clip)
			resp.Clip = &clipResp
		}
		responses[i] = resp
	}

	return utils.Success(c, responses)
}

// CancelScheduledPost cancels a scheduled post.
func (h *SocialHandler) CancelScheduledPost(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	postID := c.Params("id")
	var post models.ScheduledPost
	if err := h.db.Where("id = ? AND user_id = ?", postID, userID).First(&post).Error; err != nil {
		return utils.NotFound(c, "Scheduled post not found")
	}

	if post.Status != models.PostStatusScheduled {
		return utils.BadRequest(c, "Only scheduled posts can be cancelled")
	}

	h.db.Model(&post).Update("status", models.PostStatusCancelled)
	return utils.SuccessMessage(c, "Post cancelled successfully")
}
