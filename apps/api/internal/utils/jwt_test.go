package utils

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestJWTManager() *JWTManager {
	return NewJWTManager(
		"test-secret-key-at-least-32-chars-long",
		15*time.Minute,
		7*24*time.Hour,
		"viralclip-test",
	)
}

func TestGenerateAccessToken_Success(t *testing.T) {
	mgr := newTestJWTManager()
	userID := uuid.New()
	email := "test@example.com"
	tier := "pro"

	token, expiresAt, err := mgr.GenerateAccessToken(userID, email, tier)

	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now()))
	assert.True(t, expiresAt.Before(time.Now().Add(16*time.Minute)))
}

func TestGenerateAccessToken_ClaimsValid(t *testing.T) {
	mgr := newTestJWTManager()
	userID := uuid.New()
	email := "claims@example.com"
	tier := "free"

	tokenStr, _, err := mgr.GenerateAccessToken(userID, email, tier)
	require.NoError(t, err)

	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte("test-secret-key-at-least-32-chars-long"), nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, userID.String(), claims["user_id"])
	assert.Equal(t, email, claims["email"])
	assert.Equal(t, tier, claims["tier"])
	assert.Equal(t, "viralclip-test", claims["iss"])
}

func TestGenerateAccessToken_DifferentSecretFails(t *testing.T) {
	mgr := newTestJWTManager()
	userID := uuid.New()

	tokenStr, _, err := mgr.GenerateAccessToken(userID, "x@x.com", "free")
	require.NoError(t, err)

	_, err = jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte("wrong-secret"), nil
	})
	assert.Error(t, err)
}

func TestGenerateRefreshToken_Success(t *testing.T) {
	mgr := newTestJWTManager()

	token, err := mgr.GenerateRefreshToken()

	require.NoError(t, err)
	assert.NotEmpty(t, token)
	// 32 bytes hex-encoded = 64 chars
	assert.Len(t, token, 64)
}

func TestGenerateRefreshToken_Unique(t *testing.T) {
	mgr := newTestJWTManager()

	token1, err := mgr.GenerateRefreshToken()
	require.NoError(t, err)
	token2, err := mgr.GenerateRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2)
}

func TestGenerateResetToken_Success(t *testing.T) {
	token, err := GenerateResetToken()

	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Len(t, token, 64)
}

func TestGenerateResetToken_Unique(t *testing.T) {
	t1, err := GenerateResetToken()
	require.NoError(t, err)
	t2, err := GenerateResetToken()
	require.NoError(t, err)

	assert.NotEqual(t, t1, t2)
}

func TestNewJWTManager_FieldsSet(t *testing.T) {
	secret := "my-super-secret"
	accessDur := 30 * time.Minute
	refreshDur := 14 * 24 * time.Hour
	issuer := "my-issuer"

	mgr := NewJWTManager(secret, accessDur, refreshDur, issuer)

	assert.NotNil(t, mgr)
	assert.Equal(t, secret, mgr.secret)
	assert.Equal(t, accessDur, mgr.expiresIn)
	assert.Equal(t, refreshDur, mgr.refreshExpiresIn)
	assert.Equal(t, issuer, mgr.issuer)
}
