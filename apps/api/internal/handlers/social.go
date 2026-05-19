package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// SocialHandler handles social media account operations.
type SocialHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewSocialHandler creates a new SocialHandler.
func NewSocialHandler(db *gorm.DB, rdb *redis.Client) *SocialHandler {
	return &SocialHandler{db: db, redis: rdb}
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
		RefreshToken:   req.RefreshToken,
		TokenExpiresAt: req.ExpiresAt,
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

// Connect handles POST /social/connect alias.
func (h *SocialHandler) Connect(c *fiber.Ctx) error {
	return h.ConnectAccount(c)
}

// Disconnect handles POST /social/disconnect alias.
func (h *SocialHandler) Disconnect(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var req dto.DisconnectSocialAccountRequest
	if err := c.BodyParser(&req); err != nil || req.AccountID == uuid.Nil {
		return utils.BadRequest(c, "account_id is required")
	}

	var account models.SocialAccount
	if err := h.db.Where("id = ? AND user_id = ?", req.AccountID, userID).First(&account).Error; err != nil {
		return utils.NotFound(c, "Social account not found")
	}

	if err := h.db.Delete(&account).Error; err != nil {
		log.Error().Err(err).Msg("Failed to disconnect social account")
		return utils.InternalError(c, "Failed to disconnect account")
	}

	return utils.SuccessMessage(c, "Account disconnected successfully")
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
	return h.createScheduledPostFromRequest(c, userID, req)
}

// Schedule handles POST /schedule alias.
func (h *SocialHandler) Schedule(c *fiber.Ctx) error {
	return h.SchedulePost(c)
}

// Publish handles POST /publish for immediate publishing.
func (h *SocialHandler) Publish(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var req dto.PublishNowRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	now := time.Now().UTC()
	scheduleReq := dto.CreateScheduledPostRequest{
		ClipID:          req.ClipID,
		SocialAccountID: req.SocialAccountID,
		ScheduledAt:     now,
		PublishAt:       &now,
		Caption:         req.Caption,
		Hashtags:        req.Hashtags,
	}
	return h.createScheduledPostFromRequest(c, userID, scheduleReq)
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
			PublishAt:       p.PublishAt,
			PublishedAt:     p.PublishedAt,
			Caption:         p.Caption,
			Hashtags:        p.Hashtags,
			PlatformPostURL: p.PlatformPostURL,
			Status:          p.Status,
			ErrorMessage:    p.ErrorMessage,
			UploadProgress:  h.loadUploadProgress(context.Background(), p.ID.String(), p.UploadProgress),
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

// PublishStatus handles GET /publish/status.
func (h *SocialHandler) PublishStatus(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	postID := c.Query("post_id")
	if postID == "" {
		return utils.BadRequest(c, "post_id is required")
	}

	var post models.ScheduledPost
	if err := h.db.Where("id = ? AND user_id = ?", postID, userID).First(&post).Error; err != nil {
		return utils.NotFound(c, "Scheduled post not found")
	}

	var logs []models.PublishingLog
	if err := h.db.Where("post_id = ?", post.ID).Order("created_at DESC").Find(&logs).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch publishing logs")
		return utils.InternalError(c, "Failed to fetch publish status")
	}

	logResponses := make([]dto.PublishingLogResponse, len(logs))
	for i, l := range logs {
		logResponses[i] = dto.PublishingLogResponse{
			ID:        l.ID,
			PostID:    l.PostID,
			Status:    l.Status,
			Message:   l.Message,
			CreatedAt: l.CreatedAt,
		}
	}

	resp := dto.PublishStatusResponse{
		Post: dto.ScheduledPostResponse{
			ID:              post.ID,
			ClipID:          post.ClipID,
			UserID:          post.UserID,
			SocialAccountID: post.SocialAccountID,
			Platform:        post.Platform,
			ScheduledAt:     post.ScheduledAt,
			PublishAt:       post.PublishAt,
			PublishedAt:     post.PublishedAt,
			Caption:         post.Caption,
			Hashtags:        post.Hashtags,
			PlatformPostURL: post.PlatformPostURL,
			Status:          post.Status,
			ErrorMessage:    post.ErrorMessage,
			UploadProgress:  h.loadUploadProgress(context.Background(), post.ID.String(), post.UploadProgress),
			CreatedAt:       post.CreatedAt,
		},
		Logs: logResponses,
	}

	return utils.Success(c, resp)
}

func (h *SocialHandler) createScheduledPostFromRequest(c *fiber.Ctx, userID string, req dto.CreateScheduledPostRequest) error {
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

	post := models.ScheduledPost{
		Base:            models.Base{ID: uuid.New()},
		ClipID:          req.ClipID,
		UserID:          uuid.MustParse(userID),
		SocialAccountID: req.SocialAccountID,
		Platform:        account.Platform,
		ScheduledAt:     req.ScheduledAt,
		PublishAt:       req.PublishAt,
		Caption:         req.Caption,
		Hashtags:        req.Hashtags,
		Status:          models.PostStatusScheduled,
		UploadProgress:  0,
	}
	if post.PublishAt == nil {
		post.PublishAt = &post.ScheduledAt
	}

	if post.PublishAt != nil && post.PublishAt.Before(time.Now().UTC().Add(-5*time.Second)) {
		return utils.BadRequest(c, "Scheduled time must be in the future")
	}

	if err := h.db.Create(&post).Error; err != nil {
		log.Error().Err(err).Msg("Failed to schedule post")
		return utils.InternalError(c, "Failed to schedule post")
	}

	return utils.Created(c, dto.ScheduledPostResponse{
		ID:              post.ID,
		ClipID:          post.ClipID,
		UserID:          post.UserID,
		SocialAccountID: post.SocialAccountID,
		Platform:        post.Platform,
		ScheduledAt:     post.ScheduledAt,
		PublishAt:       post.PublishAt,
		Caption:         post.Caption,
		Hashtags:        post.Hashtags,
		Status:          post.Status,
		UploadProgress:  post.UploadProgress,
		CreatedAt:       post.CreatedAt,
	})
}

const publishUploadProgressKeyPrefix = "upload:progress:"

func (h *SocialHandler) loadUploadProgress(ctx context.Context, postID string, fallback int) int {
	if h.redis == nil || postID == "" {
		return fallback
	}
	progress, err := h.redis.Get(ctx, publishUploadProgressKeyPrefix+postID).Int()
	if err != nil {
		return fallback
	}
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
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
