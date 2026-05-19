package workers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTokenRefreshDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SocialAccount{}))
	return db
}

func TestNewTokenRefreshService(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	assert.NotNil(t, svc)
}

func TestTokenRefreshService_RefreshExpiringTokens_NoAccounts(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	// Should not panic when there are no accounts.
	assert.NotPanics(t, func() {
		svc.RefreshExpiringTokens(context.Background())
	})
}

func TestTokenRefreshService_RefreshesExpiredToken(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	// Token that already expired 1 minute ago.
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-expired",
		UserID:         "user-1",
		Platform:       "tiktok",
		AccessToken:    "old-token",
		RefreshToken:   "refresh-tiktok",
		TokenExpiresAt: &expiredAt,
		IsActive:       true,
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-expired").Error)
	assert.NotEqual(t, "old-token", account.AccessToken)
	assert.Contains(t, account.AccessToken, "refreshed_")
	assert.Equal(t, 0, account.TokenRefreshAttempts)
	assert.True(t, account.TokenExpiresAt.After(time.Now()))
}

func TestTokenRefreshService_RefreshesTokenExpiringWithinWindow(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	// Token expiring in 10 minutes — well within the 15-minute window.
	soonExpiry := time.Now().UTC().Add(10 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-soon",
		UserID:         "user-1",
		Platform:       "youtube",
		AccessToken:    "token-soon",
		RefreshToken:   "refresh-youtube",
		TokenExpiresAt: &soonExpiry,
		IsActive:       true,
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-soon").Error)
	assert.NotEqual(t, "token-soon", account.AccessToken)
	assert.Contains(t, account.AccessToken, "refreshed_")
}

func TestTokenRefreshService_SkipsTokenNotNearExpiry(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	// Token still valid for 2 hours — outside the refresh window.
	validExpiry := time.Now().UTC().Add(2 * time.Hour)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-fresh",
		UserID:         "user-1",
		Platform:       "instagram",
		AccessToken:    "fresh-token",
		RefreshToken:   "refresh-insta",
		TokenExpiresAt: &validExpiry,
		IsActive:       true,
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-fresh").Error)
	// Token must remain unchanged.
	assert.Equal(t, "fresh-token", account.AccessToken)
}

func TestTokenRefreshService_SkipsInactiveAccounts(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-inactive",
		UserID:         "user-1",
		Platform:       "tiktok",
		AccessToken:    "inactive-token",
		RefreshToken:   "refresh-inactive",
		TokenExpiresAt: &expiredAt,
		IsActive:       false, // inactive — must not be refreshed
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-inactive").Error)
	assert.Equal(t, "inactive-token", account.AccessToken)
}

func TestTokenRefreshService_SkipsAccountWithNoRefreshToken(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-no-refresh",
		UserID:         "user-1",
		Platform:       "tiktok",
		AccessToken:    "token-no-refresh",
		RefreshToken:   "", // empty — cannot refresh
		TokenExpiresAt: &expiredAt,
		IsActive:       true,
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	// Token should remain unchanged, but this particular case is handled
	// inside refreshToken (notifyFailure is called instead).
	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-no-refresh").Error)
	assert.Equal(t, "token-no-refresh", account.AccessToken)
}

func TestTokenRefreshService_NilRefreshToken_DoesNotIncrement(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:                   "acc-empty-refresh",
		UserID:               "user-1",
		Platform:             "youtube",
		AccessToken:          "token-empty",
		RefreshToken:         "",
		TokenExpiresAt:       &expiredAt,
		IsActive:             true,
		TokenRefreshAttempts: 0,
	}).Error)

	// The account has an empty refresh_token, so the WHERE clause in
	// RefreshExpiringTokens filters it out entirely — attempts counter
	// should remain 0.
	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-empty-refresh").Error)
	assert.Equal(t, 0, account.TokenRefreshAttempts)
}

func TestTokenRefreshService_HandlesNilExpiry(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	// expires_at IS NULL — treated as expired and should be refreshed.
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-nil-expiry",
		UserID:         "user-1",
		Platform:       "instagram",
		AccessToken:    "token-nil",
		RefreshToken:   "refresh-nil",
		TokenExpiresAt: nil,
		IsActive:       true,
	}).Error)

	svc.RefreshExpiringTokens(context.Background())

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-nil-expiry").Error)
	assert.NotEqual(t, "token-nil", account.AccessToken)
	assert.Contains(t, account.AccessToken, "refreshed_")
}

func TestTokenRefreshService_CancelledContext(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("acc-cancel-%d", i)
		require.NoError(t, db.Create(&SocialAccount{
			ID:             id,
			UserID:         "user-1",
			Platform:       "tiktok",
			AccessToken:    "token-cancel",
			RefreshToken:   "refresh-cancel",
			TokenExpiresAt: &expiredAt,
			IsActive:       true,
		}).Error)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Should return without panic.
	assert.NotPanics(t, func() {
		svc.RefreshExpiringTokens(ctx)
	})
}

func TestTokenRefreshService_RefreshAccountToken_UpdatesDBAndInMemoryAccount(t *testing.T) {
	db := setupTokenRefreshDB(t)
	svc := NewTokenRefreshService(db, nil)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&SocialAccount{
		ID:             "acc-single-refresh",
		UserID:         "user-1",
		Platform:       "youtube",
		AccessToken:    "old-token",
		RefreshToken:   "refresh-single",
		TokenExpiresAt: &expiredAt,
		IsActive:       true,
	}).Error)

	var account SocialAccount
	require.NoError(t, db.First(&account, "id = ?", "acc-single-refresh").Error)

	require.NoError(t, svc.RefreshAccountToken(context.Background(), &account))

	assert.Contains(t, account.AccessToken, "refreshed_")
	assert.NotNil(t, account.TokenExpiresAt)
	assert.True(t, account.TokenExpiresAt.After(time.Now()))

	var dbAccount SocialAccount
	require.NoError(t, db.First(&dbAccount, "id = ?", "acc-single-refresh").Error)
	assert.Equal(t, account.AccessToken, dbAccount.AccessToken)
	assert.Equal(t, 0, dbAccount.TokenRefreshAttempts)
}
