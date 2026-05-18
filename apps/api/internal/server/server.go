package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/middleware"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// Server holds all application dependencies.
type Server struct {
	App    *fiber.App
	DB     *gorm.DB
	Redis  *redis.Client
	Config *config.Config
}

// New creates and initializes the server with all dependencies.
func New(cfg *config.Config) (*Server, error) {
	db, err := initDatabase(cfg)
	if err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	rdb, err := initRedis(cfg)
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	app := fiber.New(fiber.Config{
		AppName:           cfg.App.Name,
		ServerHeader:      "ViralClip-AI",
		ReadTimeout:       cfg.API.ReadTimeout,
		WriteTimeout:      cfg.API.WriteTimeout,
		IdleTimeout:       cfg.API.IdleTimeout,
		BodyLimit:         cfg.API.MaxBodySize,
		EnablePrintRoutes: cfg.App.Env == "development",
		ErrorHandler:      middleware.ErrorHandler,
	})

	srv := &Server{
		App:    app,
		DB:     db,
		Redis:  rdb,
		Config: cfg,
	}

	srv.registerGlobalMiddleware()

	return srv, nil
}

func (s *Server) registerGlobalMiddleware() {
	s.App.Use(recover.New(recover.Config{
		EnableStackTrace: s.Config.App.Env != "production",
	}))

	s.App.Use(requestid.New())

	s.App.Use(middleware.ZerologMiddleware())

	s.App.Use(helmet.New())

	s.App.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(s.Config.CORS.Origins, ","),
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID",
		AllowCredentials: true,
		MaxAge:           s.Config.CORS.MaxAge,
	}))

	s.App.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	if s.Config.RateLimit.Enabled {
		s.App.Use(limiter.New(limiter.Config{
			Max:        s.Config.RateLimit.MaxRequests,
			Expiration: s.Config.RateLimit.Window,
			KeyGenerator: func(c *fiber.Ctx) string {
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please try again later.",
				})
			},
		}))
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.App.ShutdownWithContext(ctx)
}

func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Silent
	if cfg.App.Env == "development" {
		logLevel = logger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.Database.URL), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Auto-migrate models
	if err := db.AutoMigrate(
		&models.User{},
		&models.ContentProfile{},
		&models.Video{},
		&models.Clip{},
		&models.SocialAccount{},
		&models.ScheduledPost{},
		&models.ClipAnalytics{},
		&models.TrendingTopic{},
		&models.HookDetection{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrating models: %w", err)
	}

	log.Info().Msg("Database connected and migrated")
	return db, nil
}

func initRedis(cfg *config.Config) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		// Fallback to manual config
		opt = &redis.Options{
			Addr:       fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
			Password:   cfg.Redis.Password,
			DB:         cfg.Redis.DB,
			MaxRetries: cfg.Redis.MaxRetries,
			PoolSize:   cfg.Redis.PoolSize,
		}
	}

	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := rdb.Ping(ctx).Result(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	log.Info().Msg("Redis connected")
	return rdb, nil
}
