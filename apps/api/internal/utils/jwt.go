package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTManager handles JWT token operations.
type JWTManager struct {
	secret           string
	expiresIn        time.Duration
	refreshExpiresIn time.Duration
	issuer           string
}

// NewJWTManager creates a new JWTManager.
func NewJWTManager(secret string, expiresIn, refreshExpiresIn time.Duration, issuer string) *JWTManager {
	return &JWTManager{
		secret:           secret,
		expiresIn:        expiresIn,
		refreshExpiresIn: refreshExpiresIn,
		issuer:           issuer,
	}
}

// GenerateAccessToken creates a signed JWT access token.
func (j *JWTManager) GenerateAccessToken(userID uuid.UUID, email, tier string) (string, time.Time, error) {
	expiresAt := time.Now().Add(j.expiresIn)
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"email":   email,
		"tier":    tier,
		"iss":     j.issuer,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(j.secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing token: %w", err)
	}
	return signed, expiresAt, nil
}

// GenerateRefreshToken creates an opaque refresh token.
func (j *JWTManager) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateResetToken creates a password reset token.
func GenerateResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating reset token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
