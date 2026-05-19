package handlers

import (
	"bytes"
	"context"
	"database/sql"
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

// ClipHandlerV2 handles Dynamic Clip Engine V2 requests.
type ClipHandlerV2 struct {
	db       *gorm.DB
	hookRepo repositories.HookRepository
	aiURL    string
	client   *http.Client
}

type aiHistoricalRetentionContext struct {
	SampleSize     int64   `json:"sample_size"`
	AvgRetention   float64 `json:"avg_retention"`
	ShortRetention float64 `json:"short_retention"`
	LongRetention  float64 `json:"long_retention"`
}

// NewClipHandlerV2 creates a new ClipHandlerV2 with dependency injection.
func NewClipHandlerV2(db *gorm.DB, cfg *config.Config) *ClipHandlerV2 {
	return &ClipHandlerV2{
		db:       db,
		hookRepo: repositories.NewHookRepository(db),
		aiURL:    cfg.AI.ServiceURL,
		client: &http.Client{
			Timeout: cfg.AI.Timeout,
		},
	}
}

// Generate generates clip candidates using the V2 Dynamic Clip Engine.
// It fetches existing hook detections for the video from the database and
// forwards them together with the provided transcript segments to the AI
// service.
//
// POST /api/v1/videos/:videoId/clips/v2/generate
func (h *ClipHandlerV2) Generate(c *fiber.Ctx) error {
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

	var req dto.ClipV2GenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.BadRequest(c, "Invalid request body")
	}
	if len(req.Segments) == 0 {
		return utils.BadRequest(c, "segments must not be empty")
	}

	// Apply defaults.
	if req.ProfileType == "" {
		req.ProfileType = "general"
	}
	if req.MinClipScore == 0 {
		req.MinClipScore = 50
	}
	if req.MaxClips == 0 {
		req.MaxClips = 10
	}

	// Fetch stored hook detections for this video.
	fetchCtx, fetchCancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer fetchCancel()

	storedHooks, err := h.hookRepo.FindByVideo(fetchCtx, videoID)
	if err != nil {
		log.Warn().Err(err).Str("video_id", videoID.String()).Msg("Failed to fetch hook detections; continuing without them")
		storedHooks = []models.HookDetection{}
	}

	// Build AI service request.
	type aiSegment struct {
		Text  string  `json:"text"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	type aiHookDetection struct {
		Start          float64 `json:"start"`
		End            float64 `json:"end"`
		Type           string  `json:"type"`
		Score          int     `json:"score"`
		MatchedPattern string  `json:"matched_pattern"`
	}
	type aiRequest struct {
		VideoID           string                        `json:"video_id"`
		Segments          []aiSegment                   `json:"segments"`
		HookDetections    []aiHookDetection             `json:"hook_detections"`
		ProfileType       string                        `json:"profile_type"`
		MinClipScore      int                           `json:"min_clip_score"`
		MaxClips          int                           `json:"max_clips"`
		HistoricalContext *aiHistoricalRetentionContext `json:"historical_context,omitempty"`
	}

	aiSegs := make([]aiSegment, len(req.Segments))
	for i, s := range req.Segments {
		aiSegs[i] = aiSegment{Text: s.Text, Start: s.Start, End: s.End}
	}

	aiHooks := make([]aiHookDetection, len(storedHooks))
	for i, h := range storedHooks {
		aiHooks[i] = aiHookDetection{
			Start:          h.Start,
			End:            h.End,
			Type:           h.HookType,
			Score:          h.Score,
			MatchedPattern: h.MatchedPattern,
		}
	}

	historicalCtx, histErr := h.buildHistoricalRetentionContext(c.Context(), userID)
	if histErr != nil {
		log.Warn().Err(histErr).Str("user_id", userID).Msg("Failed to build historical retention context; continuing without it")
	}

	payload, err := json.Marshal(aiRequest{
		VideoID:           videoID.String(),
		Segments:          aiSegs,
		HookDetections:    aiHooks,
		ProfileType:       req.ProfileType,
		MinClipScore:      req.MinClipScore,
		MaxClips:          req.MaxClips,
		HistoricalContext: historicalCtx,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal ClipV2 AI request")
		return utils.InternalError(c, "Failed to prepare AI request")
	}

	// Call AI service.
	aiURL := fmt.Sprintf("%s/api/v1/clips/v2/generate", h.aiURL)
	callCtx, callCancel := context.WithTimeout(c.Context(), h.client.Timeout)
	defer callCancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, aiURL, bytes.NewReader(payload))
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("Failed to create ClipV2 AI request")
		return utils.InternalError(c, "Failed to contact AI service")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Info().
		Str("video_id", videoID.String()).
		Str("profile", req.ProfileType).
		Int("segments", len(aiSegs)).
		Int("hooks", len(aiHooks)).
		Msg("Calling Clip Engine V2")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("url", aiURL).Msg("ClipV2 AI service request failed")
		return utils.InternalError(c, "AI service unavailable")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read ClipV2 AI response body")
		return utils.InternalError(c, "Failed to read AI response")
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("ClipV2 AI service returned non-200")
		return utils.InternalError(c, "AI service returned an error")
	}

	// Parse response.
	type aiClip struct {
		Start          string  `json:"start"`
		End            string  `json:"end"`
		StartSeconds   float64 `json:"start_seconds"`
		EndSeconds     float64 `json:"end_seconds"`
		Score          int     `json:"score"`
		HookScore      float64 `json:"hook_score"`
		EmotionScore   float64 `json:"emotion_score"`
		StoryScore     float64 `json:"story_score"`
		RetentionScore float64 `json:"retention_score"`
		ProfileType    string  `json:"profile_type"`
	}
	type aiResponse struct {
		VideoID     string   `json:"video_id"`
		ProfileType string   `json:"profile_type"`
		Clips       []aiClip `json:"clips"`
		Total       int      `json:"total"`
	}

	var aiResp aiResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		log.Error().Err(err).Msg("Failed to parse ClipV2 AI response")
		return utils.InternalError(c, "Failed to parse AI response")
	}

	// Convert and return.
	clips := make([]dto.ClipV2ResultItem, len(aiResp.Clips))
	for i, ac := range aiResp.Clips {
		clips[i] = dto.ClipV2ResultItem{
			Start:          ac.Start,
			End:            ac.End,
			StartSeconds:   ac.StartSeconds,
			EndSeconds:     ac.EndSeconds,
			Score:          ac.Score,
			HookScore:      ac.HookScore,
			EmotionScore:   ac.EmotionScore,
			StoryScore:     ac.StoryScore,
			RetentionScore: ac.RetentionScore,
			ProfileType:    ac.ProfileType,
		}
	}

	log.Info().
		Str("video_id", videoID.String()).
		Int("clips", len(clips)).
		Msg("Clip Engine V2 completed")

	return utils.Success(c, dto.ClipV2GenerateResponse{
		VideoID:     videoID.String(),
		ProfileType: aiResp.ProfileType,
		Clips:       clips,
		Total:       len(clips),
	})
}

func (h *ClipHandlerV2) buildHistoricalRetentionContext(ctx context.Context, userID string) (*aiHistoricalRetentionContext, error) {
	type row struct {
		SampleSize     int64
		AvgRetention   sql.NullFloat64
		ShortRetention sql.NullFloat64
		LongRetention  sql.NullFloat64
	}

	var r row
	if err := h.db.WithContext(ctx).
		Table("clip_analytics ca").
		Select(`COUNT(*) AS sample_size,
			AVG(CASE
				WHEN c.duration > 0 THEN
					CASE WHEN (ca.watch_time / c.duration) > 1 THEN 1.0 ELSE (ca.watch_time / c.duration) END
				ELSE NULL
			END) AS avg_retention,
			AVG(CASE
				WHEN c.duration > 0 AND c.duration < 30 THEN
					CASE WHEN (ca.watch_time / c.duration) > 1 THEN 1.0 ELSE (ca.watch_time / c.duration) END
				ELSE NULL
			END) AS short_retention,
			AVG(CASE
				WHEN c.duration > 0 AND c.duration >= 30 THEN
					CASE WHEN (ca.watch_time / c.duration) > 1 THEN 1.0 ELSE (ca.watch_time / c.duration) END
				ELSE NULL
			END) AS long_retention`).
		Joins("JOIN clips c ON c.id = ca.clip_id").
		Where("c.user_id = ? AND c.deleted_at IS NULL", userID).
		Scan(&r).Error; err != nil {
		return nil, err
	}

	if r.SampleSize == 0 || !r.AvgRetention.Valid {
		return nil, nil
	}

	out := &aiHistoricalRetentionContext{
		SampleSize:     r.SampleSize,
		AvgRetention:   r.AvgRetention.Float64,
		ShortRetention: r.AvgRetention.Float64,
		LongRetention:  r.AvgRetention.Float64,
	}
	if r.ShortRetention.Valid {
		out.ShortRetention = r.ShortRetention.Float64
	}
	if r.LongRetention.Valid {
		out.LongRetention = r.LongRetention.Float64
	}
	return out, nil
}
