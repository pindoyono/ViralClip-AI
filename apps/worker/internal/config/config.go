package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds worker configuration.
type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Worker   WorkerConfig
	AI       AIConfig
	Storage  StorageConfig
	Log      LogConfig
}

type AppConfig struct {
	Env     string
	Version string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	URL        string
	MaxRetries int
	PoolSize   int
}

type WorkerConfig struct {
	Concurrency  int
	PollInterval time.Duration
	MaxRetries   int
}

type AIConfig struct {
	ServiceURL string
	Timeout    time.Duration
}

type StorageConfig struct {
	Provider  string
	LocalPath string
}

type LogConfig struct {
	Level  string
	Format string
}

// Load reads config from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Env:     getEnv("APP_ENV", "development"),
			Version: getEnv("APP_VERSION", "0.1.0"),
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", ""),
			MaxOpenConns:    getIntEnv("DATABASE_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getIntEnv("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDurationEnv("DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL:        getEnv("REDIS_URL", "redis://localhost:6379/0"),
			MaxRetries: getIntEnv("REDIS_MAX_RETRIES", 3),
			PoolSize:   getIntEnv("REDIS_POOL_SIZE", 10),
		},
		Worker: WorkerConfig{
			Concurrency:  getIntEnv("WORKER_CONCURRENCY", 4),
			PollInterval: getDurationEnv("WORKER_POLL_INTERVAL", 5*time.Second),
			MaxRetries:   getIntEnv("WORKER_MAX_RETRIES", 3),
		},
		AI: AIConfig{
			ServiceURL: getEnv("AI_SERVICE_URL", "http://localhost:8000"),
			Timeout:    getDurationEnv("AI_SERVICE_TIMEOUT", 300*time.Second),
		},
		Storage: StorageConfig{
			Provider:  getEnv("STORAGE_PROVIDER", "local"),
			LocalPath: getEnv("LOCAL_STORAGE_PATH", "./storage"),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	if cfg.Database.URL == "" {
		host := getEnv("DATABASE_HOST", "localhost")
		port := getEnv("DATABASE_PORT", "5432")
		name := getEnv("DATABASE_NAME", "viralclip")
		user := getEnv("DATABASE_USER", "viralclip")
		pass := getEnv("DATABASE_PASSWORD", "")
		ssl := getEnv("DATABASE_SSL_MODE", "disable")
		cfg.Database.URL = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
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

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultValue
}
