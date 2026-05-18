package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

const (
	defaultQueuePageLimit = 20
	maxQueuePageLimit     = 100
)

// QueueHandler exposes queue status, failed job inspection, and retry
// monitoring endpoints.
type QueueHandler struct {
	metrics *services.QueueMetricsService
}

// NewQueueHandler creates a QueueHandler.
func NewQueueHandler(metrics *services.QueueMetricsService) *QueueHandler {
	return &QueueHandler{metrics: metrics}
}

// Status handles GET /queue/status.
// Returns live queue depths (from Redis) and failed-job aggregates (from DB).
func (h *QueueHandler) Status(c *fiber.Ctx) error {
	report, err := h.metrics.Status(c.Context())
	if err != nil {
		log.Error().Err(err).Msg("QueueHandler.Status: failed to collect metrics")
		return utils.InternalError(c, "Failed to collect queue metrics")
	}

	return utils.Success(c, report)
}

// Failed handles GET /queue/failed.
// Returns paginated FailedJob records, optionally filtered by queue or status.
//
// Query params:
//
//	queue  – filter by queue name
//	status – filter by status (pending | recovering | exhausted)
//	limit  – page size (default 20, max 100)
//	offset – page offset
func (h *QueueHandler) Failed(c *fiber.Ctx) error {
	queueName := c.Query("queue")
	status := c.Query("status")
	limit, offset := parseLimitOffset(c)

	jobs, total, err := h.metrics.ListFailedJobs(c.Context(), queueName, status, limit, offset)
	if err != nil {
		log.Error().Err(err).Msg("QueueHandler.Failed: failed to list failed jobs")
		return utils.InternalError(c, "Failed to list failed jobs")
	}

	return utils.Success(c, fiber.Map{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"jobs":   jobs,
	})
}

// Retry handles GET /queue/retry.
// Returns jobs that are eligible for recovery (pending / recovering with
// retry_count < max_retries). The actual retry is performed automatically
// by the RecoveryWorker; this endpoint lets operators inspect the backlog.
//
// Query params:
//
//	limit  – page size (default 20, max 100)
//	offset – page offset
func (h *QueueHandler) Retry(c *fiber.Ctx) error {
	limit, offset := parseLimitOffset(c)

	jobs, total, err := h.metrics.ListRetryableJobs(c.Context(), limit, offset)
	if err != nil {
		log.Error().Err(err).Msg("QueueHandler.Retry: failed to list retryable jobs")
		return utils.InternalError(c, "Failed to list retryable jobs")
	}

	return utils.Success(c, fiber.Map{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"jobs":   jobs,
	})
}

// parseLimitOffset extracts and validates limit/offset query parameters.
func parseLimitOffset(c *fiber.Ctx) (limit, offset int) {
	limit = defaultQueuePageLimit
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > maxQueuePageLimit {
		limit = maxQueuePageLimit
	}

	if o, err := strconv.Atoi(c.Query("offset")); err == nil && o >= 0 {
		offset = o
	}

	return limit, offset
}
