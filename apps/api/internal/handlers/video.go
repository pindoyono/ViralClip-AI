package handlers

import (
	"fmt"
	"path/filepath"
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

// VideoHandler handles video-related requests.
type VideoHandler struct {
	db          *gorm.DB
	storagePath string
	storageURL  string
}

// NewVideoHandler creates a new VideoHandler.
func NewVideoHandler(db *gorm.DB, storagePath, storageURL string) *VideoHandler {
	return &VideoHandler{db: db, storagePath: storagePath, storageURL: storageURL}
}

// Upload handles video file upload.
func (h *VideoHandler) Upload(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	title := c.FormValue("title")
	if title == "" {
		return utils.BadRequest(c, "Title is required")
	}

	file, err := c.FormFile("video")
	if err != nil {
		return utils.BadRequest(c, "Video file is required")
	}

	allowedTypes := map[string]bool{
		".mp4":  true,
		".mov":  true,
		".avi":  true,
		".mkv":  true,
		".webm": true,
	}

	ext := filepath.Ext(file.Filename)
	if !allowedTypes[ext] {
		return utils.BadRequest(c, "Unsupported video format. Allowed: mp4, mov, avi, mkv, webm")
	}

	videoID := uuid.New()
	storagePath := filepath.Join(h.storagePath, "videos", userID, videoID.String()+ext)

	if err := c.SaveFile(file, storagePath); err != nil {
		log.Error().Err(err).Msg("Failed to save video file")
		return utils.InternalError(c, "Failed to save video file")
	}

	storageURL := fmt.Sprintf("%s/videos/%s/%s%s", h.storageURL, userID, videoID.String(), ext)

	video := models.Video{
		Base:             models.Base{ID: videoID},
		UserID:           uuid.MustParse(userID),
		Title:            title,
		Description:      c.FormValue("description"),
		OriginalFilename: file.Filename,
		StoragePath:      storagePath,
		StorageURL:       storageURL,
		FileSize:         file.Size,
		MimeType:         file.Header.Get("Content-Type"),
		Status:           models.VideoStatusPending,
	}

	if profileIDStr := c.FormValue("content_profile_id"); profileIDStr != "" {
		if profileID, err := uuid.Parse(profileIDStr); err == nil {
			video.ContentProfileID = &profileID
		}
	}

	if err := h.db.Create(&video).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create video record")
		return utils.InternalError(c, "Failed to create video record")
	}

	log.Info().
		Str("video_id", video.ID.String()).
		Str("user_id", userID).
		Str("title", video.Title).
		Msg("Video uploaded successfully")

	return utils.Created(c, dto.VideoResponse{
		ID:           video.ID,
		UserID:       video.UserID,
		Title:        video.Title,
		Description:  video.Description,
		StorageURL:   video.StorageURL,
		FileSize:     video.FileSize,
		Status:       video.Status,
		CreatedAt:    video.CreatedAt,
	})
}

// List returns a paginated list of videos for the authenticated user.
func (h *VideoHandler) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var pagination dto.PaginationRequest
	if err := c.QueryParser(&pagination); err != nil {
		return utils.BadRequest(c, "Invalid pagination parameters")
	}
	pagination.Normalize()

	var videos []models.Video
	var total int64

	query := h.db.Model(&models.Video{}).Where("user_id = ?", userID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	if err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit).
		Find(&videos).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch videos")
		return utils.InternalError(c, "Failed to fetch videos")
	}

	responses := make([]dto.VideoResponse, len(videos))
	for i, v := range videos {
		var clipsCount int64
		h.db.Model(&models.Clip{}).Where("video_id = ?", v.ID).Count(&clipsCount)

		responses[i] = dto.VideoResponse{
			ID:               v.ID,
			UserID:           v.UserID,
			ContentProfileID: v.ContentProfileID,
			Title:            v.Title,
			Description:      v.Description,
			StorageURL:       v.StorageURL,
			ThumbnailURL:     v.ThumbnailURL,
			Duration:         v.Duration,
			FileSize:         v.FileSize,
			Width:            v.Width,
			Height:           v.Height,
			Status:           v.Status,
			ErrorMessage:     v.ErrorMessage,
			ClipsCount:       int(clipsCount),
			CreatedAt:        v.CreatedAt,
			ProcessedAt:      v.ProcessedAt,
		}
	}

	totalPages := int(total) / pagination.Limit
	if int(total)%pagination.Limit != 0 {
		totalPages++
	}

	return utils.Success(c, dto.VideoListResponse{
		Videos: responses,
		Pagination: dto.PaginationMeta{
			Page:       pagination.Page,
			Limit:      pagination.Limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// Get returns a single video by ID.
func (h *VideoHandler) Get(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID := c.Params("id")
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	var clipsCount int64
	h.db.Model(&models.Clip{}).Where("video_id = ?", video.ID).Count(&clipsCount)

	return utils.Success(c, dto.VideoResponse{
		ID:               video.ID,
		UserID:           video.UserID,
		ContentProfileID: video.ContentProfileID,
		Title:            video.Title,
		Description:      video.Description,
		StorageURL:       video.StorageURL,
		ThumbnailURL:     video.ThumbnailURL,
		Duration:         video.Duration,
		FileSize:         video.FileSize,
		Width:            video.Width,
		Height:           video.Height,
		Status:           video.Status,
		ErrorMessage:     video.ErrorMessage,
		ClipsCount:       int(clipsCount),
		CreatedAt:        video.CreatedAt,
		ProcessedAt:      video.ProcessedAt,
	})
}

// Delete deletes a video and its associated clips.
func (h *VideoHandler) Delete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID := c.Params("id")
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	if video.Status == models.VideoStatusProcessing {
		return utils.BadRequest(c, "Cannot delete a video that is currently being processed")
	}

	// Soft delete all clips first
	h.db.Where("video_id = ?", video.ID).Delete(&models.Clip{})

	// Soft delete the video
	if err := h.db.Delete(&video).Error; err != nil {
		log.Error().Err(err).Msg("Failed to delete video")
		return utils.InternalError(c, "Failed to delete video")
	}

	log.Info().
		Str("video_id", videoID).
		Str("user_id", userID).
		Msg("Video deleted")

	return utils.SuccessMessage(c, "Video deleted successfully")
}

// ProcessVideo triggers video processing via worker queue.
func (h *VideoHandler) ProcessVideo(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID := c.Params("id")
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	if video.Status != models.VideoStatusPending && video.Status != models.VideoStatusFailed {
		return utils.BadRequest(c, "Video is already being processed or completed")
	}

	now := time.Now()
	h.db.Model(&video).Updates(map[string]interface{}{
		"status":     models.VideoStatusProcessing,
		"updated_at": now,
	})

	log.Info().
		Str("video_id", videoID).
		Str("user_id", userID).
		Msg("Video processing triggered")

	return utils.SuccessMessage(c, "Video processing started")
}
