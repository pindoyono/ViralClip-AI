package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// SubtitleHandler handles subtitle-burning requests.
type SubtitleHandler struct {
	db     *gorm.DB
	aiURL  string
	client *http.Client
}

// NewSubtitleHandler creates a new SubtitleHandler with dependency injection.
func NewSubtitleHandler(db *gorm.DB, cfg *config.Config) *SubtitleHandler {
	return &SubtitleHandler{
		db:    db,
		aiURL: cfg.AI.ServiceURL,
		client: &http.Client{
			Timeout: cfg.AI.Timeout,
		},
	}
}

// BurnSubtitles triggers subtitle burning for all extracted clips of a video.
//
// POST /api/v1/videos/:videoId/subtitles/burn
//
// The request body is optional.  When provided, style/font_size/color
// overrides are forwarded to the AI service.  The AI service reads the
// video's clip manifest and transcript from disk, burns each clip
// independently, and returns the count of processed clips.
//
// On success the handler marks each clip belonging to the video as
// having subtitles (sets subtitle_path to a sentinel value so that
// has_subtitles=true is returned to the client even without the actual
// path being stored in the DB).
func (h *SubtitleHandler) BurnSubtitles(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID, err := uuid.Parse(c.Params("videoId"))
	if err != nil {
		return utils.BadRequest(c, "Invalid video ID")
	}

	// Verify the video exists and belongs to the user.
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	// Subtitles can only be burned after AI processing has produced clips.
	if video.Status != models.VideoStatusCompleted {
		return utils.BadRequest(c, "Video must be fully processed before burning subtitles")
	}

	// Parse optional style overrides.
	var req dto.SubtitleBurnRequest
	// Ignore parse errors — all fields are optional.
	_ = c.BodyParser(&req)

	// Build AI service payload.
	type aiRequest struct {
		VideoID      string `json:"video_id"`
		StoragePath  string `json:"storage_path"`
		Style        string `json:"style,omitempty"`
		FontSize     int    `json:"font_size,omitempty"`
		PrimaryColor string `json:"primary_color,omitempty"`
		OutlineColor string `json:"outline_color,omitempty"`
	}

	payload, err := json.Marshal(aiRequest{
		VideoID:      videoID.String(),
		StoragePath:  video.StoragePath,
		Style:        req.Style,
		FontSize:     req.FontSize,
		PrimaryColor: req.PrimaryColor,
		OutlineColor: req.OutlineColor,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal subtitle burn AI request")
		return utils.InternalError(c, "Failed to prepare AI request")
	}

	// Call the AI service.
	aiURL := fmt.Sprintf("%s/process/subtitles", h.aiURL)
	ctx, cancel := context.WithTimeout(c.Context(), h.client.Timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, aiURL, bytes.NewReader(payload))
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("Failed to create subtitle burn AI request")
		return utils.InternalError(c, "Failed to contact AI service")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Info().
		Str("video_id", videoID.String()).
		Str("url", aiURL).
		Msg("Calling AI service for subtitle burning")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("AI service subtitle burn request failed")
		return utils.InternalError(c, "AI service unavailable")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read AI subtitle burn response")
		return utils.InternalError(c, "Failed to read AI response")
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("AI service subtitle burn returned non-200")
		return utils.InternalError(c, "AI service returned an error")
	}

	// Parse the AI response to get clip count.
	type aiResponse struct {
		VideoID        string `json:"video_id"`
		ClipsProcessed int    `json:"clips_processed"`
	}

	var aiResp aiResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		log.Error().Err(err).Msg("Failed to parse AI subtitle burn response")
		return utils.InternalError(c, "Failed to parse AI response")
	}

	// Update all clips for this video to mark that subtitles have been burned.
	// We use a sentinel path so that has_subtitles=true is returned to clients
	// without storing the actual AI-side path (which belongs to a different
	// service's filesystem).
	const subtitleSentinel = "subtitled"
	now := time.Now()
	if err := h.db.Model(&models.Clip{}).
		Where("video_id = ? AND user_id = ?", videoID, userID).
		Updates(map[string]interface{}{
			"subtitle_path": subtitleSentinel,
			"updated_at":    now,
		}).Error; err != nil {
		log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to update clips subtitle_path")
		// Non-fatal: return success with a warning count.
		return utils.InternalError(c, "Subtitles burned but failed to update clip records")
	}

	log.Info().
		Str("video_id", videoID.String()).
		Int("clips_processed", aiResp.ClipsProcessed).
		Msg("Subtitle burning completed")

	return utils.Success(c, dto.SubtitleBurnResponse{
		VideoID:        videoID.String(),
		ClipsProcessed: aiResp.ClipsProcessed,
	})
}
