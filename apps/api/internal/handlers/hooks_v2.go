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
	"github.com/pindoyono/viralclip-ai/apps/api/internal/repositories"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// HookHandlerV2 handles Hook Detection Engine V2 requests.
type HookHandlerV2 struct {
	db       *gorm.DB
	hookRepo repositories.HookRepository
	aiURL    string
	client   *http.Client
}

// NewHookHandlerV2 creates a new HookHandlerV2 with dependency injection.
func NewHookHandlerV2(db *gorm.DB, cfg *config.Config) *HookHandlerV2 {
	return &HookHandlerV2{
		db:       db,
		hookRepo: repositories.NewHookRepository(db),
		aiURL:    cfg.AI.ServiceURL,
		client: &http.Client{
			Timeout: cfg.AI.Timeout,
		},
	}
}

// Detect calls the AI service to detect hooks in a transcript and persists
// the results against the video.
//
// POST /api/v1/videos/:videoId/hooks/detect
func (h *HookHandlerV2) Detect(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID, err := uuid.Parse(c.Params("videoId"))
	if err != nil {
		return utils.BadRequest(c, "Invalid video ID")
	}

	// Verify the video belongs to the user.
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	var req dto.HookDetectRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}
	if len(req.Segments) == 0 {
		return utils.BadRequest(c, "segments must not be empty")
	}
	if req.MinScore < 0 || req.MinScore > 100 {
		return utils.BadRequest(c, "min_score must be between 0 and 100")
	}
	// Apply default only when the field was not set by the client (i.e. zero-
	// value AND not explicitly sent).  Since JSON zero values are
	// indistinguishable from absent values with Go's int type, we treat 0 as
	// "not provided" and substitute a sensible default of 50.  Clients that
	// genuinely want all hooks regardless of score should send min_score=1.
	if req.MinScore == 0 {
		req.MinScore = 50
	}

	// Build the payload for the AI service.
	type aiSegment struct {
		Text  string  `json:"text"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	type aiRequest struct {
		VideoID  string      `json:"video_id"`
		Segments []aiSegment `json:"segments"`
		MinScore int         `json:"min_score"`
	}

	aiSegs := make([]aiSegment, len(req.Segments))
	for i, s := range req.Segments {
		aiSegs[i] = aiSegment{Text: s.Text, Start: s.Start, End: s.End}
	}

	payload, err := json.Marshal(aiRequest{
		VideoID:  videoID.String(),
		Segments: aiSegs,
		MinScore: req.MinScore,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal AI request")
		return utils.InternalError(c, "Failed to prepare AI request")
	}

	// Call the AI service.
	ctx, cancel := context.WithTimeout(c.Context(), h.client.Timeout)
	defer cancel()

	aiURL := fmt.Sprintf("%s/api/v1/hooks/v2/detect", h.aiURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, aiURL, bytes.NewReader(payload))
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("Failed to create AI request")
		return utils.InternalError(c, "Failed to contact AI service")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Info().Str("video_id", videoID.String()).Str("url", aiURL).Msg("Calling AI hook detection V2")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("AI service request failed")
		return utils.InternalError(c, "AI service unavailable")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read AI response body")
		return utils.InternalError(c, "Failed to read AI response")
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("AI service returned non-200")
		return utils.InternalError(c, "AI service returned an error")
	}

	// Parse the AI response.
	type aiHook struct {
		Start          float64 `json:"start"`
		End            float64 `json:"end"`
		Type           string  `json:"type"`
		Score          int     `json:"score"`
		MatchedPattern string  `json:"matched_pattern"`
	}
	type aiResponse struct {
		VideoID string   `json:"video_id"`
		Hooks   []aiHook `json:"hooks"`
		Total   int      `json:"total"`
	}

	var aiResp aiResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		log.Error().Err(err).Msg("Failed to parse AI response")
		return utils.InternalError(c, "Failed to parse AI response")
	}

	// Convert to model and persist via repository.
	uid, err := uuid.Parse(userID)
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to parse user ID from context")
		return utils.InternalError(c, "Invalid user session")
	}
	detections := make([]models.HookDetection, len(aiResp.Hooks))
	for i, h := range aiResp.Hooks {
		detections[i] = models.HookDetection{
			VideoID:        videoID,
			UserID:         uid,
			Start:          h.Start,
			End:            h.End,
			HookType:       h.Type,
			Score:          h.Score,
			MatchedPattern: h.MatchedPattern,
		}
	}

	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()

	if err := h.hookRepo.Save(saveCtx, videoID, uid, detections); err != nil {
		log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to save hook detections")
		return utils.InternalError(c, "Failed to save hook detections")
	}

	// Build response.
	results := make([]dto.HookDetectionResultResponse, len(detections))
	for i, d := range detections {
		results[i] = dto.HookDetectionResultResponse{
			Start:          d.Start,
			End:            d.End,
			Type:           d.HookType,
			Score:          d.Score,
			MatchedPattern: d.MatchedPattern,
		}
	}

	log.Info().
		Str("video_id", videoID.String()).
		Int("hooks", len(results)).
		Msg("Hook detection V2 completed")

	return utils.Success(c, dto.HookDetectResponse{
		VideoID: videoID.String(),
		Hooks:   results,
		Total:   len(results),
	})
}

// List returns all stored hook detections for a video.
//
// GET /api/v1/videos/:videoId/hooks
func (h *HookHandlerV2) List(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID, err := uuid.Parse(c.Params("videoId"))
	if err != nil {
		return utils.BadRequest(c, "Invalid video ID")
	}

	// Verify ownership.
	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()

	var detections []models.HookDetection
	if hookType := c.Query("type"); hookType != "" {
		detections, err = h.hookRepo.FindByVideoAndType(ctx, videoID, hookType)
	} else {
		detections, err = h.hookRepo.FindByVideo(ctx, videoID)
	}
	if err != nil {
		log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to fetch hook detections")
		return utils.InternalError(c, "Failed to fetch hook detections")
	}

	results := make([]dto.HookDetectionResultResponse, len(detections))
	for i, d := range detections {
		results[i] = dto.HookDetectionResultResponse{
			Start:          d.Start,
			End:            d.End,
			Type:           d.HookType,
			Score:          d.Score,
			MatchedPattern: d.MatchedPattern,
		}
	}

	return utils.Success(c, dto.HookListResponse{
		VideoID: videoID.String(),
		Hooks:   results,
		Total:   len(results),
	})
}
