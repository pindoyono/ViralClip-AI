package workers

import (
	"bytes"
	"context"
	"crypto/rand"
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
	HookText     string    `json:"hook_text"`
	AIRationale  string    `json:"ai_rationale"`
	StartTime    float64   `json:"start_time"`
	EndTime      float64   `json:"end_time"`
	Duration     float64   `json:"duration"`
	StoragePath  string    `json:"storage_path"`
	StorageURL   string    `json:"storage_url"`
	ViralScore   float64   `json:"viral_score"`
	Hashtags     string    `json:"hashtags"`      // JSON array string
	SuggestedFor string    `json:"suggested_for"` // JSON array string
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Clip) TableName() string { return "clips" }

// ScheduledPost is a minimal worker-side representation of a scheduled post.
type ScheduledPost struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	ClipID          string     `json:"clip_id"`
	UserID          string     `json:"user_id"`
	SocialAccountID string     `json:"social_account_id"`
	Platform        string     `json:"platform"`
	Caption         string     `json:"caption"`
	Hashtags        string     `json:"hashtags"`
	ScheduledAt     time.Time  `json:"scheduled_at"`
	PublishAt       *time.Time `json:"publish_at"`
	PublishedAt     *time.Time `json:"published_at"`
	Status          string     `json:"status"`
	RetryCount      int        `json:"retry_count"`
	ErrorMessage    string     `json:"error_message"`
	PlatformPostID  string     `json:"platform_post_id"`
	PlatformPostURL string     `json:"platform_post_url"`
	UploadProgress  int        `gorm:"default:0" json:"upload_progress"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (ScheduledPost) TableName() string { return "scheduled_posts" }

// SocialAccount is a minimal worker-side representation of a connected account.
type SocialAccount struct {
	ID                   string     `gorm:"primaryKey" json:"id"`
	UserID               string     `json:"user_id"`
	Platform             string     `json:"platform"`
	AccessToken          string     `json:"access_token"`
	RefreshToken         string     `json:"refresh_token"`
	TokenExpiresAt       *time.Time `gorm:"column:expires_at" json:"expires_at"`
	IsActive             bool       `json:"is_active"`
	TokenRefreshAttempts int        `gorm:"default:0" json:"token_refresh_attempts"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (SocialAccount) TableName() string { return "social_accounts" }

// PublishingLog captures status transitions and errors while publishing posts.
type PublishingLog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	PostID    string    `json:"post_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PublishingLog) TableName() string { return "publishing_logs" }

// ClipAnalytics is a minimal worker-side representation of clip performance metrics.
type ClipAnalytics struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	ClipID         string    `gorm:"index;not null" json:"clip_id"`
	PostID         string    `gorm:"index" json:"post_id"`
	Platform       string    `json:"platform"`
	RecordedAt     time.Time `json:"recorded_at"`
	Views          int64     `json:"views"`
	Likes          int64     `json:"likes"`
	Comments       int64     `json:"comments"`
	Shares         int64     `json:"shares"`
	Saves          int64     `json:"saves"`
	Reach          int64     `json:"reach"`
	Impressions    int64     `json:"impressions"`
	EngagementRate float64   `json:"engagement_rate"`
	WatchTime      float64   `json:"watch_time"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (ClipAnalytics) TableName() string { return "clip_analytics" }

// FailedJobRecord mirrors the API's FailedJob model for worker-side DB access.
type FailedJobRecord struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	JobID        string     `gorm:"index;not null" json:"job_id"`
	QueueName    string     `gorm:"not null" json:"queue_name"`
	Payload      string     `json:"payload"`
	ErrorMessage string     `json:"error_message"`
	RetryCount   int        `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries   int        `gorm:"not null;default:3" json:"max_retries"`
	Status       string     `gorm:"not null;default:'pending'" json:"status"`
	LastRetryAt  *time.Time `json:"last_retry_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (FailedJobRecord) TableName() string { return "failed_jobs" }

// newUUID generates a random UUID v4 string without external dependencies.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read failures are catastrophic (e.g. insufficient entropy); panic to
		// avoid silently creating duplicate IDs.
		panic(fmt.Sprintf("failed to generate UUID: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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
		"status":     VideoStatusCompleted,
		"updated_at": now,
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
	db           *gorm.DB
	redis        *redis.Client
	httpClient   *http.Client
	maxRetries   int
	tokenRefresh *TokenRefreshService
}

// NewPublishingWorker creates a new PublishingWorker.
func NewPublishingWorker(db *gorm.DB, rdb *redis.Client) *PublishingWorker {
	return &PublishingWorker{
		db:           db,
		redis:        rdb,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		maxRetries:   3,
		tokenRefresh: NewTokenRefreshService(db, rdb),
	}
}

// ProcessScheduledPosts publishes posts that are due.
func (w *PublishingWorker) ProcessScheduledPosts(ctx context.Context) {
	log.Info().Msg("PublishingWorker: scanning for publishable posts")
	var posts []ScheduledPost
	if err := w.db.WithContext(ctx).
		Where("status = ?", "publishing").
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

	var post ScheduledPost
	if err := w.db.WithContext(ctx).Where("id = ?", postID).First(&post).Error; err != nil {
		log.Error().Err(err).Str("post_id", postID).Msg("Failed to load scheduled post")
		return
	}

	var account SocialAccount
	if err := w.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_active = ?", post.SocialAccountID, post.UserID, true).
		First(&account).Error; err != nil {
		w.failPostWithRetry(ctx, post, "connected social account is missing or inactive")
		return
	}

	if err := w.createPublishingLog(ctx, post.ID, "publishing", "publishing started"); err != nil {
		log.Error().Err(err).Str("post_id", post.ID).Msg("Failed to insert publishing log")
	}

	if tokenErr := w.ensureValidAccessToken(ctx, &account); tokenErr != nil {
		w.failPostWithRetry(ctx, post, tokenErr.Error())
		return
	}

	// Load the clip so the uploader can access the storage path / URL.
	var clip Clip
	if err := w.db.WithContext(ctx).Where("id = ?", post.ClipID).First(&clip).Error; err != nil {
		w.failPostWithRetry(ctx, post, fmt.Sprintf("clip not found: %v", err))
		return
	}

	// Delegate the actual upload to the platform-specific uploader.
	uploader := uploaderForPlatform(post.Platform, w.redis)
	platformPostID, platformPostURL, uploadErr := uploader.Upload(ctx, post, account, clip)
	if uploadErr != nil {
		w.failPostWithRetry(ctx, post, fmt.Sprintf("upload failed: %v", uploadErr))
		return
	}

	now := time.Now()
	if err := w.db.WithContext(ctx).Table("scheduled_posts").Where("id = ?", postID).Updates(map[string]interface{}{
		"status":            "published",
		"published_at":      now,
		"retry_count":       post.RetryCount,
		"platform_post_id":  platformPostID,
		"platform_post_url": platformPostURL,
		"upload_progress":   100,
		"error_message":     "",
		"updated_at":        now,
	}).Error; err != nil {
		w.failPostWithRetry(ctx, post, fmt.Sprintf("failed to persist publish result: %v", err))
		return
	}

	_ = w.createPublishingLog(ctx, post.ID, "published", "post published successfully")

	log.Info().Str("post_id", postID).Msg("Post published successfully")
}

func (w *PublishingWorker) ensureValidAccessToken(ctx context.Context, account *SocialAccount) error {
	if account.AccessToken == "" {
		return fmt.Errorf("missing access token")
	}
	if account.TokenExpiresAt == nil || account.TokenExpiresAt.After(time.Now().UTC().Add(30*time.Second)) {
		return nil
	}
	if account.RefreshToken == "" {
		return fmt.Errorf("access token expired and refresh_token is missing")
	}
	if err := w.tokenRefresh.RefreshAccountToken(ctx, account); err != nil {
		return fmt.Errorf("access token refresh failed: %w", err)
	}
	return nil
}

func (w *PublishingWorker) failPostWithRetry(ctx context.Context, post ScheduledPost, reason string) {
	nextRetryCount := post.RetryCount + 1
	status := "scheduled"
	msg := fmt.Sprintf("publish failed: %s", reason)
	nextPublishAt := time.Now().UTC().Add(time.Duration(nextRetryCount*2) * time.Minute)
	updates := map[string]interface{}{
		"retry_count":   nextRetryCount,
		"error_message": reason,
		"updated_at":    time.Now().UTC(),
		"publish_at":    nextPublishAt,
		"scheduled_at":  nextPublishAt,
		"status":        status,
	}
	if nextRetryCount >= w.maxRetries {
		updates["status"] = "failed"
		msg = fmt.Sprintf("publish permanently failed after %d retries: %s", nextRetryCount, reason)
	}
	if err := w.db.WithContext(ctx).Table("scheduled_posts").Where("id = ?", post.ID).Updates(updates).Error; err != nil {
		log.Error().Err(err).Str("post_id", post.ID).Msg("failed to update post after publish failure")
	}
	_ = w.createPublishingLog(ctx, post.ID, updates["status"].(string), msg)
}

func (w *PublishingWorker) createPublishingLog(ctx context.Context, postID, status, message string) error {
	now := time.Now().UTC()
	rec := PublishingLog{
		ID:        newUUID(),
		PostID:    postID,
		Status:    status,
		Message:   message,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return w.db.WithContext(ctx).Create(&rec).Error
}

// SchedulerWorker transitions due posts into the publishing queue/state.
type SchedulerWorker struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewSchedulerWorker creates a new SchedulerWorker.
func NewSchedulerWorker(db *gorm.DB, rdb *redis.Client) *SchedulerWorker {
	return &SchedulerWorker{db: db, redis: rdb}
}

// EnqueueDuePosts marks due scheduled posts as publishing.
func (w *SchedulerWorker) EnqueueDuePosts(ctx context.Context) {
	log.Info().Msg("SchedulerWorker: scanning for due scheduled posts")
	var posts []ScheduledPost
	if err := w.db.WithContext(ctx).
		Where("status IN ? AND COALESCE(publish_at, scheduled_at) <= ?", []string{"scheduled", "pending"}, time.Now().UTC()).
		Limit(50).
		Find(&posts).Error; err != nil {
		log.Error().Err(err).Msg("SchedulerWorker: failed to query due posts")
		return
	}

	for _, post := range posts {
		if err := w.db.WithContext(ctx).Model(&ScheduledPost{}).Where("id = ?", post.ID).Updates(map[string]interface{}{
			"status":     "publishing",
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
			log.Error().Err(err).Str("post_id", post.ID).Msg("SchedulerWorker: failed to transition post to publishing")
			continue
		}
		log.Info().Str("post_id", post.ID).Msg("SchedulerWorker: queued post for publishing")
	}
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

	// Allowlist prevents SQL injection via table names
	allowedTables := map[string]bool{
		"videos":          true,
		"clips":           true,
		"scheduled_posts": true,
	}
	tables := []string{"videos", "clips", "scheduled_posts"}
	for _, table := range tables {
		if !allowedTables[table] {
			log.Warn().Str("table", table).Msg("Skipping disallowed table in cleanup")
			continue
		}
		result := w.db.WithContext(ctx).
			Exec(fmt.Sprintf("DELETE FROM %s WHERE deleted_at IS NOT NULL AND deleted_at < ?", table), cutoff) //nolint:gosec // table name is from an explicit allowlist
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
