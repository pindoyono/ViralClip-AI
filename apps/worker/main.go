package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/pindoyono/viralclip-ai/apps/worker/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/worker/internal/workers"
)

func main() {
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	setupLogger(cfg.Log.Level, cfg.Log.Format)

	log.Info().
		Str("env", cfg.App.Env).
		Int("concurrency", cfg.Worker.Concurrency).
		Msg("Starting ViralClip AI Worker")

	db, err := initDatabase(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}

	rdb, err := initRedis(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	// Initialize workers
	videoWorker := workers.NewVideoProcessingWorker(db, rdb, cfg.AI.ServiceURL, cfg.Worker.MaxRetries)
	publishingWorker := workers.NewPublishingWorker(db, rdb)
	cleanupWorker := workers.NewCleanupWorker(db, rdb)
	analyticsWorker := workers.NewAnalyticsWorker(db, rdb)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	// Video processing loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(cfg.Worker.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				videoWorker.ProcessPendingVideos(ctx)
			}
		}
	}()

	// Publishing loop (every minute)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				publishingWorker.ProcessScheduledPosts(ctx)
			}
		}
	}()

	// Cleanup loop (every hour)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupWorker.CleanupOldData(ctx)
			}
		}
	}()

	// Analytics sync loop (every 15 minutes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				analyticsWorker.SyncAnalytics(ctx)
			}
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down worker...")
	cancel()
	wg.Wait()
	log.Info().Msg("Worker stopped")
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

func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.Database.URL), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	return db, sqlDB.Ping()
}

func initRedis(cfg *config.Config) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = rdb.Ping(ctx).Result()
	return rdb, err
}
