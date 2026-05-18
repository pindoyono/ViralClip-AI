package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base provides common fields for all models.
type Base struct {
	ID        uuid.UUID      `gorm:"type:varchar(36);primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// SubscriptionTier represents the user's subscription level.
type SubscriptionTier string

const (
	TierFree       SubscriptionTier = "free"
	TierStarter    SubscriptionTier = "starter"
	TierPro        SubscriptionTier = "pro"
	TierEnterprise SubscriptionTier = "enterprise"
)

// User represents a registered user.
type User struct {
	Base
	Email                string           `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash         string           `gorm:"not null" json:"-"`
	Name                 string           `gorm:"not null" json:"name"`
	AvatarURL            string           `json:"avatar_url"`
	IsEmailVerified      bool             `gorm:"not null" json:"is_email_verified"`
	IsActive             bool             `gorm:"not null" json:"is_active"`
	Tier                 SubscriptionTier `gorm:"default:'free'" json:"tier"`
	StripeCustomerID     string           `gorm:"uniqueIndex" json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string           `json:"stripe_subscription_id,omitempty"`
	GoogleID             string           `gorm:"index" json:"google_id,omitempty"`
	RefreshToken         string           `json:"-"`
	LastLoginAt          *time.Time       `json:"last_login_at,omitempty"`
	ResetToken           string           `json:"-"`
	ResetTokenExpiry     *time.Time       `json:"-"`

	// Relationships
	ContentProfiles []ContentProfile `gorm:"foreignKey:UserID" json:"content_profiles,omitempty"`
	Videos          []Video          `gorm:"foreignKey:UserID" json:"videos,omitempty"`
	SocialAccounts  []SocialAccount  `gorm:"foreignKey:UserID" json:"social_accounts,omitempty"`
}

// ContentProfile stores the AI-generated content strategy for a user.
type ContentProfile struct {
	Base
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Name        string    `gorm:"not null" json:"name"`
	Platform    string    `gorm:"not null" json:"platform"` // youtube, tiktok, instagram, general
	Niche       string    `json:"niche"`
	ToneStyle   string    `json:"tone_style"`                // educational, entertaining, inspirational
	AudienceAge string    `json:"audience_age"`              // 18-24, 25-34, etc.
	Keywords    string    `gorm:"type:text" json:"keywords"` // comma-separated
	IsDefault   bool      `gorm:"default:false" json:"is_default"`

	User   User    `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Videos []Video `gorm:"foreignKey:ContentProfileID" json:"videos,omitempty"`
}

// VideoStatus tracks the processing state of a video.
type VideoStatus string

const (
	VideoStatusPending    VideoStatus = "pending"
	VideoStatusUploading  VideoStatus = "uploading"
	VideoStatusProcessing VideoStatus = "processing"
	VideoStatusCompleted  VideoStatus = "completed"
	VideoStatusFailed     VideoStatus = "failed"
)

// Video represents an uploaded source video.
type Video struct {
	Base
	UserID           uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	ContentProfileID *uuid.UUID  `gorm:"type:uuid;index" json:"content_profile_id,omitempty"`
	Title            string      `gorm:"not null" json:"title"`
	Description      string      `gorm:"type:text" json:"description"`
	OriginalFilename string      `json:"original_filename"`
	StoragePath      string      `json:"storage_path"`
	StorageURL       string      `json:"storage_url"`
	ThumbnailURL     string      `json:"thumbnail_url"`
	Duration         float64     `json:"duration"`  // in seconds
	FileSize         int64       `json:"file_size"` // in bytes
	MimeType         string      `json:"mime_type"`
	Width            int         `json:"width"`
	Height           int         `json:"height"`
	FPS              float64     `json:"fps"`
	Status           VideoStatus `gorm:"default:'pending'" json:"status"`
	ErrorMessage     string      `json:"error_message,omitempty"`
	TranscriptPath   string      `json:"transcript_path,omitempty"`
	ProcessedAt      *time.Time  `json:"processed_at,omitempty"`

	User           User            `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ContentProfile *ContentProfile `gorm:"foreignKey:ContentProfileID" json:"content_profile,omitempty"`
	Clips          []Clip          `gorm:"foreignKey:VideoID" json:"clips,omitempty"`
}

// ClipStatus tracks the state of a generated clip.
type ClipStatus string

const (
	ClipStatusGenerating ClipStatus = "generating"
	ClipStatusReady      ClipStatus = "ready"
	ClipStatusPublished  ClipStatus = "published"
	ClipStatusFailed     ClipStatus = "failed"
	ClipStatusArchived   ClipStatus = "archived"
)

// Clip represents an AI-generated clip from a source video.
type Clip struct {
	Base
	VideoID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"video_id"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Title        string     `gorm:"not null" json:"title"`
	Description  string     `gorm:"type:text" json:"description"`
	HookText     string     `gorm:"type:text" json:"hook_text"`
	StoragePath  string     `json:"storage_path"`
	StorageURL   string     `json:"storage_url"`
	ThumbnailURL string     `json:"thumbnail_url"`
	StartTime    float64    `json:"start_time"`  // seconds from original video
	EndTime      float64    `json:"end_time"`    // seconds from original video
	Duration     float64    `json:"duration"`    // clip duration in seconds
	ViralScore   float64    `json:"viral_score"` // 0-100 AI-generated score
	AIRationale  string     `gorm:"type:text" json:"ai_rationale"`
	Hashtags     string     `gorm:"type:text" json:"hashtags"`      // JSON array
	SuggestedFor string     `gorm:"type:text" json:"suggested_for"` // JSON array of platforms
	Status       ClipStatus `gorm:"default:'generating'" json:"status"`
	SubtitlePath string     `json:"subtitle_path,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`

	Video     Video           `gorm:"foreignKey:VideoID" json:"video,omitempty"`
	User      User            `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Analytics []ClipAnalytics `gorm:"foreignKey:ClipID" json:"analytics,omitempty"`
	Posts     []ScheduledPost `gorm:"foreignKey:ClipID" json:"posts,omitempty"`
}

// SocialPlatform identifies a social media platform.
type SocialPlatform string

const (
	PlatformTikTok    SocialPlatform = "tiktok"
	PlatformYouTube   SocialPlatform = "youtube"
	PlatformInstagram SocialPlatform = "instagram"
)

// SocialAccount represents a connected social media account.
type SocialAccount struct {
	Base
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Platform       SocialPlatform `gorm:"not null" json:"platform"`
	PlatformUserID string         `gorm:"not null" json:"platform_user_id"`
	Username       string         `json:"username"`
	DisplayName    string         `json:"display_name"`
	AvatarURL      string         `json:"avatar_url"`
	AccessToken    string         `json:"-"`
	RefreshToken   string         `json:"-"`
	TokenExpiresAt *time.Time     `gorm:"column:expires_at" json:"expires_at,omitempty"`
	IsActive       bool           `gorm:"default:true" json:"is_active"`
	FollowersCount int64          `json:"followers_count"`
	LastSyncedAt   *time.Time     `json:"last_synced_at,omitempty"`

	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// PostStatus represents the publishing status of a scheduled post.
type PostStatus string

const (
	PostStatusScheduled  PostStatus = "scheduled"
	PostStatusPublishing PostStatus = "publishing"
	PostStatusPublished  PostStatus = "published"
	PostStatusFailed     PostStatus = "failed"
	PostStatusCancelled  PostStatus = "cancelled"
)

// ScheduledPost represents a clip scheduled for publication on a social platform.
type ScheduledPost struct {
	Base
	ClipID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"clip_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	SocialAccountID uuid.UUID      `gorm:"type:uuid;not null;index" json:"social_account_id"`
	Platform        SocialPlatform `gorm:"not null" json:"platform"`
	ScheduledAt     time.Time      `gorm:"not null" json:"scheduled_at"`
	PublishAt       *time.Time     `json:"publish_at,omitempty"`
	PublishedAt     *time.Time     `json:"published_at,omitempty"`
	Caption         string         `gorm:"type:text" json:"caption"`
	Hashtags        string         `gorm:"type:text" json:"hashtags"`
	PlatformPostID  string         `json:"platform_post_id,omitempty"`
	PlatformPostURL string         `json:"platform_post_url,omitempty"`
	Status          PostStatus     `gorm:"default:'scheduled'" json:"status"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	RetryCount      int            `gorm:"default:0" json:"retry_count"`

	Clip          Clip            `gorm:"foreignKey:ClipID" json:"clip,omitempty"`
	User          User            `gorm:"foreignKey:UserID" json:"user,omitempty"`
	SocialAccount SocialAccount   `gorm:"foreignKey:SocialAccountID" json:"social_account,omitempty"`
	Logs          []PublishingLog `gorm:"foreignKey:PostID" json:"logs,omitempty"`
}

// PublishingLog tracks publish attempts and outcomes for scheduled posts.
type PublishingLog struct {
	Base
	PostID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"post_id"`
	Status  PostStatus `gorm:"not null" json:"status"`
	Message string     `gorm:"type:text" json:"message"`

	Post ScheduledPost `gorm:"foreignKey:PostID" json:"post,omitempty"`
}

// ClipAnalytics stores performance metrics for published clips.
type ClipAnalytics struct {
	Base
	ClipID         uuid.UUID      `gorm:"type:uuid;not null;index" json:"clip_id"`
	PostID         *uuid.UUID     `gorm:"type:uuid;index" json:"post_id,omitempty"`
	Platform       SocialPlatform `json:"platform"`
	RecordedAt     time.Time      `gorm:"not null" json:"recorded_at"`
	Views          int64          `json:"views"`
	Likes          int64          `json:"likes"`
	Comments       int64          `json:"comments"`
	Shares         int64          `json:"shares"`
	Saves          int64          `json:"saves"`
	Reach          int64          `json:"reach"`
	Impressions    int64          `json:"impressions"`
	EngagementRate float64        `json:"engagement_rate"`
	WatchTime      float64        `json:"watch_time"` // average watch time in seconds

	Clip Clip           `gorm:"foreignKey:ClipID" json:"clip,omitempty"`
	Post *ScheduledPost `gorm:"foreignKey:PostID" json:"post,omitempty"`
}

// TrendingTopic stores trending topics discovered by the analytics worker.
type TrendingTopic struct {
	Base
	Platform   SocialPlatform `gorm:"not null;index" json:"platform"`
	Topic      string         `gorm:"not null" json:"topic"`
	Hashtag    string         `json:"hashtag"`
	Category   string         `json:"category"`
	TrendScore float64        `json:"trend_score"`
	PostCount  int64          `json:"post_count"`
	ViewCount  int64          `json:"view_count"`
	GrowthRate float64        `json:"growth_rate"` // percentage growth
	Region     string         `json:"region"`
	ExpiresAt  time.Time      `json:"expires_at"`
}

// ViralOpportunity stores external trend signals collected by the worker.
type ViralOpportunity struct {
	Base
	SourcePlatform  string    `gorm:"not null;uniqueIndex:idx_viral_opportunity_source,priority:1" json:"source_platform"`
	ExternalVideoID string    `gorm:"not null;uniqueIndex:idx_viral_opportunity_source,priority:2" json:"external_video_id"`
	ChannelID       string    `gorm:"not null" json:"channel_id"`
	Title           string    `gorm:"type:text;not null" json:"title"`
	Category        string    `json:"category"`
	SourceQuery     string    `json:"source_query"`
	Views           int64     `gorm:"not null;default:0" json:"views"`
	PreviousViews   int64     `gorm:"not null;default:0" json:"previous_views"`
	Likes           int64     `gorm:"not null;default:0" json:"likes"`
	Comments        int64     `gorm:"not null;default:0" json:"comments"`
	SubscriberCount int64     `gorm:"not null;default:0" json:"subscriber_count"`
	PublishedAt     time.Time `gorm:"not null;index" json:"published_at"`
	LastCollectedAt time.Time `gorm:"not null;index" json:"last_collected_at"`
	ViewVelocity    float64   `gorm:"not null;default:0" json:"view_velocity"`
	EngagementRate  float64   `gorm:"not null;default:0" json:"engagement_rate"`
	OutlierScore    float64   `gorm:"not null;default:0" json:"outlier_score"`
	GrowthScore     float64   `gorm:"not null;default:0" json:"growth_score"`
	ViralScore      float64   `gorm:"not null;default:0;index" json:"viral_score"`
}

// FailedJobStatus tracks the recovery lifecycle of a dead-letter job.
type FailedJobStatus string

const (
	FailedJobStatusPending    FailedJobStatus = "pending"
	FailedJobStatusRecovering FailedJobStatus = "recovering"
	FailedJobStatusExhausted  FailedJobStatus = "exhausted"
)

// FailedJob persists dead-letter queue entries for inspection and recovery.
type FailedJob struct {
	ID           uuid.UUID       `gorm:"type:varchar(36);primaryKey" json:"id"`
	JobID        string          `gorm:"not null;index" json:"job_id"`
	QueueName    string          `gorm:"not null;index" json:"queue_name"`
	Payload      string          `gorm:"type:text;not null" json:"payload"`
	ErrorMessage string          `gorm:"type:text" json:"error_message"`
	RetryCount   int             `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries   int             `gorm:"not null;default:3" json:"max_retries"`
	Status       FailedJobStatus `gorm:"not null;default:'pending'" json:"status"`
	LastRetryAt  *time.Time      `json:"last_retry_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (f *FailedJob) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}

// HookDetection stores a detected hook moment from the V2 Hook Engine.
type HookDetection struct {
	Base
	VideoID        uuid.UUID `gorm:"type:uuid;not null;index" json:"video_id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Start          float64   `gorm:"not null" json:"start"`      // seconds
	End            float64   `gorm:"not null" json:"end"`        // seconds
	HookType       string    `gorm:"not null;index" json:"type"` // curiosity|emotion|storytelling|controversy|cta
	Score          int       `gorm:"not null" json:"score"`      // 0-100
	MatchedPattern string    `json:"matched_pattern"`            // text fragment that triggered detection

	Video Video `gorm:"foreignKey:VideoID" json:"video,omitempty"`
	User  User  `gorm:"foreignKey:UserID"  json:"user,omitempty"`
}
