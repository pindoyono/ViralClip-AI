package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword_Success(t *testing.T) {
	hash, err := HashPassword("securepassword123")

	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	// bcrypt hashes start with $2a$ or $2b$
	assert.True(t, strings.HasPrefix(hash, "$2"))
}

func TestHashPassword_DifferentForSameInput(t *testing.T) {
	hash1, err := HashPassword("mypassword")
	require.NoError(t, err)
	hash2, err := HashPassword("mypassword")
	require.NoError(t, err)

	// bcrypt generates a different salt each time
	assert.NotEqual(t, hash1, hash2)
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")

	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestCheckPasswordHash_CorrectPassword(t *testing.T) {
	password := "correct-horse-battery-staple"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	result := CheckPasswordHash(password, hash)

	assert.True(t, result)
}

func TestCheckPasswordHash_WrongPassword(t *testing.T) {
	hash, err := HashPassword("original")
	require.NoError(t, err)

	result := CheckPasswordHash("different", hash)

	assert.False(t, result)
}

func TestCheckPasswordHash_EmptyPasswordAgainstHash(t *testing.T) {
	hash, err := HashPassword("nonempty")
	require.NoError(t, err)

	result := CheckPasswordHash("", hash)

	assert.False(t, result)
}

func TestCheckPasswordHash_InvalidHash(t *testing.T) {
	result := CheckPasswordHash("password", "notahash")

	assert.False(t, result)
}

func TestCheckPasswordHash_RoundTrip(t *testing.T) {
	testCases := []string{
		"short",
		"a-much-longer-password-with-special-chars-!@#$%",
		"unicode-パスワード",
		"1234567890abcdefghij",
	}

	for _, password := range testCases {
		t.Run(password, func(t *testing.T) {
			hash, err := HashPassword(password)
			require.NoError(t, err)
			assert.True(t, CheckPasswordHash(password, hash))
			assert.False(t, CheckPasswordHash(password+"x", hash))
		})
	}
}
