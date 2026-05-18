package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/dto"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
)

// ViralOpportunityHandler exposes read APIs for collected trends.
type ViralOpportunityHandler struct {
	service *services.ViralOpportunityService
}

// NewViralOpportunityHandler constructs a handler.
func NewViralOpportunityHandler(service *services.ViralOpportunityService) *ViralOpportunityHandler {
	return &ViralOpportunityHandler{service: service}
}

// List returns paginated viral opportunities.
func (h *ViralOpportunityHandler) List(c *fiber.Ctx) error {
	if middleware.GetUserID(c) == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	var pagination dto.PaginationRequest
	if err := c.QueryParser(&pagination); err != nil {
		return utils.BadRequest(c, "Invalid pagination parameters")
	}
	pagination.Normalize()

	opportunities, total, err := h.service.List(c.Context(), services.ViralOpportunityFilters{
		Page:     pagination.Page,
		Limit:    pagination.Limit,
		Category: c.Query("category"),
		Query:    c.Query("query"),
	})
	if err != nil {
		return utils.InternalError(c, "Failed to fetch viral opportunities")
	}

	responses := make([]dto.ViralOpportunityResponse, len(opportunities))
	for i, opportunity := range opportunities {
		responses[i] = services.ToOpportunityResponse(opportunity)
	}

	totalPages := int(total) / pagination.Limit
	if int(total)%pagination.Limit != 0 {
		totalPages++
	}

	return utils.Success(c, dto.ViralOpportunityListResponse{
		Data:       responses,
		Total:      total,
		Page:       pagination.Page,
		Limit:      pagination.Limit,
		TotalPages: totalPages,
	})
}

// Trending returns the hottest recent opportunities.
func (h *ViralOpportunityHandler) Trending(c *fiber.Ctx) error {
	if middleware.GetUserID(c) == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	opportunities, err := h.service.Trending(c.Context(), 20)
	if err != nil {
		return utils.InternalError(c, "Failed to fetch trending opportunities")
	}

	responses := make([]dto.ViralOpportunityResponse, len(opportunities))
	for i, opportunity := range opportunities {
		responses[i] = services.ToOpportunityResponse(opportunity)
	}

	return utils.Success(c, responses)
}

// Recommendations returns user-specific recommended opportunities.
func (h *ViralOpportunityHandler) Recommendations(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return utils.Unauthorized(c, "Not authenticated")
	}

	results, err := h.service.RecommendForUser(c.Context(), userID, 20)
	if err != nil {
		return utils.InternalError(c, "Failed to generate opportunity recommendations")
	}

	return utils.Success(c, results)
}
