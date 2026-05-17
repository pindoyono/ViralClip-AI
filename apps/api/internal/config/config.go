package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the complete application configuration.
type Config struct {
	App      AppConfig
	API      APIConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Log      LogConfig
	CORS     CORSConfig
	Storage  StorageConfig
	AI       AIConfig
	Google   GoogleConfig
	TikTok   TikTokConfig
	Instagram InstagramConfig
	Stripe   StripeConfig
	RateLimit RateLimitConfig
}

type AppConfig struct {
	Name    string
	Version string
	Env     string
	URL     string
}

type APIConfig struct {
	Host         string
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxBodySize  int
}

type DatabaseConfig struct {
	Host            string
	Port            string
	Name            string
	User            string
	Password        string
	SSLMode         string
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host       string
	Port       string
	Password   string
	DB         int
	URL        string
	MaxRetries int
	PoolSize   int
}

type JWTConfig struct {
	Secret            string
	ExpiresIn         time.Duration
	RefreshExpiresIn  time.Duration
	Issuer            string
}

type LogConfig struct {
	Level  string
	Format string
	Output string
}

type CORSConfig struct {
	Origins []string
	MaxAge  int
}

type StorageConfig struct {
	Provider        string
	LocalPath       string
	LocalURL        string
	GCSBucket       string
	GCSProjectID    string
	CredentialsFile string

	// Google Drive storage credentials (used when Provider == "google_drive").
	GoogleDriveClientID     string
	GoogleDriveClientSecret string
	GoogleDriveRefreshToken string
	// GoogleDriveFolderID is the Drive folder used as the root for the
	// ViralClipAI folder tree. Empty means "My Drive" root.
	GoogleDriveFolderID string
}

type AIConfig struct {
	ServiceURL string
	Timeout    time.Duration
	OpenAIKey  string
	OpenAIModel string
}

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

type TikTokConfig struct {
	ClientKey    string
	ClientSecret string
	RedirectURI  string
}

type InstagramConfig struct {
	AppID       string
	AppSecret   string
	RedirectURI string
}

type StripeConfig struct {
	SecretKey      string
	PublishableKey string
	WebhookSecret  string
}

type RateLimitConfig struct {
	Enabled     bool
	MaxRequests int
	Window      time.Duration
}

// Load reads config from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Name:    getEnv("APP_NAME", "ViralClip AI"),
			Version: getEnv("APP_VERSION", "0.1.0"),
			Env:     getEnv("APP_ENV", "development"),
			URL:     getEnv("APP_URL", "http://localhost:3000"),
		},
		API: APIConfig{
			Host:         getEnv("API_HOST", "0.0.0.0"),
			Port:         getEnv("API_PORT", "8080"),
			ReadTimeout:  getDurationEnv("API_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDurationEnv("API_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDurationEnv("API_IDLE_TIMEOUT", 120*time.Second),
			MaxBodySize:  getIntEnv("API_MAX_BODY_SIZE_MB", 500) * 1024 * 1024,
		},
		Database: DatabaseConfig{
			Host:            getEnv("DATABASE_HOST", "localhost"),
			Port:            getEnv("DATABASE_PORT", "5432"),
			Name:            getEnv("DATABASE_NAME", "viralclip"),
			User:            getEnv("DATABASE_USER", "viralclip"),
			Password:        getEnv("DATABASE_PASSWORD", ""),
			SSLMode:         getEnv("DATABASE_SSL_MODE", "disable"),
			URL:             getEnv("DATABASE_URL", ""),
			MaxOpenConns:    getIntEnv("DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("DATABASE_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getDurationEnv("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			Host:       getEnv("REDIS_HOST", "localhost"),
			Port:       getEnv("REDIS_PORT", "6379"),
			Password:   getEnv("REDIS_PASSWORD", ""),
			DB:         getIntEnv("REDIS_DB", 0),
			URL:        getEnv("REDIS_URL", ""),
			MaxRetries: getIntEnv("REDIS_MAX_RETRIES", 3),
			PoolSize:   getIntEnv("REDIS_POOL_SIZE", 10),
		},
		JWT: JWTConfig{
			Secret:           getEnv("JWT_SECRET", ""),
			ExpiresIn:        getDurationEnv("JWT_EXPIRES_IN", 24*time.Hour),
			RefreshExpiresIn: getDurationEnv("JWT_REFRESH_EXPIRES_IN", 168*time.Hour),
			Issuer:           getEnv("JWT_ISSUER", "viralclip-ai"),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
			Output: getEnv("LOG_OUTPUT", "stdout"),
		},
		CORS: CORSConfig{
			Origins: strings.Split(getEnv("CORS_ORIGINS", "http://localhost:3000"), ","),
			MaxAge:  getIntEnv("CORS_MAX_AGE", 86400),
		},
		Storage: StorageConfig{
			Provider:        getEnv("STORAGE_PROVIDER", "local"),
			LocalPath:       getEnv("LOCAL_STORAGE_PATH", "./storage"),
			LocalURL:        getEnv("LOCAL_STORAGE_URL", "http://localhost:8080/storage"),
			GCSBucket:       getEnv("GCS_BUCKET_NAME", ""),
			GCSProjectID:    getEnv("GCS_PROJECT_ID", ""),
			CredentialsFile: getEnv("GOOGLE_APPLICATION_CREDENTIALS", ""),

			GoogleDriveClientID:     getEnv("GOOGLE_DRIVE_CLIENT_ID", ""),
			GoogleDriveClientSecret: getEnv("GOOGLE_DRIVE_CLIENT_SECRET", ""),
			GoogleDriveRefreshToken: getEnv("GOOGLE_DRIVE_REFRESH_TOKEN", ""),
			GoogleDriveFolderID:     getEnv("GOOGLE_DRIVE_FOLDER_ID", ""),
		},
		AI: AIConfig{
			ServiceURL:  getEnv("AI_SERVICE_URL", "http://localhost:8000"),
			Timeout:     getDurationEnv("AI_SERVICE_TIMEOUT", 300*time.Second),
			OpenAIKey:   getEnv("OPENAI_API_KEY", ""),
			OpenAIModel: getEnv("OPENAI_MODEL", "gpt-4-turbo-preview"),
		},
		Google: GoogleConfig{
			ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURI:  getEnv("GOOGLE_REDIRECT_URI", ""),
		},
		TikTok: TikTokConfig{
			ClientKey:    getEnv("TIKTOK_CLIENT_KEY", ""),
			ClientSecret: getEnv("TIKTOK_CLIENT_SECRET", ""),
			RedirectURI:  getEnv("TIKTOK_REDIRECT_URI", ""),
		},
		Instagram: InstagramConfig{
			AppID:       getEnv("INSTAGRAM_APP_ID", ""),
			AppSecret:   getEnv("INSTAGRAM_APP_SECRET", ""),
			RedirectURI: getEnv("INSTAGRAM_REDIRECT_URI", ""),
		},
		Stripe: StripeConfig{
			SecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
			PublishableKey: getEnv("STRIPE_PUBLISHABLE_KEY", ""),
			WebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		},
		RateLimit: RateLimitConfig{
			Enabled:     getBoolEnv("RATE_LIMIT_ENABLED", true),
			MaxRequests: getIntEnv("RATE_LIMIT_MAX_REQUESTS", 100),
			Window:      getDurationEnv("RATE_LIMIT_WINDOW", time.Minute),
		},
	}

	// Build DATABASE_URL if not explicitly set
	if cfg.Database.URL == "" {
		cfg.Database.URL = fmt.Sprintf(
			"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Host,
			cfg.Database.Port,
			cfg.Database.Name,
			cfg.Database.SSLMode,
		)
	}

	// Build REDIS_URL if not explicitly set
	if cfg.Redis.URL == "" {
		if cfg.Redis.Password != "" {
			cfg.Redis.URL = fmt.Sprintf("redis://:%s@%s:%s/%d", cfg.Redis.Password, cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.DB)
		} else {
			cfg.Redis.URL = fmt.Sprintf("redis://%s:%s/%d", cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.DB)
		}
	}

	// Validate required config
	if cfg.App.Env == "production" {
		if cfg.JWT.Secret == "" {
			return nil, fmt.Errorf("JWT_SECRET is required in production")
		}
		if len(cfg.JWT.Secret) < 32 {
			return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters")
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultValue
}
