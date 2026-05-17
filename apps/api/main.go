package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/routes"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/server"
)

func main() {
	// Load .env in non-production environments
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Configure logger
	setupLogger(cfg.Log.Level, cfg.Log.Format)

	log.Info().
		Str("version", cfg.App.Version).
		Str("env", cfg.App.Env).
		Msg("Starting ViralClip AI API server")

	// Initialize server with all dependencies
	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize server")
	}

	// Register all routes
	routes.Register(srv)

	// Start server in a goroutine
	go func() {
		addr := cfg.API.Host + ":" + cfg.API.Port
		log.Info().Str("addr", addr).Msg("API server listening")
		if err := srv.App.Listen(addr); err != nil {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Graceful shutdown on interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited cleanly")
}

func setupLogger(level, format string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	if format == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	} else {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	}
}
