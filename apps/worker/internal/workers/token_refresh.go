package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	// tokenRefreshWindow is how far ahead of expiry we proactively refresh tokens.
	// Refreshing 15 minutes early gives the PublishingWorker fresh tokens to use.
	tokenRefreshWindow = 15 * time.Minute

	// tokenRefreshFailureChannel is the Redis Pub/Sub channel where refresh
	// failures are published so monitoring dashboards and alerting can react.
	tokenRefreshFailureChannel = "token:refresh:failures"
)

// tokenRefreshFailureEvent is the payload published to Redis on refresh failure.
type tokenRefreshFailureEvent struct {
	AccountID string `json:"account_id"`
	Platform  string `json:"platform"`
	Reason    string `json:"reason"`
	Timestamp string `json:"ts"`
}

// TokenRefreshService proactively refreshes social-account OAuth tokens that
// are about to expire.  It is designed to run on a periodic timer (e.g. every
// 15 minutes) so that the PublishingWorker always finds a valid access token
// and never stalls waiting for an inline refresh.
//
// Responsibilities:
//  1. Query all active accounts whose tokens expire within tokenRefreshWindow.
//  2. Call the appropriate platform OAuth endpoint to obtain a new token pair.
//  3. Persist the new access_token and updated expires_at to the database.
//  4. On failure: log the error, increment token_refresh_attempts, and publish
//     a notification to the tokenRefreshFailureChannel Redis Pub/Sub channel.
type TokenRefreshService struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewTokenRefreshService creates a new TokenRefreshService.
func NewTokenRefreshService(db *gorm.DB, rdb *redis.Client) *TokenRefreshService {
	return &TokenRefreshService{db: db, redis: rdb}
}

// RefreshExpiringTokens queries all active social accounts whose access tokens
// expire within tokenRefreshWindow and attempts to refresh each of them.
// Safe to call concurrently; each account is refreshed independently.
func (s *TokenRefreshService) RefreshExpiringTokens(ctx context.Context) {
	log.Info().Msg("TokenRefreshService: scanning for expiring tokens")

	deadline := time.Now().UTC().Add(tokenRefreshWindow)

	var accounts []SocialAccount
	if err := s.db.WithContext(ctx).
		Where(
			"is_active = ? AND refresh_token != '' AND (expires_at IS NULL OR expires_at < ?)",
			true, deadline,
		).
		Limit(100).
		Find(&accounts).Error; err != nil {
		log.Error().Err(err).Msg("TokenRefreshService: failed to query expiring accounts")
		return
	}

	if len(accounts) == 0 {
		log.Debug().Msg("TokenRefreshService: no expiring tokens found")
		return
	}

	log.Info().Int("count", len(accounts)).Msg("TokenRefreshService: refreshing expiring tokens")

	for i := range accounts {
		select {
		case <-ctx.Done():
			return
		default:
			if err := s.refreshToken(ctx, &accounts[i]); err != nil {
				log.Warn().
					Err(err).
					Str("account_id", accounts[i].ID).
					Str("platform", accounts[i].Platform).
					Msg("TokenRefreshService: failed refreshing expiring token")
			}
		}
	}
}

// EnsureValidAccessToken guarantees the given account has a usable access token
// for immediate upload. If the token is expired (or near expiry), it refreshes
// the token, persists it in DB, and updates the account object in-place.
func (s *TokenRefreshService) EnsureValidAccessToken(ctx context.Context, account *SocialAccount) error {
	if account.AccessToken == "" {
		return fmt.Errorf("missing access token")
	}
	if account.TokenExpiresAt == nil || account.TokenExpiresAt.After(time.Now().UTC().Add(30*time.Second)) {
		return nil
	}

	if err := s.refreshToken(ctx, account); err != nil {
		return fmt.Errorf("access token refresh failed: %w", err)
	}
	return nil
}

func (s *TokenRefreshService) refreshToken(ctx context.Context, account *SocialAccount) error {
	if account.RefreshToken == "" {
		s.notifyFailure(ctx, account.ID, account.Platform, "missing refresh_token")
		return fmt.Errorf("missing refresh_token")
	}

	newToken, newExpiry, err := s.callPlatformRefresh(ctx, *account)
	if err != nil {
		log.Error().
			Err(err).
			Str("account_id", account.ID).
			Str("platform", account.Platform).
			Msg("TokenRefreshService: platform refresh call failed")
		s.notifyFailure(ctx, account.ID, account.Platform, err.Error())

		// Increment the retry counter so operators can identify accounts that
		// consistently fail and may need manual intervention.
		if dbErr := s.db.WithContext(ctx).
			Table("social_accounts").
			Where("id = ?", account.ID).
			Updates(map[string]interface{}{
				"token_refresh_attempts": gorm.Expr("COALESCE(token_refresh_attempts, 0) + 1"),
				"updated_at":             time.Now().UTC(),
			}).Error; dbErr != nil {
			log.Error().Err(dbErr).Str("account_id", account.ID).
				Msg("TokenRefreshService: failed to increment token_refresh_attempts")
		}
		return err
	}

	if err := s.db.WithContext(ctx).
		Table("social_accounts").
		Where("id = ?", account.ID).
		Updates(map[string]interface{}{
			"access_token":            newToken,
			"expires_at":              newExpiry,
			"token_refresh_attempts":  0,
			"updated_at":              time.Now().UTC(),
		}).Error; err != nil {
		log.Error().Err(err).Str("account_id", account.ID).
			Msg("TokenRefreshService: failed to persist refreshed token")
		return err
	}

	account.AccessToken = newToken
	account.TokenExpiresAt = &newExpiry
	account.TokenRefreshAttempts = 0

	log.Info().
		Str("account_id", account.ID).
		Str("platform", account.Platform).
		Time("new_expiry", newExpiry).
		Msg("TokenRefreshService: token refreshed successfully")
	return nil
}

// callPlatformRefresh calls the platform's OAuth token refresh endpoint.
//
// TODO: replace each stub with a real HTTPS call:
//   - YouTube:   POST https://oauth2.googleapis.com/token
//     body: grant_type=refresh_token&refresh_token=…&client_id=…&client_secret=…
//   - Instagram: POST https://api.instagram.com/oauth/access_token  (short-lived)
//     or GET https://graph.instagram.com/refresh_access_token  (long-lived)
//   - TikTok:    POST https://open.tiktokapis.com/v2/oauth/token/refresh/
func (s *TokenRefreshService) callPlatformRefresh(_ context.Context, account SocialAccount) (string, time.Time, error) {
	if account.RefreshToken == "" {
		return "", time.Time{}, fmt.Errorf("no refresh_token for account %s", account.ID)
	}

	// Stub: derive a new access token from the refresh token.
	// In production, replace with a real HTTP POST to the platform OAuth endpoint.
	newToken := "refreshed_" + account.RefreshToken + "_" + fmt.Sprintf("%d", time.Now().Unix())
	newExpiry := time.Now().UTC().Add(1 * time.Hour)
	return newToken, newExpiry, nil
}

// notifyFailure logs the failure and publishes a structured event to Redis
// Pub/Sub so that downstream consumers (alerting, dashboards) can react.
func (s *TokenRefreshService) notifyFailure(ctx context.Context, accountID, platform, reason string) {
	log.Warn().
		Str("account_id", accountID).
		Str("platform", platform).
		Str("reason", reason).
		Msg("TokenRefreshService: token refresh failed")

	if s.redis == nil {
		return
	}

	event := tokenRefreshFailureEvent{
		AccountID: accountID,
		Platform:  platform,
		Reason:    reason,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Msg("TokenRefreshService: failed to marshal failure event")
		return
	}

	if err := s.redis.Publish(ctx, tokenRefreshFailureChannel, string(data)).Err(); err != nil {
		log.Error().Err(err).Msg("TokenRefreshService: failed to publish failure notification")
	}
}
