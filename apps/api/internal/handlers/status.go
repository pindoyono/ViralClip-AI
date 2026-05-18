package handlers

import (
	"context"
	"encoding/json"
	"time"

	fiber "github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/websocket"
)

// Redis key prefix for job status (must mirror the worker's queue package).
const jobStatusKeyPrefix = "job:"

// VideoStatusBroadcastChannel is the Redis Pub/Sub channel pattern workers
// publish to after each pipeline stage. The API subscribes to "*" on this
// prefix and fans out to connected WebSocket clients.
const VideoStatusBroadcastChannel = "video:status:"

// StatusHandler provides both the HTTP job-status endpoint and the WebSocket
// upgrade endpoint for real-time pipeline notifications.
type StatusHandler struct {
	db        *gorm.DB
	rdb       *redis.Client
	hub       *websocket.Hub
	jwtSecret string
}

// NewStatusHandler creates a StatusHandler.
func NewStatusHandler(db *gorm.DB, rdb *redis.Client, hub *websocket.Hub, jwtSecret string) *StatusHandler {
	return &StatusHandler{
		db:        db,
		rdb:       rdb,
		hub:       hub,
		jwtSecret: jwtSecret,
	}
}

// ---------------------------------------------------------------------------
// HTTP: GET /api/v1/videos/:id/job-status
// ---------------------------------------------------------------------------

// GetJobStatus returns the current processing stage and status of a video job.
//
// It combines:
//   - video.status from the database
//   - the job:{videoID} key from Redis (set by the worker pipeline)
//
// The derived stages array gives the client a clear picture of which pipeline
// step the video is currently in and which have completed.
func (h *StatusHandler) GetJobStatus(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	videoID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.BadRequest(c, "Invalid video ID")
	}

	var video models.Video
	if err := h.db.Where("id = ? AND user_id = ?", videoID, userID).First(&video).Error; err != nil {
		return utils.NotFound(c, "Video not found")
	}

	// Read the last-known job status from Redis.
	jobStatus := ""
	if h.rdb != nil {
		val, redisErr := h.rdb.Get(c.Context(), jobStatusKeyPrefix+videoID.String()).Result()
		if redisErr == nil {
			jobStatus = val
		}
	}

	resp := buildJobStatusResponse(videoID.String(), string(video.Status), jobStatus)
	return utils.Success(c, resp)
}

// buildJobStatusResponse derives the pipeline stage breakdown from the DB
// video status and the Redis job status string.
func buildJobStatusResponse(videoID, videoStatus, jobStatus string) dto.JobStatusResponse {
	stages := []dto.PipelineStageInfo{
		{Stage: dto.PipelineStageTranscript, Status: dto.StageStatusPending, Label: "Transcription"},
		{Stage: dto.PipelineStageClip, Status: dto.StageStatusPending, Label: "Clip Generation"},
		{Stage: dto.PipelineStageSubtitle, Status: dto.StageStatusPending, Label: "Subtitle Burning"},
		{Stage: dto.PipelineStageUpload, Status: dto.StageStatusPending, Label: "Finalising"},
	}

	currentStage := dto.PipelineStageTranscript

	switch videoStatus {
	case "failed":
		// Mark the active stage as failed; previous stages done.
		stageFromJob := pipelineStageFromJobStatus(jobStatus)
		for i := range stages {
			if stages[i].Stage == stageFromJob {
				stages[i].Status = dto.StageStatusFailed
				currentStage = stageFromJob
				break
			} else {
				stages[i].Status = dto.StageStatusDone
			}
		}
	case "completed":
		for i := range stages {
			stages[i].Status = dto.StageStatusDone
		}
		currentStage = dto.PipelineStageCompleted
	case "processing":
		stageFromJob := pipelineStageFromJobStatus(jobStatus)
		currentStage = stageFromJob
		for i := range stages {
			if stages[i].Stage == stageFromJob {
				stages[i].Status = dto.StageStatusProcessing
				break
			}
			stages[i].Status = dto.StageStatusDone
		}
	default: // pending
		// All stages pending; transcript is the first to run.
	}

	return dto.JobStatusResponse{
		VideoID:      videoID,
		VideoStatus:  videoStatus,
		JobStatus:    jobStatus,
		CurrentStage: currentStage,
		Stages:       stages,
	}
}

// pipelineStageFromJobStatus maps the raw Redis job status string to the
// current pipeline stage. The worker re-uses the same Redis key across all
// stages (it always uses the video ID as the job ID), so we infer the stage
// from the video.status + Redis status combination.
//
// Workers write the stage name into the Redis key as part of the Pub/Sub
// channel payload (see StatusBroadcaster), but the legacy `TrackStatus` call
// only writes `processing` / `done` / `failed`. We therefore default to
// transcript when we can't be more precise.
func pipelineStageFromJobStatus(jobStatus string) dto.PipelineStage {
	// Workers may embed stage info as "stage:transcript:processing" etc.
	// in future. For now we return transcript as default.
	switch jobStatus {
	case "clip:processing", "clip:done":
		return dto.PipelineStageClip
	case "subtitle:processing", "subtitle:done":
		return dto.PipelineStageSubtitle
	case "upload:processing", "upload:done":
		return dto.PipelineStageUpload
	default:
		return dto.PipelineStageTranscript
	}
}

// ---------------------------------------------------------------------------
// WebSocket: GET /ws
// ---------------------------------------------------------------------------

// WSUpgrade is the HTTP handler that upgrades a connection to WebSocket.
// It must be registered as a route *before* the WebSocket handler itself.
// Authentication is via a JWT passed as the `token` query parameter (the
// standard Authorization header is not available during the WS handshake).
func (h *StatusHandler) WSUpgrade(c *fiber.Ctx) error {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "token query parameter required",
		})
	}

	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})
	if err != nil || !parsed.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "invalid or expired token",
		})
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "malformed token claims",
		})
	}

	userID, _ := claims["user_id"].(string)
	if userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"error":   "missing user_id in token",
		})
	}

	// Store authenticated userID for the WebSocket handler.
	c.Locals("ws_user_id", userID)

	if fiberws.IsWebSocketUpgrade(c) {
		return c.Next()
	}

	return fiber.ErrUpgradeRequired
}

// WSHandler is the upgraded WebSocket handler.
// Register it after WSUpgrade using fiberws.New(h.WSHandler).
func (h *StatusHandler) WSHandler(c *fiberws.Conn) {
	userID, _ := c.Locals("ws_user_id").(string)
	if userID == "" {
		_ = c.Close()
		return
	}

	log.Info().Str("user_id", userID).Msg("WebSocket client connected")
	h.hub.HandleConnection(c, userID)
}

// ---------------------------------------------------------------------------
// StatusBroadcaster: Redis Pub/Sub → WebSocket hub fan-out
// ---------------------------------------------------------------------------

// StatusBroadcaster subscribes to Redis Pub/Sub stage-change events published
// by the worker and forwards them to the appropriate WebSocket client.
type StatusBroadcaster struct {
	rdb *redis.Client
	hub *websocket.Hub
	db  *gorm.DB
}

// NewStatusBroadcaster creates a StatusBroadcaster.
func NewStatusBroadcaster(rdb *redis.Client, hub *websocket.Hub, db *gorm.DB) *StatusBroadcaster {
	return &StatusBroadcaster{rdb: rdb, hub: hub, db: db}
}

// Run subscribes to the Redis Pub/Sub channel pattern and fans out messages.
// It exits when ctx is cancelled. Call in a dedicated goroutine.
func (b *StatusBroadcaster) Run(ctx context.Context) {
	if b.rdb == nil {
		log.Warn().Msg("StatusBroadcaster: no Redis client, skipping Pub/Sub subscription")
		return
	}

	pubsub := b.rdb.PSubscribe(ctx, VideoStatusBroadcastChannel+"*")
	defer pubsub.Close()

	log.Info().Msg("StatusBroadcaster: subscribed to video:status:* Pub/Sub channel")

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("StatusBroadcaster: shutting down")
			return

		case msg, ok := <-ch:
			if !ok {
				return
			}
			b.handlePubSubMessage(ctx, msg.Payload)
		}
	}
}

// validPipelineStages is the set of stage names that workers are allowed to
// publish. Events with unknown stage names are silently dropped to avoid
// propagating invalid data to clients.
var validPipelineStages = map[string]dto.PipelineStage{
	"transcript": dto.PipelineStageTranscript,
	"clip":       dto.PipelineStageClip,
	"subtitle":   dto.PipelineStageSubtitle,
	"upload":     dto.PipelineStageUpload,
}

// handlePubSubMessage decodes a stage-change event and sends it to the
// connected WebSocket client that owns the video.
func (b *StatusBroadcaster) handlePubSubMessage(ctx context.Context, payload string) {
	var event StageChangeEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		log.Warn().Err(err).Str("payload", payload).Msg("StatusBroadcaster: failed to decode event")
		return
	}

	// Validate that stage is a known pipeline stage before converting.
	pipelineStage, ok := validPipelineStages[event.Stage]
	if !ok {
		log.Warn().
			Str("stage", event.Stage).
			Str("video_id", event.VideoID).
			Msg("StatusBroadcaster: unknown pipeline stage in event, dropping")
		return
	}

	// Look up the user who owns this video.
	userID := event.UserID
	if userID == "" {
		// Fallback: query DB.
		var v models.Video
		if err := b.db.WithContext(ctx).Select("user_id").Where("id = ?", event.VideoID).First(&v).Error; err != nil {
			log.Warn().Str("video_id", event.VideoID).Msg("StatusBroadcaster: video not found for event")
			return
		}
		userID = v.UserID.String()
	}

	msg := dto.WSMessage{
		Type:    "status_update",
		VideoID: event.VideoID,
		Payload: dto.JobStatusResponse{
			VideoID:      event.VideoID,
			VideoStatus:  event.VideoStatus,
			JobStatus:    event.Stage + ":" + event.Status,
			CurrentStage: pipelineStage,
			Stages:       buildJobStatusResponse(event.VideoID, event.VideoStatus, event.Stage+":"+event.Status).Stages,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("StatusBroadcaster: failed to marshal WS message")
		return
	}

	b.hub.SendToUser(userID, data)

	log.Debug().
		Str("user_id", userID).
		Str("video_id", event.VideoID).
		Str("stage", event.Stage).
		Str("status", event.Status).
		Msg("StatusBroadcaster: forwarded stage-change to WebSocket")
}

// StageChangeEvent is the JSON envelope published to Redis Pub/Sub by workers.
type StageChangeEvent struct {
	VideoID     string    `json:"video_id"`
	UserID      string    `json:"user_id"`
	Stage       string    `json:"stage"`
	Status      string    `json:"status"`
	VideoStatus string    `json:"video_status"`
	Timestamp   time.Time `json:"timestamp"`
}
