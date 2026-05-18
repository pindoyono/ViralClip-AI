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
	ID              uuid.UUID              `json:"id"`
	Name            string                 `json:"name"`
	Email           string                 `json:"email"`
	AvatarURL       string                 `json:"avatar_url"`
	IsEmailVerified bool                   `json:"is_email_verified"`
	Tier            models.SubscriptionTier `json:"tier"`
	CreatedAt       time.Time              `json:"created_at"`
	LastLoginAt     *time.Time             `json:"last_login_at,omitempty"`
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

// =============================================================================
// Scheduled Post DTOs
// =============================================================================

type CreateScheduledPostRequest struct {
	ClipID          uuid.UUID `json:"clip_id" validate:"required"`
	SocialAccountID uuid.UUID `json:"social_account_id" validate:"required"`
	ScheduledAt     time.Time `json:"scheduled_at" validate:"required"`
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
	PublishedAt     *time.Time            `json:"published_at,omitempty"`
	Caption         string                `json:"caption"`
	Hashtags        string                `json:"hashtags"`
	PlatformPostURL string                `json:"platform_post_url,omitempty"`
	Status          models.PostStatus     `json:"status"`
	ErrorMessage    string                `json:"error_message,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	Clip            *ClipResponse         `json:"clip,omitempty"`
}

// =============================================================================
// Analytics DTOs
// =============================================================================

type AnalyticsSummaryResponse struct {
	TotalViews      int64         `json:"total_views"`
	TotalLikes      int64         `json:"total_likes"`
	TotalComments   int64         `json:"total_comments"`
	TotalShares     int64         `json:"total_shares"`
	AvgEngagement   float64       `json:"avg_engagement_rate"`
	TopClip         *ClipResponse `json:"top_clip,omitempty"`
	TopPlatform     string        `json:"top_platform"`
	PublishedClips  int           `json:"published_clips"`
	ClipsPublished  int           `json:"clips_published"`
	ScheduledPosts  int           `json:"scheduled_posts"`
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
