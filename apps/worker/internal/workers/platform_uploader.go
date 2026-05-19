package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// uploadProgressKeyPrefix is the Redis key prefix for live upload progress.
const uploadProgressKeyPrefix = "upload:progress:"

// uploadProgressTTL is how long the Redis upload-progress key is retained after
// the upload completes (provides a short window for the client to poll).
const uploadProgressTTL = time.Hour

// idPrefix returns the first n characters of s, or all of s if it is shorter.
func idPrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
// Each implementation is responsible for updating upload progress in Redis as
// the upload advances, so the client can poll GET /publish/status for live feedback.
type PlatformUploader interface {
	// Upload uploads the clip to the platform and returns the
	// platform-assigned post ID and a public URL for the post.
	Upload(ctx context.Context, post ScheduledPost, account SocialAccount, clip Clip) (platformPostID, platformPostURL string, err error)
}

// uploaderForPlatform returns the correct PlatformUploader for the given
// platform name (tiktok | instagram | youtube).  Falls back to TikTok for
// unknown platform values.
func uploaderForPlatform(platform string, rdb *redis.Client) PlatformUploader {
	switch platform {
	case "youtube":
		return &YouTubeShortUploader{rdb: rdb}
	case "instagram":
		return &InstagramReelUploader{rdb: rdb}
	default:
		return &TikTokUploader{rdb: rdb}
	}
}

// setUploadProgress stores the upload progress percentage (0–100) in Redis.
// The key expires after uploadProgressTTL, so stale keys are cleaned up
// automatically even when the worker crashes mid-upload.
func setUploadProgress(ctx context.Context, rdb *redis.Client, postID string, pct int) {
	if rdb == nil {
		return
	}
	key := uploadProgressKeyPrefix + postID
	if err := rdb.Set(ctx, key, pct, uploadProgressTTL).Err(); err != nil {
		log.Warn().Err(err).Str("post_id", postID).Msg("platform_uploader: failed to set upload progress")
	}
}

// GetUploadProgress reads the current upload progress for a post from Redis.
// Returns -1 if no progress key exists (upload not started or key expired).
func GetUploadProgress(ctx context.Context, rdb *redis.Client, postID string) int {
	if rdb == nil {
		return -1
	}
	val, err := rdb.Get(ctx, uploadProgressKeyPrefix+postID).Int()
	if err != nil {
		return -1
	}
	return val
}

// ---------------------------------------------------------------------------
// YouTube Shorts uploader
// ---------------------------------------------------------------------------

// YouTubeShortUploader uploads a clip as a YouTube Short.
//
// Production integration points (not yet implemented):
//   - Initiate a resumable upload session:
//     POST https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable
//     Authorization: Bearer {access_token}
//   - Stream the video file in chunks, updating progress via setUploadProgress.
//   - Retrieve the uploaded video ID from the final 200 response body.
type YouTubeShortUploader struct {
	rdb *redis.Client
}

func (u *YouTubeShortUploader) Upload(ctx context.Context, post ScheduledPost, account SocialAccount, clip Clip) (string, string, error) {
	log.Info().Str("post_id", post.ID).Str("clip_id", clip.ID).Msg("YouTubeShortUploader: starting upload")

	setUploadProgress(ctx, u.rdb, post.ID, 0)

	// TODO: replace with real YouTube Data API v3 resumable upload.
	// Step 1 – create upload session (POST /upload/youtube/v3/videos?uploadType=resumable)
	// Step 2 – upload video bytes in chunks, calling setUploadProgress after each chunk.
	// Step 3 – parse the video ID from the final response.

	setUploadProgress(ctx, u.rdb, post.ID, 25)
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	setUploadProgress(ctx, u.rdb, post.ID, 75)

	platformPostID := fmt.Sprintf("yt_%s_%d", idPrefix(post.ID, 8), time.Now().UnixNano())
	platformPostURL := fmt.Sprintf("https://www.youtube.com/shorts/%s", platformPostID)

	setUploadProgress(ctx, u.rdb, post.ID, 100)
	log.Info().
		Str("post_id", post.ID).
		Str("platform_post_id", platformPostID).
		Msg("YouTubeShortUploader: upload complete")

	return platformPostID, platformPostURL, nil
}

// ---------------------------------------------------------------------------
// Instagram Reels uploader
// ---------------------------------------------------------------------------

// InstagramReelUploader uploads a clip as an Instagram Reel.
//
// Production integration points (not yet implemented):
//   - Create a container:
//     POST https://graph.instagram.com/v18.0/{ig-user-id}/media
//     with video_url pointing to a publicly accessible file (or use chunked upload).
//   - Wait for container status to become FINISHED (poll /media?fields=status_code).
//   - Publish the container:
//     POST https://graph.instagram.com/v18.0/{ig-user-id}/media_publish
//   - Return the media ID as the platformPostID.
type InstagramReelUploader struct {
	rdb *redis.Client
}

func (u *InstagramReelUploader) Upload(ctx context.Context, post ScheduledPost, account SocialAccount, clip Clip) (string, string, error) {
	log.Info().Str("post_id", post.ID).Str("clip_id", clip.ID).Msg("InstagramReelUploader: starting upload")

	setUploadProgress(ctx, u.rdb, post.ID, 0)

	// TODO: replace with real Instagram Graph API media upload flow.

	setUploadProgress(ctx, u.rdb, post.ID, 50)
	if err := ctx.Err(); err != nil {
		return "", "", err
	}

	platformPostID := fmt.Sprintf("ig_%s_%d", idPrefix(post.ID, 8), time.Now().UnixNano())
	platformPostURL := fmt.Sprintf("https://www.instagram.com/reel/%s", platformPostID)

	setUploadProgress(ctx, u.rdb, post.ID, 100)
	log.Info().
		Str("post_id", post.ID).
		Str("platform_post_id", platformPostID).
		Msg("InstagramReelUploader: upload complete")

	return platformPostID, platformPostURL, nil
}

// ---------------------------------------------------------------------------
// TikTok uploader
// ---------------------------------------------------------------------------

// TikTokUploader uploads a clip to TikTok using the Direct Post API.
//
// Production integration points (not yet implemented):
//   - Initialise upload:
//     POST https://open.tiktokapis.com/v2/post/publish/video/init/
//     Authorization: Bearer {access_token}
//   - Upload video file in chunks to the upload URL returned above,
//     calling setUploadProgress after each chunk.
//   - Complete publish:
//     POST https://open.tiktokapis.com/v2/post/publish/video/init/ (finalize)
//   - Return the publish_id as the platformPostID.
type TikTokUploader struct {
	rdb *redis.Client
}

func (u *TikTokUploader) Upload(ctx context.Context, post ScheduledPost, account SocialAccount, clip Clip) (string, string, error) {
	log.Info().Str("post_id", post.ID).Str("clip_id", clip.ID).Msg("TikTokUploader: starting upload")

	setUploadProgress(ctx, u.rdb, post.ID, 0)

	// TODO: replace with real TikTok Direct Post API upload flow.

	setUploadProgress(ctx, u.rdb, post.ID, 33)
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	setUploadProgress(ctx, u.rdb, post.ID, 66)

	platformPostID := fmt.Sprintf("tt_%s_%d", idPrefix(post.ID, 8), time.Now().UnixNano())
	platformPostURL := fmt.Sprintf("https://www.tiktok.com/@user/video/%s", platformPostID)

	setUploadProgress(ctx, u.rdb, post.ID, 100)
	log.Info().
		Str("post_id", post.ID).
		Str("platform_post_id", platformPostID).
		Msg("TikTokUploader: upload complete")

	return platformPostID, platformPostURL, nil
}
