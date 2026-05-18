package repositories_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
	"github.com/pindoyono/viralclip-ai/apps/api/internal/repositories"
)

// newTestDB opens an in-memory SQLite database and auto-migrates the models
// needed for the repository tests.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&models.User{},
		&models.Video{},
		&models.HookDetection{},
	))
	return db
}

// seed helpers

func seedUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{
		Email:        "test@example.com",
		PasswordHash: "hash",
		Name:         "Test User",
		IsActive:     true,
	}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func seedVideo(t *testing.T, db *gorm.DB, userID uuid.UUID) models.Video {
	t.Helper()
	v := models.Video{
		UserID:           userID,
		Title:            "Test Video",
		OriginalFilename: "test.mp4",
	}
	require.NoError(t, db.Create(&v).Error)
	return v
}

func buildDetections(videoID, userID uuid.UUID, count int) []models.HookDetection {
	hookTypes := []string{"curiosity", "emotion", "storytelling", "controversy", "cta"}
	hooks := make([]models.HookDetection, count)
	for i := range hooks {
		hooks[i] = models.HookDetection{
			VideoID:        videoID,
			UserID:         userID,
			Start:          float64(i * 5),
			End:            float64(i*5 + 4),
			HookType:       hookTypes[i%len(hookTypes)],
			Score:          50 + i,
			MatchedPattern: "test pattern",
		}
	}
	return hooks
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHookRepository_Save_And_FindByVideo(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	hooks := buildDetections(v.ID, u.ID, 3)
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, hooks))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

func TestHookRepository_Save_ReplacesExisting(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	// First save: 3 hooks
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, buildDetections(v.ID, u.ID, 3)))

	// Second save: 2 hooks – should replace the first batch
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, buildDetections(v.ID, u.ID, 2)))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	assert.Len(t, got, 2, "second Save should replace first batch")
}

func TestHookRepository_Save_EmptyHooks_ClearsExisting(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	require.NoError(t, repo.Save(ctx, v.ID, u.ID, buildDetections(v.ID, u.ID, 3)))
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, []models.HookDetection{}))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHookRepository_FindByVideo_OrderedByScoreDesc(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	hooks := buildDetections(v.ID, u.ID, 5) // scores 50,51,52,53,54
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, hooks))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	require.Len(t, got, 5)

	for i := 1; i < len(got); i++ {
		assert.GreaterOrEqual(t, got[i-1].Score, got[i].Score,
			"results should be sorted by score descending")
	}
}

func TestHookRepository_FindByVideoAndType(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	hooks := buildDetections(v.ID, u.ID, 5) // types cycle: curiosity,emotion,storytelling,controversy,cta
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, hooks))

	got, err := repo.FindByVideoAndType(ctx, v.ID, "curiosity")
	require.NoError(t, err)
	assert.NotEmpty(t, got)
	for _, h := range got {
		assert.Equal(t, "curiosity", h.HookType)
	}
}

func TestHookRepository_FindByVideoAndType_NoMatch(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	hooks := buildDetections(v.ID, u.ID, 1) // only curiosity
	hooks[0].HookType = "curiosity"
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, hooks))

	got, err := repo.FindByVideoAndType(ctx, v.ID, "cta")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHookRepository_DeleteByVideo(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	require.NoError(t, repo.Save(ctx, v.ID, u.ID, buildDetections(v.ID, u.ID, 4)))
	require.NoError(t, repo.DeleteByVideo(ctx, v.ID))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHookRepository_FindByVideo_UnknownVideo(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	got, err := repo.FindByVideo(ctx, uuid.New())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestHookRepository_VideoIDStamped(t *testing.T) {
	db := newTestDB(t)
	repo := repositories.NewHookRepository(db)
	ctx := context.Background()

	u := seedUser(t, db)
	v := seedVideo(t, db, u.ID)

	hooks := []models.HookDetection{
		{Start: 0, End: 5, HookType: "curiosity", Score: 80, MatchedPattern: "secret"},
	}
	require.NoError(t, repo.Save(ctx, v.ID, u.ID, hooks))

	got, err := repo.FindByVideo(ctx, v.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, v.ID, got[0].VideoID)
	assert.Equal(t, u.ID, got[0].UserID)
}
