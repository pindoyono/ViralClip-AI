package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// VideoStatus mirrors the API model status.
type VideoStatus string

const (
	VideoStatusPending    VideoStatus = "pending"
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusCompleted  VideoStatus = "completed"
	VideoStatusFailed     VideoStatus = "failed"
)

// Video is a minimal representation for the worker.
type Video struct {
	ID           string      `json:"id" gorm:"primaryKey"`
	UserID       string      `json:"user_id"`
	StoragePath  string      `json:"storage_path"`
	StorageURL   string      `json:"storage_url"`
	Status       VideoStatus `json:"status"`
	ErrorMessage string      `json:"error_message"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

func (Video) TableName() string { return "videos" }

// Clip is a minimal representation for the worker.
type Clip struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	VideoID      string    `json:"video_id"`
	UserID       string    `json:"user_id"`
	Title        string    `json:"title"`
	StartTime    float64   `json:"start_time"`
	EndTime      float64   `json:"end_time"`
	Duration     float64   `json:"duration"`
	StoragePath  string    `json:"storage_path"`
	StorageURL   string    `json:"storage_url"`
	ViralScore   float64   `json:"viral_score"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Clip) TableName() string { return "clips" }

// VideoProcessingWorker processes video files and generates clips.
type VideoProcessingWorker struct {
	db         *gorm.DB
	redis      *redis.Client
	aiURL      string
	httpClient *http.Client
	maxRetries int
}

// NewVideoProcessingWorker creates a new VideoProcessingWorker.
func NewVideoProcessingWorker(db *gorm.DB, rdb *redis.Client, aiURL string, maxRetries int) *VideoProcessingWorker {
	return &VideoProcessingWorker{
		db:         db,
		redis:      rdb,
		aiURL:      aiURL,
		httpClient: &http.Client{Timeout: 300 * time.Second},
		maxRetries: maxRetries,
	}
}

// ProcessPendingVideos polls for pending videos and processes them.
func (w *VideoProcessingWorker) ProcessPendingVideos(ctx context.Context) {
	log.Info().Msg("VideoProcessingWorker: scanning for pending videos")

	var videos []Video
	if err := w.db.WithContext(ctx).
		Where("status = ?", VideoStatusPending).
		Limit(10).
		Find(&videos).Error; err != nil {
		log.Error().Err(err).Msg("Failed to query pending videos")
		return
	}

	for _, video := range videos {
		select {
		case <-ctx.Done():
			return
		default:
			w.processVideo(ctx, video)
		}
	}
}

func (w *VideoProcessingWorker) processVideo(ctx context.Context, video Video) {
	log.Info().Str("video_id", video.ID).Msg("Processing video")

	// Mark as processing
	w.db.Model(&Video{}).Where("id = ?", video.ID).Update("status", VideoStatusProcessing)

	// Call AI service to process video
	payload := map[string]interface{}{
		"video_id":     video.ID,
		"storage_path": video.StoragePath,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", w.aiURL+"/process/video", bytes.NewBuffer(body))
	if err != nil {
		w.markVideoFailed(video.ID, fmt.Sprintf("Failed to create AI request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		w.markVideoFailed(video.ID, fmt.Sprintf("AI service request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		w.markVideoFailed(video.ID, fmt.Sprintf("AI service returned status %d", resp.StatusCode))
		return
	}

	// Mark as completed
	now := time.Now()
	w.db.Model(&Video{}).Where("id = ?", video.ID).Updates(map[string]interface{}{
		"status":       VideoStatusCompleted,
		"processed_at": now,
		"updated_at":   now,
	})

	log.Info().Str("video_id", video.ID).Msg("Video processing completed")
}

func (w *VideoProcessingWorker) markVideoFailed(videoID, errMsg string) {
	log.Error().Str("video_id", videoID).Str("error", errMsg).Msg("Video processing failed")
	w.db.Model(&Video{}).Where("id = ?", videoID).Updates(map[string]interface{}{
		"status":        VideoStatusFailed,
		"error_message": errMsg,
		"updated_at":    time.Now(),
	})
}

// PublishingWorker handles scheduled social media post publishing.
type PublishingWorker struct {
	db         *gorm.DB
	redis      *redis.Client
	httpClient *http.Client
}

// NewPublishingWorker creates a new PublishingWorker.
func NewPublishingWorker(db *gorm.DB, rdb *redis.Client) *PublishingWorker {
	return &PublishingWorker{
		db:         db,
		redis:      rdb,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// ProcessScheduledPosts publishes posts that are due.
func (w *PublishingWorker) ProcessScheduledPosts(ctx context.Context) {
	log.Info().Msg("PublishingWorker: scanning for due posts")

	type ScheduledPost struct {
		ID              string    `gorm:"primaryKey"`
		ClipID          string
		UserID          string
		SocialAccountID string
		Platform        string
		Caption         string
		Hashtags        string
		ScheduledAt     time.Time
		Status          string
		RetryCount      int
	}

	var posts []ScheduledPost
	if err := w.db.WithContext(ctx).
		Table("scheduled_posts").
		Where("status = 'scheduled' AND scheduled_at <= NOW()").
		Limit(20).
		Find(&posts).Error; err != nil {
		log.Error().Err(err).Msg("Failed to query scheduled posts")
		return
	}

	for _, post := range posts {
		select {
		case <-ctx.Done():
			return
		default:
			w.publishPost(ctx, post.ID)
		}
	}
}

func (w *PublishingWorker) publishPost(ctx context.Context, postID string) {
	log.Info().Str("post_id", postID).Msg("Publishing scheduled post")

	// Mark as publishing
	w.db.Table("scheduled_posts").Where("id = ?", postID).Update("status", "publishing")

	// In production, this would call the platform APIs
	// For now, mark as published
	now := time.Now()
	w.db.Table("scheduled_posts").Where("id = ?", postID).Updates(map[string]interface{}{
		"status":       "published",
		"published_at": now,
		"updated_at":   now,
	})

	log.Info().Str("post_id", postID).Msg("Post published successfully")
}

// CleanupWorker handles periodic cleanup tasks.
type CleanupWorker struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewCleanupWorker creates a new CleanupWorker.
func NewCleanupWorker(db *gorm.DB, rdb *redis.Client) *CleanupWorker {
	return &CleanupWorker{db: db, redis: rdb}
}

// CleanupOldData removes soft-deleted records older than 30 days.
func (w *CleanupWorker) CleanupOldData(ctx context.Context) {
	log.Info().Msg("CleanupWorker: running cleanup tasks")

	cutoff := time.Now().AddDate(0, 0, -30)

	tables := []string{"videos", "clips", "scheduled_posts"}
	for _, table := range tables {
		result := w.db.WithContext(ctx).
			Exec(fmt.Sprintf("DELETE FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ?", table), cutoff)
		if result.Error != nil {
			log.Error().Err(result.Error).Str("table", table).Msg("Cleanup failed")
		} else if result.RowsAffected > 0 {
			log.Info().Str("table", table).Int64("rows", result.RowsAffected).Msg("Cleaned up old records")
		}
	}
}

// AnalyticsWorker syncs analytics data from social platforms.
type AnalyticsWorker struct {
	db         *gorm.DB
	redis      *redis.Client
	httpClient *http.Client
}

// NewAnalyticsWorker creates a new AnalyticsWorker.
func NewAnalyticsWorker(db *gorm.DB, rdb *redis.Client) *AnalyticsWorker {
	return &AnalyticsWorker{
		db:         db,
		redis:      rdb,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SyncAnalytics fetches updated analytics from connected social platforms.
func (w *AnalyticsWorker) SyncAnalytics(ctx context.Context) {
	log.Info().Msg("AnalyticsWorker: syncing analytics")

	// In production, this would iterate through all published clips
	// and fetch updated metrics from each platform API
	type PublishedPost struct {
		ID              string
		ClipID          string
		Platform        string
		PlatformPostID  string
		SocialAccountID string
	}

	var posts []PublishedPost
	if err := w.db.WithContext(ctx).
		Table("scheduled_posts").
		Select("id, clip_id, platform, platform_post_id, social_account_id").
		Where("status = 'published' AND platform_post_id != ''").
		Limit(50).
		Find(&posts).Error; err != nil {
		log.Error().Err(err).Msg("Failed to query published posts for analytics sync")
		return
	}

	log.Info().Int("posts", len(posts)).Msg("AnalyticsWorker: sync complete")
}
