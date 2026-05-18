package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// =============================================================================
// Auth DTOs
// =============================================================================

type RegisterRequest struct {
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type AuthResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresAt    time.Time    `json:"expires_at"`
}

// =============================================================================
// User DTOs
// =============================================================================

type UserResponse struct {
	ID              uuid.UUID               `json:"id"`
	Name            string                  `json:"name"`
	Email           string                  `json:"email"`
	AvatarURL       string                  `json:"avatar_url"`
	IsEmailVerified bool                    `json:"is_email_verified"`
	Tier            models.SubscriptionTier `json:"tier"`
	CreatedAt       time.Time               `json:"created_at"`
	LastLoginAt     *time.Time              `json:"last_login_at,omitempty"`
}

type UpdateUserRequest struct {
	Name      *string `json:"name" validate:"omitempty,min=2,max=100"`
	AvatarURL *string `json:"avatar_url" validate:"omitempty,url"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=128"`
}

// =============================================================================
// Video DTOs
// =============================================================================

type UploadVideoRequest struct {
	Title            string     `form:"title" validate:"required,min=1,max=255"`
	Description      string     `form:"description"`
	ContentProfileID *uuid.UUID `form:"content_profile_id"`
}

type VideoResponse struct {
	ID               uuid.UUID          `json:"id"`
	UserID           uuid.UUID          `json:"user_id"`
	ContentProfileID *uuid.UUID         `json:"content_profile_id,omitempty"`
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	StorageURL       string             `json:"storage_url"`
	ThumbnailURL     string             `json:"thumbnail_url"`
	Duration         float64            `json:"duration"`
	FileSize         int64              `json:"file_size"`
	Width            int                `json:"width"`
	Height           int                `json:"height"`
	Status           models.VideoStatus `json:"status"`
	ErrorMessage     string             `json:"error_message,omitempty"`
	ClipsCount       int                `json:"clips_count"`
	CreatedAt        time.Time          `json:"created_at"`
	ProcessedAt      *time.Time         `json:"processed_at,omitempty"`
}

type VideoListResponse struct {
	Videos     []VideoResponse `json:"videos"`
	Pagination PaginationMeta  `json:"pagination"`
}

// =============================================================================
// Clip DTOs
// =============================================================================

type ClipResponse struct {
	ID           uuid.UUID         `json:"id"`
	VideoID      uuid.UUID         `json:"video_id"`
	UserID       uuid.UUID         `json:"user_id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	HookText     string            `json:"hook_text"`
	StorageURL   string            `json:"storage_url"`
	ThumbnailURL string            `json:"thumbnail_url"`
	StartTime    float64           `json:"start_time"`
	EndTime      float64           `json:"end_time"`
	Duration     float64           `json:"duration"`
	ViralScore   float64           `json:"viral_score"`
	AIRationale  string            `json:"ai_rationale"`
	Hashtags     []string          `json:"hashtags"`
	SuggestedFor []string          `json:"suggested_for"`
	Status       models.ClipStatus `json:"status"`
	HasSubtitles bool              `json:"has_subtitles"`
	CreatedAt    time.Time         `json:"created_at"`
}

type ClipListResponse struct {
	Data       []ClipResponse `json:"data"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}

type UpdateClipRequest struct {
	Title       *string  `json:"title" validate:"omitempty,min=1,max=255"`
	Description *string  `json:"description"`
	HookText    *string  `json:"hook_text"`
	Hashtags    []string `json:"hashtags"`
}

// =============================================================================
// Content Profile DTOs
// =============================================================================

type CreateContentProfileRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100"`
	Platform    string `json:"platform" validate:"required,oneof=youtube tiktok instagram general"`
	Niche       string `json:"niche"`
	ToneStyle   string `json:"tone_style" validate:"omitempty,oneof=educational entertaining inspirational informative humorous"`
	AudienceAge string `json:"audience_age"`
	Keywords    string `json:"keywords"`
	IsDefault   bool   `json:"is_default"`
}

type UpdateContentProfileRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=100"`
	Niche       *string `json:"niche"`
	ToneStyle   *string `json:"tone_style"`
	AudienceAge *string `json:"audience_age"`
	Keywords    *string `json:"keywords"`
	IsDefault   *bool   `json:"is_default"`
}

type ContentProfileResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Name        string    `json:"name"`
	Platform    string    `json:"platform"`
	Niche       string    `json:"niche"`
	ToneStyle   string    `json:"tone_style"`
	AudienceAge string    `json:"audience_age"`
	Keywords    string    `json:"keywords"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}

// =============================================================================
// Social Account DTOs
// =============================================================================

type SocialAccountResponse struct {
	ID             uuid.UUID             `json:"id"`
	UserID         uuid.UUID             `json:"user_id"`
	Platform       models.SocialPlatform `json:"platform"`
	Username       string                `json:"username"`
	DisplayName    string                `json:"display_name"`
	AvatarURL      string                `json:"avatar_url"`
	IsActive       bool                  `json:"is_active"`
	FollowersCount int64                 `json:"followers_count"`
	ConnectedAt    time.Time             `json:"connected_at"`
	LastSyncedAt   *time.Time            `json:"last_synced_at,omitempty"`
}

type ConnectSocialAccountRequest struct {
	Platform       string     `json:"platform" validate:"required,oneof=tiktok instagram youtube twitter"`
	Username       string     `json:"username" validate:"required,min=1,max=100"`
	DisplayName    string     `json:"display_name"`
	AvatarURL      string     `json:"avatar_url"`
	AccessToken    string     `json:"access_token"`
	RefreshToken   string     `json:"refresh_token"`
	ExpiresAt      *time.Time `json:"expires_at"`
	FollowersCount int64      `json:"followers_count"`
}

type DisconnectSocialAccountRequest struct {
	AccountID uuid.UUID `json:"account_id" validate:"required"`
}

// =============================================================================
// Scheduled Post DTOs
// =============================================================================

type CreateScheduledPostRequest struct {
	ClipID          uuid.UUID  `json:"clip_id" validate:"required"`
	SocialAccountID uuid.UUID  `json:"social_account_id" validate:"required"`
	ScheduledAt     time.Time  `json:"scheduled_at" validate:"required"`
	PublishAt       *time.Time `json:"publish_at,omitempty"`
	Caption         string     `json:"caption"`
	Hashtags        string     `json:"hashtags"`
}

type PublishNowRequest struct {
	ClipID          uuid.UUID `json:"clip_id" validate:"required"`
	SocialAccountID uuid.UUID `json:"social_account_id" validate:"required"`
	Caption         string    `json:"caption"`
	Hashtags        string    `json:"hashtags"`
}

type ScheduledPostResponse struct {
	ID              uuid.UUID             `json:"id"`
	ClipID          uuid.UUID             `json:"clip_id"`
	UserID          uuid.UUID             `json:"user_id"`
	SocialAccountID uuid.UUID             `json:"social_account_id"`
	Platform        models.SocialPlatform `json:"platform"`
	ScheduledAt     time.Time             `json:"scheduled_at"`
	PublishAt       *time.Time            `json:"publish_at,omitempty"`
	PublishedAt     *time.Time            `json:"published_at,omitempty"`
	Caption         string                `json:"caption"`
	Hashtags        string                `json:"hashtags"`
	PlatformPostURL string                `json:"platform_post_url,omitempty"`
	Status          models.PostStatus     `json:"status"`
	ErrorMessage    string                `json:"error_message,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	Clip            *ClipResponse         `json:"clip,omitempty"`
}

type PublishingLogResponse struct {
	ID        uuid.UUID         `json:"id"`
	PostID    uuid.UUID         `json:"post_id"`
	Status    models.PostStatus `json:"status"`
	Message   string            `json:"message"`
	CreatedAt time.Time         `json:"created_at"`
}

type PublishStatusResponse struct {
	Post ScheduledPostResponse   `json:"post"`
	Logs []PublishingLogResponse `json:"logs"`
}

// =============================================================================
// Analytics DTOs
// =============================================================================

type AnalyticsSummaryResponse struct {
	TotalViews     int64         `json:"total_views"`
	TotalLikes     int64         `json:"total_likes"`
	TotalComments  int64         `json:"total_comments"`
	TotalShares    int64         `json:"total_shares"`
	AvgEngagement  float64       `json:"avg_engagement_rate"`
	TopClip        *ClipResponse `json:"top_clip,omitempty"`
	TopPlatform    string        `json:"top_platform"`
	PublishedClips int           `json:"published_clips"`
	// ClipsPublished is an alias for PublishedClips kept for backward compatibility.
	ClipsPublished int `json:"clips_published"`
	ScheduledPosts int `json:"scheduled_posts"`
}

// ClipAnalyticsResponse represents per-platform analytics for a single clip.
type ClipAnalyticsResponse struct {
	ID             string  `json:"id"`
	ClipID         string  `json:"clip_id"`
	Platform       string  `json:"platform"`
	Views          int64   `json:"views"`
	Likes          int64   `json:"likes"`
	Comments       int64   `json:"comments"`
	Shares         int64   `json:"shares"`
	Saves          int64   `json:"saves"`
	Reach          int64   `json:"reach"`
	EngagementRate float64 `json:"engagement_rate"`
	SyncedAt       string  `json:"synced_at"`
}

// =============================================================================
// Trending Topics DTOs
// =============================================================================

type TrendingTopicResponse struct {
	ID         uuid.UUID             `json:"id"`
	Platform   models.SocialPlatform `json:"platform"`
	Topic      string                `json:"topic"`
	Hashtag    string                `json:"hashtag"`
	Category   string                `json:"category"`
	TrendScore float64               `json:"trend_score"`
	PostCount  int64                 `json:"post_count"`
	GrowthRate float64               `json:"growth_rate"`
	ExpiresAt  time.Time             `json:"expires_at"`
}

type ViralOpportunityResponse struct {
	ID              uuid.UUID `json:"id"`
	SourcePlatform  string    `json:"source_platform"`
	ExternalVideoID string    `json:"external_video_id"`
	ChannelID       string    `json:"channel_id"`
	Title           string    `json:"title"`
	Category        string    `json:"category"`
	SourceQuery     string    `json:"source_query"`
	Views           int64     `json:"views"`
	PreviousViews   int64     `json:"previous_views"`
	Likes           int64     `json:"likes"`
	Comments        int64     `json:"comments"`
	SubscriberCount int64     `json:"subscriber_count"`
	PublishedAt     time.Time `json:"published_at"`
	LastCollectedAt time.Time `json:"last_collected_at"`
	ViewVelocity    float64   `json:"view_velocity"`
	EngagementRate  float64   `json:"engagement_rate"`
	OutlierScore    float64   `json:"outlier_score"`
	GrowthScore     float64   `json:"growth_score"`
	ViralScore      float64   `json:"viral_score"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ViralOpportunityListResponse struct {
	Data       []ViralOpportunityResponse `json:"data"`
	Total      int64                      `json:"total"`
	Page       int                        `json:"page"`
	Limit      int                        `json:"limit"`
	TotalPages int                        `json:"total_pages"`
}

type ViralOpportunityRecommendationResponse struct {
	Opportunity         ViralOpportunityResponse `json:"opportunity"`
	RecommendationScore float64                  `json:"recommendation_score"`
	Reasons             []string                 `json:"reasons"`
	MatchedProfiles     []string                 `json:"matched_profiles"`
}

// =============================================================================
// Common DTOs
// =============================================================================

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type PaginationRequest struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
}

func (p *PaginationRequest) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 || p.Limit > 100 {
		p.Limit = 20
	}
}

func (p *PaginationRequest) Offset() int {
	return (p.Page - 1) * p.Limit
}

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// =============================================================================
// Hook Detection V2 DTOs
// =============================================================================

// HookDetectionSegmentRequest is a single transcript segment sent to the
// V2 detection endpoint.
type HookDetectionSegmentRequest struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// HookDetectRequest is the request body for POST /api/v1/videos/:id/hooks/detect.
type HookDetectRequest struct {
	Segments []HookDetectionSegmentRequest `json:"segments" validate:"required,min=1"`
	MinScore int                           `json:"min_score"` // default 50
}

// HookDetectionResultResponse is a single detected hook returned to the client.
type HookDetectionResultResponse struct {
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Type           string  `json:"type"`
	Score          int     `json:"score"`
	MatchedPattern string  `json:"matched_pattern"`
}

// HookDetectResponse is the response body for the detect endpoint.
type HookDetectResponse struct {
	VideoID string                        `json:"video_id"`
	Hooks   []HookDetectionResultResponse `json:"hooks"`
	Total   int                           `json:"total"`
}

// HookListResponse is the response body for listing stored detections.
type HookListResponse struct {
	VideoID string                        `json:"video_id"`
	Hooks   []HookDetectionResultResponse `json:"hooks"`
	Total   int                           `json:"total"`
}

// =============================================================================
// Clip Engine V2 DTOs
// =============================================================================

// ClipV2Segment is a single transcript segment in a V2 clip generation request.
type ClipV2Segment struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// ClipV2HookDetection is a hook detection result forwarded to the AI service.
type ClipV2HookDetection struct {
	Start          float64 `json:"start"`
	End            float64 `json:"end"`
	Type           string  `json:"type"`
	Score          int     `json:"score"`
	MatchedPattern string  `json:"matched_pattern"`
}

// ClipV2GenerateRequest is the request body for POST /api/v1/videos/:id/clips/v2/generate.
type ClipV2GenerateRequest struct {
	Segments     []ClipV2Segment `json:"segments" validate:"required,min=1"`
	ProfileType  string          `json:"profile_type"`   // gaming|comedy|education|politics|podcast|general
	MinClipScore int             `json:"min_clip_score"` // default 50
	MaxClips     int             `json:"max_clips"`      // default 10
}

// ClipV2ResultItem is a single clip candidate returned from the V2 engine.
type ClipV2ResultItem struct {
	Start          string  `json:"start"` // HH:MM:SS
	End            string  `json:"end"`   // HH:MM:SS
	StartSeconds   float64 `json:"start_seconds"`
	EndSeconds     float64 `json:"end_seconds"`
	Score          int     `json:"score"`
	HookScore      float64 `json:"hook_score"`
	EmotionScore   float64 `json:"emotion_score"`
	StoryScore     float64 `json:"story_score"`
	RetentionScore float64 `json:"retention_score"`
	ProfileType    string  `json:"profile_type"`
}

// ClipV2GenerateResponse is the response body for the V2 clip generation endpoint.
type ClipV2GenerateResponse struct {
	VideoID     string             `json:"video_id"`
	ProfileType string             `json:"profile_type"`
	Clips       []ClipV2ResultItem `json:"clips"`
	Total       int                `json:"total"`
}

// =============================================================================
// Subtitle Burning DTOs
// =============================================================================

// SubtitleBurnRequest is the request body for POST /api/v1/videos/:id/subtitles/burn.
// All fields are optional; when omitted the AI service uses its defaults.
type SubtitleBurnRequest struct {
	// Style controls the visual appearance of the subtitles.
	// Valid values: "default", "bold", "outline", "shadow".
	Style string `json:"style"`

	// FontSize is the subtitle font size in points (12–72, default 24).
	FontSize int `json:"font_size"`

	// PrimaryColor is the subtitle text colour in ASS/SSA &HBBGGRR format.
	PrimaryColor string `json:"primary_color"`

	// OutlineColor is the subtitle outline/border colour in ASS/SSA &HBBGGRR format.
	OutlineColor string `json:"outline_color"`
}

// SubtitleBurnResponse is the response body for the subtitle burn endpoint.
type SubtitleBurnResponse struct {
	VideoID        string `json:"video_id"`
	ClipsProcessed int    `json:"clips_processed"`
}

// =============================================================================
// Real-Time Job Status DTOs
// =============================================================================

// PipelineStage represents the name of a video processing pipeline step.
type PipelineStage string

const (
	PipelineStageTranscript PipelineStage = "transcript"
	PipelineStageClip       PipelineStage = "clip"
	PipelineStageSubtitle   PipelineStage = "subtitle"
	PipelineStageUpload     PipelineStage = "upload"
	PipelineStageCompleted  PipelineStage = "completed"
)

// StageStatus is the status of a single pipeline stage.
type StageStatus string

const (
	StageStatusPending    StageStatus = "pending"
	StageStatusProcessing StageStatus = "processing"
	StageStatusDone       StageStatus = "done"
	StageStatusFailed     StageStatus = "failed"
	StageStatusSkipped    StageStatus = "skipped"
)

// PipelineStageInfo describes one stage in the processing pipeline.
type PipelineStageInfo struct {
	Stage  PipelineStage `json:"stage"`
	Status StageStatus   `json:"status"`
	Label  string        `json:"label"`
}

// JobStatusResponse is returned by GET /api/v1/videos/:id/job-status.
type JobStatusResponse struct {
	VideoID      string              `json:"video_id"`
	VideoStatus  string              `json:"video_status"`
	JobStatus    string              `json:"job_status"`
	CurrentStage PipelineStage       `json:"current_stage"`
	Stages       []PipelineStageInfo `json:"stages"`
}

// WSMessage is the envelope for WebSocket messages pushed to clients.
type WSMessage struct {
	// Type identifies the message kind. Current values: "status_update", "ping".
	Type    string      `json:"type"`
	VideoID string      `json:"video_id,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

// =============================================================================
// Metadata Enhancement DTOs
// =============================================================================

// EnhanceMetadataRequest is the optional request body for
// POST /api/v1/clips/:id/metadata/enhance.
type EnhanceMetadataRequest struct {
	// Platform specifies the target social platform for metadata optimisation.
	// Valid values: tiktok | instagram | youtube | twitter. Defaults to "tiktok".
	Platform string `json:"platform"`
	// Niche is an optional content niche descriptor (e.g. "tech", "fitness").
	Niche string `json:"niche"`
	// Tone is an optional tone descriptor (e.g. "educational", "humorous").
	Tone string `json:"tone"`
}

// MetadataEnhanceResponse is returned after the clip is enhanced.
// The clip's title, description, and hashtags are updated in the database.
type MetadataEnhanceResponse struct {
	// Clip is the updated clip record.
	Clip ClipResponse `json:"clip"`
	// Keywords are SEO-relevant keywords suggested by the AI.
	Keywords []string `json:"keywords"`
	// Category is the primary content category inferred by the AI.
	Category string `json:"category"`
	// OptimalPostTimes contains suggested posting times (e.g. "7:00 PM EST on Weekdays").
	OptimalPostTimes []string `json:"optimal_post_times"`
}

// =============================================================================
// Learning Feedback Engine DTOs (Task 3)
// =============================================================================

// ClipCPSResponse is a clip decorated with its Content Performance Score.
type ClipCPSResponse struct {
	ClipID     uuid.UUID `json:"clip_id"`
	Title      string    `json:"title"`
	Platform   string    `json:"platform"`
	CPS        float64   `json:"cps"`
	Views      int64     `json:"views"`
	Likes      int64     `json:"likes"`
	Comments   int64     `json:"comments"`
	WatchTime  float64   `json:"watch_time"`
	Duration   float64   `json:"duration"`
	ViralScore float64   `json:"viral_score"`
}

// HookPatternResponse describes CPS performance aggregated by hook type.
type HookPatternResponse struct {
	HookType    string  `json:"hook_type"`
	AvgCPS      float64 `json:"avg_cps"`
	ClipCount   int     `json:"clip_count"`
	AvgViews    float64 `json:"avg_views"`
	Improvement float64 `json:"improvement_pct"` // % improvement over baseline
}

// RecommendationResponse is a human-readable learning insight.
type RecommendationResponse struct {
	ProfileName string  `json:"profile_name"`
	Platform    string  `json:"platform"`
	Insight     string  `json:"insight"`
	Confidence  float64 `json:"confidence"`
}

// =============================================================================
// Resumable Upload DTOs (Task 5)
// =============================================================================

// UploadProgressResponse is returned by GET /api/v1/videos/:id/upload-progress.
// It also matches the shape pushed over WebSocket:
//
//	{"progress": 67, "status": "uploading"}
type UploadProgressResponse struct {
	// UploadID is the identifier used to track this upload session.
	UploadID string `json:"upload_id"`
	// Progress is the percentage of bytes uploaded (0–100).
	Progress int `json:"progress"`
	// Status is the current upload state: "uploading", "completed", or "failed".
	Status string `json:"status"`
	// UploadedBytes is the number of bytes confirmed received by the storage backend.
	UploadedBytes int64 `json:"uploaded_bytes"`
	// TotalBytes is the expected total file size in bytes.
	TotalBytes int64 `json:"total_bytes"`
	// Error contains a description when Status is "failed".
	Error string `json:"error,omitempty"`
}
