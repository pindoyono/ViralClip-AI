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
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// MetadataHandler handles AI-powered clip metadata enhancement.
type MetadataHandler struct {
	db     *gorm.DB
	aiURL  string
	client *http.Client
}

// NewMetadataHandler creates a new MetadataHandler.
func NewMetadataHandler(db *gorm.DB, cfg *config.Config) *MetadataHandler {
	return &MetadataHandler{
		db:    db,
		aiURL: cfg.AI.ServiceURL,
		client: &http.Client{
			Timeout: cfg.AI.Timeout,
		},
	}
}

// aiMetadataRequest mirrors the Python MetadataRequest Pydantic schema.
type aiMetadataRequest struct {
	VideoID    string `json:"video_id"`
	Transcript string `json:"transcript"`
	Platform   string `json:"platform"`
	Niche      string `json:"niche,omitempty"`
	Tone       string `json:"tone,omitempty"`
}

// aiMetadataResponse mirrors the Python MetadataResponse Pydantic schema.
type aiMetadataResponse struct {
	VideoID          string   `json:"video_id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Hashtags         []string `json:"hashtags"`
	Keywords         []string `json:"keywords"`
	Category         string   `json:"category"`
	OptimalPostTimes []string `json:"optimal_post_times"`
}

// Enhance calls the AI metadata service and updates the clip in the database.
//
// POST /api/v1/clips/:id/metadata/enhance
//
// The request body is optional. When provided, platform/niche/tone overrides
// are forwarded to the AI service.  The AI service generates an optimised
// title, description, and hashtag list for the target platform.  The updated
// values are persisted to the clip record and returned to the caller.
func (h *MetadataHandler) Enhance(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	clipID := c.Params("id")

	// Load clip and verify ownership.
	var clip models.Clip
	if err := h.db.Where("id = ? AND user_id = ?", clipID, userID).First(&clip).Error; err != nil {
		return utils.NotFound(c, "Clip not found")
	}

	// Parse optional request body (all fields optional).
	var req dto.EnhanceMetadataRequest
	if err := c.BodyParser(&req); err != nil && len(c.Body()) > 0 {
		return utils.BadRequest(c, "Invalid request body")
	}
	if req.Platform == "" {
		req.Platform = "tiktok"
	}

	// Build a pseudo-transcript from the clip's existing text fields so the
	// AI service has enough context even when the original transcript file is
	// not stored in the database.
	transcript := buildTranscriptContext(clip)

	// Call the AI metadata service.
	aiResp, err := h.callMetadataService(c.Context(), clip.ID.String(), transcript, req)
	if err != nil {
		log.Error().Err(err).Str("clip_id", clipID).Msg("MetadataHandler: AI service error")
		return utils.InternalError(c, "AI metadata service unavailable")
	}

	// Persist enhanced fields to the clip record.
	hashtagsJSON, _ := json.Marshal(aiResp.Hashtags)
	updates := map[string]interface{}{
		"title":       aiResp.Title,
		"description": aiResp.Description,
		"hashtags":    string(hashtagsJSON),
		"updated_at":  time.Now(),
	}
	if err := h.db.Model(&clip).Updates(updates).Error; err != nil {
		log.Error().Err(err).Str("clip_id", clipID).Msg("MetadataHandler: failed to update clip")
		return utils.InternalError(c, "Failed to save enhanced metadata")
	}

	// Reload the clip to return the latest state.
	if err := h.db.First(&clip, "id = ?", clipID).Error; err != nil {
		return utils.InternalError(c, "Failed to reload clip")
	}

	log.Info().
		Str("clip_id", clipID).
		Str("user_id", userID).
		Str("platform", req.Platform).
		Msg("MetadataHandler: clip metadata enhanced")

	return utils.Success(c, dto.MetadataEnhanceResponse{
		Clip:             toClipResponse(clip),
		Keywords:         aiResp.Keywords,
		Category:         aiResp.Category,
		OptimalPostTimes: aiResp.OptimalPostTimes,
	})
}

// buildTranscriptContext assembles a text passage from the clip's existing
// metadata fields so the AI service has enough context to generate meaningful
// metadata without requiring the full video transcript.
func buildTranscriptContext(clip models.Clip) string {
	var buf bytes.Buffer
	if clip.Title != "" {
		fmt.Fprintf(&buf, "Title: %s\n", clip.Title)
	}
	if clip.HookText != "" {
		fmt.Fprintf(&buf, "Hook: %s\n", clip.HookText)
	}
	if clip.Description != "" {
		fmt.Fprintf(&buf, "Description: %s\n", clip.Description)
	}
	if clip.AIRationale != "" {
		fmt.Fprintf(&buf, "AI Rationale: %s\n", clip.AIRationale)
	}
	// Add existing hashtags so the AI can build on them.
	if clip.Hashtags != "" && clip.Hashtags != "[]" {
		var tags []string
		if err := json.Unmarshal([]byte(clip.Hashtags), &tags); err == nil && len(tags) > 0 {
			fmt.Fprintf(&buf, "Existing hashtags: %v\n", tags)
		}
	}
	return buf.String()
}

// callMetadataService sends a POST request to the AI service's /metadata
// endpoint and decodes the response.
func (h *MetadataHandler) callMetadataService(
	ctx context.Context,
	clipID, transcript string,
	req dto.EnhanceMetadataRequest,
) (*aiMetadataResponse, error) {
	payload := aiMetadataRequest{
		VideoID:    clipID,
		Transcript: transcript,
		Platform:   req.Platform,
		Niche:      req.Niche,
		Tone:       req.Tone,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/metadata", h.aiURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("AI service request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var aiResp aiMetadataResponse
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &aiResp, nil
}
