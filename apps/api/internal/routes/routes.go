package routes

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/handlers"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/server"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
	"github.com/rs/zerolog/log"
)

// Register sets up all application routes.
func Register(srv *server.Server) {
	app := srv.App

	jwtManager := utils.NewJWTManager(
		srv.Config.JWT.Secret,
		srv.Config.JWT.ExpiresIn,
		srv.Config.JWT.RefreshExpiresIn,
		srv.Config.JWT.Issuer,
	)

	// Build the storage service once and share it with handlers that need it.
	storageSvc, err := storage.NewStorageService(context.Background(), srv.Config)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize storage service")
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(srv.DB, jwtManager)
	videoHandler := handlers.NewVideoHandler(srv.DB, storageSvc)
	clipHandler := handlers.NewClipHandler(srv.DB)
	socialHandler := handlers.NewSocialHandler(srv.DB)
	analyticsHandler := handlers.NewAnalyticsHandler(srv.DB)
	trendingHandler := handlers.NewTrendingHandler(srv.DB)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "viralclip-api",
			"version": srv.Config.App.Version,
		})
	})

	// JWT middleware (applied globally, skips /auth routes)
	app.Use(middleware.Protected(srv.Config.JWT.Secret))

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Authentication routes (public)
	auth := v1.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.RefreshToken)
	auth.Post("/forgot-password", func(c *fiber.Ctx) error {
		return utils.SuccessMessage(c, "If that email exists, a reset link has been sent")
	})
	auth.Post("/reset-password", func(c *fiber.Ctx) error {
		return utils.SuccessMessage(c, "Password reset successfully")
	})

	// Protected routes below
	auth.Post("/logout", authHandler.Logout)
	auth.Get("/me", authHandler.Me)

	// Video routes
	videos := v1.Group("/videos")
	videos.Post("/", videoHandler.Upload)
	videos.Get("/", videoHandler.List)
	videos.Get("/:id", videoHandler.Get)
	videos.Delete("/:id", videoHandler.Delete)
	videos.Post("/:id/process", videoHandler.ProcessVideo)
	videos.Get("/:videoId/clips", clipHandler.GetByVideo)

	// Clip routes
	clips := v1.Group("/clips")
	clips.Get("/", clipHandler.List)
	clips.Get("/:id", clipHandler.Get)
	clips.Patch("/:id", clipHandler.Update)
	clips.Delete("/:id", clipHandler.Delete)

	// Social media routes
	social := v1.Group("/social")
	social.Get("/accounts", socialHandler.ListAccounts)
	social.Delete("/accounts/:id", socialHandler.DisconnectAccount)
	social.Post("/schedule", socialHandler.SchedulePost)
	social.Get("/schedule", socialHandler.ListScheduledPosts)
	social.Delete("/schedule/:id", socialHandler.CancelScheduledPost)

	// Analytics routes
	analytics := v1.Group("/analytics")
	analytics.Get("/summary", analyticsHandler.Summary)

	// Trending topics routes
	trending := v1.Group("/trending")
	trending.Get("/", trendingHandler.List)

	// Catch-all 404
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":    "not_found",
				"message": "The requested resource was not found",
			},
		})
	})
}
