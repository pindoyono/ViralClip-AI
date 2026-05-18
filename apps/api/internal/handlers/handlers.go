package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// ClipHandler handles clip-related requests.
type ClipHandler struct {
	db *gorm.DB
}

// NewClipHandler creates a new ClipHandler.
func NewClipHandler(db *gorm.DB) *ClipHandler {
	return &ClipHandler{db: db}
}

// List returns paginated clips for the authenticated user.
func (h *ClipHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var pagination dto.PaginationRequest
	if err := c.QueryParser(&pagination); err != nil {
		return utils.BadRequest(c, "Invalid pagination parameters")
	}
	pagination.Normalize()

	var clips []models.Clip
	var total int64

	query := h.db.Model(&models.Clip{}).Where("user_id = ?", userID)

	if videoID := c.Query("video_id"); videoID != "" {
		query = query.Where("video_id = ?", videoID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	if err := query.
		Order("viral_score DESC, created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit).
		Find(&clips).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch clips")
		return utils.InternalError(c, "Failed to fetch clips")
	}

	responses := make([]dto.ClipResponse, len(clips))
	for i, clip := range clips {
		responses[i] = toClipResponse(clip)
	}

	totalPages := int(total) / pagination.Limit
	if int(total)%pagination.Limit != 0 {
		totalPages++
	}

	return utils.Success(c, dto.ClipListResponse{
		Data:       responses,
		Total:      total,
		Page:       pagination.Page,
		Limit:      pagination.Limit,
		TotalPages: totalPages,
	})
}

// Get returns a single clip by ID.
func (h *ClipHandler) Get(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	clipID := c.Params("id")
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", clipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	return utils.Success(c, toClipResponse(clip))
}

// Update updates clip metadata.
func (h *ClipHandler) Update(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	clipID := c.Params("id")
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", clipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	var req dto.UpdateClipRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.HookText != nil {
		updates["hook_text"] = *req.HookText
	}
	if req.Hashtags != nil {
		hashtagsJSON, _ := json.Marshal(req.Hashtags)
		updates["hashtags"] = string(hashtagsJSON)
	}

	if err := h.db.Model(&clip).Updates(updates).Error; err != nil {
		log.Error().Err(err).Msg("Failed to update clip")
		return utils.InternalError(c, "Failed to update clip")
	}

	h.db.First(&clip, "id = ?", clipID)
	return utils.Success(c, toClipResponse(clip))
}

// Delete soft-deletes a clip.
func (h *ClipHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	clipID := c.Params("id")
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", clipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	if err := h.db.Delete(&clip).Error; err != nil {
		log.Error().Err(err).Msg("Failed to delete clip")
		return utils.InternalError(c, "Failed to delete clip")
	}

	return utils.SuccessMessage(c, "Clip deleted successfully")
}

// GetByVideo returns all clips for a specific video.
func (h *ClipHandler) GetByVideo(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID := c.Params("videoId")

	// Verify user owns the video
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	var clips []models.Clip
	if err := h.db.
		Where("video_id = ? AND user_id = ?", videoID, userID).
		Order("viral_score DESC").
		Find(&clips).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch clips for video")
		return utils.InternalError(c, "Failed to fetch clips")
	}

	responses := make([]dto.ClipResponse, len(clips))
	for i, clip := range clips {
		responses[i] = toClipResponse(clip)
	}

	return utils.Success(c, responses)
}

func toClipResponse(clip models.Clip) dto.ClipResponse {
	var hashtags []string
	if clip.Hashtags != "" {
		_ = json.Unmarshal([]byte(clip.Hashtags), &hashtags)
	}

	var suggestedFor []string
	if clip.SuggestedFor != "" {
		_ = json.Unmarshal([]byte(clip.SuggestedFor), &suggestedFor)
	}

	return dto.ClipResponse{
		ID:           clip.ID,
		VideoID:      clip.VideoID,
		UserID:       clip.UserID,
		Title:        clip.Title,
		Description:  clip.Description,
		HookText:     clip.HookText,
		StorageURL:   clip.StorageURL,
		ThumbnailURL: clip.ThumbnailURL,
		StartTime:    clip.StartTime,
		EndTime:      clip.EndTime,
		Duration:     clip.Duration,
		ViralScore:   clip.ViralScore,
		AIRationale:  clip.AIRationale,
		Hashtags:     hashtags,
		SuggestedFor: suggestedFor,
		Status:       clip.Status,
		CreatedAt:    clip.CreatedAt,
	}
}

// AnalyticsHandler handles analytics requests.
type AnalyticsHandler struct {
	db *gorm.DB
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(db *gorm.DB) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

// Summary returns analytics summary for the authenticated user.
func (h *AnalyticsHandler) Summary(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var totalViews, totalLikes, totalComments, totalShares int64

	// Aggregate analytics for user's clips
	h.db.Model(&models.ClipAnalytics{}).
		Joins("JOIN clips ON clips.id = clip_analytics.clip_id").
		Where("clips.user_id = ?", userID).
		Select("SUM(views), SUM(likes), SUM(comments), SUM(shares)").
		Row().
		Scan(&totalViews, &totalLikes, &totalComments, &totalShares)

	var publishedClips, scheduledPosts int64
	h.db.Model(&models.Clip{}).Where("user_id = ? AND status = 'published'", userID).Count(&publishedClips)
	h.db.Model(&models.ScheduledPost{}).Where("user_id = ? AND status = 'scheduled'", userID).Count(&scheduledPosts)

	// Get top clip by viral score
	var topClip models.Clip
	h.db.Where("user_id = ?", userID).Order("viral_score DESC").First(&topClip)

	// Determine top platform by total views
	type platformViews struct {
		Platform string
		Total    int64
	}
	var topPlatformRow platformViews
	h.db.Model(&models.ClipAnalytics{}).
		Joins("JOIN clips ON clips.id = clip_analytics.clip_id").
		Where("clips.user_id = ?", userID).
		Select("clip_analytics.platform AS platform, SUM(clip_analytics.views) AS total").
		Group("clip_analytics.platform").
		Order("total DESC").
		Limit(1).
		Scan(&topPlatformRow)

	summary := dto.AnalyticsSummaryResponse{
		TotalViews:     totalViews,
		TotalLikes:     totalLikes,
		TotalComments:  totalComments,
		TotalShares:    totalShares,
		TopPlatform:    topPlatformRow.Platform,
		PublishedClips: int(publishedClips),
		ClipsPublished: int(publishedClips),
		ScheduledPosts: int(scheduledPosts),
	}

	if topClip.ID != uuid.Nil {
		resp := toClipResponse(topClip)
		summary.TopClip = &resp
	}

	// avg_engagement_rate as a fraction 0–1 (compatible with frontend display)
	if totalViews > 0 {
		summary.AvgEngagement = float64(totalLikes+totalComments+totalShares) / float64(totalViews)
	}

	return utils.Success(c, summary)
}

// ClipAnalytics returns per-platform analytics for a single clip.
func (h *AnalyticsHandler) ClipAnalytics(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	clipID := c.Params("id")

	// Verify the clip belongs to the user
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", clipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	var analytics []models.ClipAnalytics
	if err := h.db.Where("clip_id = ?", clipID).
		Order("recorded_at DESC").
		Find(&analytics).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch clip analytics")
		return utils.InternalError(c, "Failed to fetch clip analytics")
	}

	responses := make([]dto.ClipAnalyticsResponse, len(analytics))
	for i, a := range analytics {
		responses[i] = dto.ClipAnalyticsResponse{
			ID:             a.ID.String(),
			ClipID:         a.ClipID.String(),
			Platform:       string(a.Platform),
			Views:          a.Views,
			Likes:          a.Likes,
			Comments:       a.Comments,
			Shares:         a.Shares,
			Saves:          a.Saves,
			Reach:          a.Reach,
			EngagementRate: a.EngagementRate,
			SyncedAt:       a.RecordedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return utils.Success(c, responses)
}

// TrendingHandler handles trending topic requests.
type TrendingHandler struct {
	db *gorm.DB
}

// NewTrendingHandler creates a new TrendingHandler.
func NewTrendingHandler(db *gorm.DB) *TrendingHandler {
	return &TrendingHandler{db: db}
}

// List returns trending topics with optional platform filter.
func (h *TrendingHandler) List(c *fiber.Ctx) error {
	query := h.db.Model(&models.TrendingTopic{})

	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	var topics []models.TrendingTopic
	if err := query.
		Where("expires_at > NOW()").
		Order("trend_score DESC").
		Limit(50).
		Find(&topics).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch trending topics")
		return utils.InternalError(c, "Failed to fetch trending topics")
	}

	responses := make([]dto.TrendingTopicResponse, len(topics))
	for i, t := range topics {
		responses[i] = dto.TrendingTopicResponse{
			ID:         t.ID,
			Platform:   t.Platform,
			Topic:      t.Topic,
			Hashtag:    t.Hashtag,
			Category:   t.Category,
			TrendScore: t.TrendScore,
			PostCount:  t.PostCount,
			GrowthRate: t.GrowthRate,
			ExpiresAt:  t.ExpiresAt,
		}
	}

	return utils.Success(c, responses)
}
