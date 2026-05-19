package routes

import (
	"context"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/handlers"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	apiqueue "github.com/pindoyono/viralclip-ai/apps/api/internal/queue"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/server"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/services"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/storage"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/utils"
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

	// Build the queue publisher so VideoHandler can enqueue transcript jobs.
	var queuePublisher apiqueue.VideoPublisher
	if srv.Config.Redis.URL != "" {
		opt, parseErr := redis.ParseURL(srv.Config.Redis.URL)
		if parseErr != nil {
			log.Warn().Err(parseErr).Msg("Invalid Redis URL; queue publishing disabled")
		} else {
			rdb := redis.NewClient(opt)
			queuePublisher = apiqueue.NewPublisher(rdb, srv.Config.Redis.MaxRetries)
		}
	}

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(srv.DB, jwtManager)
	videoHandler := handlers.NewVideoHandler(srv.DB, storageSvc, queuePublisher).WithHub(srv.Hub)
	clipHandler := handlers.NewClipHandler(srv.DB)
	socialHandler := handlers.NewSocialHandler(srv.DB)
	analyticsHandler := handlers.NewAnalyticsHandler(srv.DB)
	trendingHandler := handlers.NewTrendingHandler(srv.DB)
	contentProfileHandler := handlers.NewContentProfileHandler(srv.DB)
	hookHandlerV2 := handlers.NewHookHandlerV2(srv.DB, srv.Config)
	clipHandlerV2 := handlers.NewClipHandlerV2(srv.DB, srv.Config)
	subtitleHandler := handlers.NewSubtitleHandler(srv.DB, srv.Config)
	statusHandler := handlers.NewStatusHandler(srv.DB, srv.Redis, srv.Hub, srv.Config.JWT.Secret)
	metadataHandler := handlers.NewMetadataHandler(srv.DB, srv.Config)
	viralOpportunityService := services.NewViralOpportunityService(srv.DB, services.NewRecommendationEngine())
	viralOpportunityHandler := handlers.NewViralOpportunityHandler(viralOpportunityService)

	// Queue metrics handler (requires Redis).
	var queueHandler *handlers.QueueHandler
	if srv.Redis != nil {
		queueMetricsSvc := services.NewQueueMetricsService(srv.DB, srv.Redis)
		queueHandler = handlers.NewQueueHandler(queueMetricsSvc)
	}

	// Start the Redis Pub/Sub → WebSocket broadcaster in the background.
	if srv.Redis != nil {
		broadcaster := handlers.NewStatusBroadcaster(srv.Redis, srv.Hub, srv.DB)
		go broadcaster.Run(context.Background())
	}

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
	auth.Patch("/me", authHandler.UpdateProfile)
	auth.Patch("/me/password", authHandler.ChangePassword)

	// Video routes
	videos := v1.Group("/videos")
	videos.Post("/", videoHandler.Upload)
	videos.Get("/", videoHandler.List)
	videos.Get("/:id", videoHandler.Get)
	videos.Delete("/:id", videoHandler.Delete)
	videos.Post("/:id/process", videoHandler.ProcessVideo)
	videos.Get("/:id/upload-progress", videoHandler.GetUploadProgress)
	videos.Get("/:videoId/clips", clipHandler.GetByVideo)

	// Hook Detection V2 routes
	videos.Post("/:videoId/hooks/detect", hookHandlerV2.Detect)
	videos.Get("/:videoId/hooks", hookHandlerV2.List)

	// Clip Engine V2 routes
	videos.Post("/:videoId/clips/v2/generate", clipHandlerV2.Generate)

	// Subtitle burning routes
	videos.Post("/:videoId/subtitles/burn", subtitleHandler.BurnSubtitles)

	// Job status route (HTTP polling fallback)
	videos.Get("/:id/job-status", statusHandler.GetJobStatus)

	// WebSocket route (authenticated via ?token= query param)
	// WSUpgrade validates the JWT and sets ws_user_id in Locals before upgrade.
	app.Get("/ws", statusHandler.WSUpgrade, fiberws.New(statusHandler.WSHandler))

	// Clip routes
	clips := v1.Group("/clips")
	clips.Get("/", clipHandler.List)
	clips.Get("/:id", clipHandler.Get)
	clips.Patch("/:id", clipHandler.Update)
	clips.Delete("/:id", clipHandler.Delete)

	// Metadata enhancement route
	clips.Post("/:id/metadata/enhance", metadataHandler.Enhance)

	// Social media routes
	social := v1.Group("/social")
	social.Get("/accounts", socialHandler.ListAccounts)
	social.Post("/accounts", socialHandler.ConnectAccount)
	social.Delete("/accounts/:id", socialHandler.DisconnectAccount)
	social.Post("/connect", socialHandler.Connect)
	social.Post("/disconnect", socialHandler.Disconnect)
	social.Post("/schedule", socialHandler.SchedulePost)
	social.Get("/schedule", socialHandler.ListScheduledPosts)
	social.Delete("/schedule/:id", socialHandler.CancelScheduledPost)
	v1.Post("/schedule", socialHandler.Schedule)
	v1.Post("/publish", socialHandler.Publish)
	v1.Get("/publish/status", socialHandler.PublishStatus)

	// Analytics routes
	analytics := v1.Group("/analytics")
	analytics.Get("/summary", analyticsHandler.Summary)
	analytics.Get("/top-clips", analyticsHandler.TopClips)
	analytics.Get("/worst-clips", analyticsHandler.WorstClips)
	analytics.Get("/hook-patterns", analyticsHandler.HookPatterns)
	analytics.Get("/recommendations", analyticsHandler.LearningRecommendations)

	// Per-clip analytics
	clips.Get("/:id/analytics", analyticsHandler.ClipAnalytics)

	// Trending topics routes
	trending := v1.Group("/trending")
	trending.Get("/", trendingHandler.List)

	// Viral opportunity routes
	viralOpportunities := v1.Group("/viral-opportunities")
	viralOpportunities.Get("/", viralOpportunityHandler.List)
	viralOpportunities.Get("/trending", viralOpportunityHandler.Trending)
	viralOpportunities.Get("/recommendations", viralOpportunityHandler.Recommendations)

	// Content Profile routes
	contentProfiles := v1.Group("/content-profiles")
	contentProfiles.Get("/", contentProfileHandler.List)
	contentProfiles.Post("/", contentProfileHandler.Create)
	contentProfiles.Patch("/:id", contentProfileHandler.Update)
	contentProfiles.Delete("/:id", contentProfileHandler.Delete)

	// Queue monitoring routes (protected — operators/admins only in production).
	if queueHandler != nil {
		queueGroup := v1.Group("/queue")
		queueGroup.Get("/status", queueHandler.Status)
		queueGroup.Get("/failed", queueHandler.Failed)
		queueGroup.Get("/retry", queueHandler.Retry)
	}

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
