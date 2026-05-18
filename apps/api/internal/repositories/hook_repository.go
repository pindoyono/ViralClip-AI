// Package repositories provides data-access abstractions for the ViralClip API.
//
// Each repository defines an interface (for testability / DI) plus a GORM-
// backed implementation.  Handlers receive only the interface so they remain
// independent of the persistence layer.
package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/models"
)

// HookRepository defines the data-access contract for hook detections.
type HookRepository interface {
	// Save persists a slice of hook detections for a video/user pair.
	// Existing detections for the same video are replaced atomically.
	Save(ctx context.Context, videoID, userID uuid.UUID, hooks []models.HookDetection) error

	// FindByVideo returns all hook detections for the given video,
	// ordered by score descending.
	FindByVideo(ctx context.Context, videoID uuid.UUID) ([]models.HookDetection, error)

	// FindByVideoAndType returns detections filtered by hook type.
	FindByVideoAndType(ctx context.Context, videoID uuid.UUID, hookType string) ([]models.HookDetection, error)

	// DeleteByVideo removes all hook detections for a video.
	DeleteByVideo(ctx context.Context, videoID uuid.UUID) error
}

// gormHookRepository is the GORM-backed implementation of HookRepository.
type gormHookRepository struct {
	db *gorm.DB
}

// NewHookRepository creates a new GORM-backed HookRepository.
func NewHookRepository(db *gorm.DB) HookRepository {
	return &gormHookRepository{db: db}
}

// Save replaces all hook detections for a video within a single transaction.
func (r *gormHookRepository) Save(
	ctx context.Context,
	videoID, userID uuid.UUID,
	hooks []models.HookDetection,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Remove previous detections for this video so we never accumulate stale rows.
		if err := tx.Where("video_id = ?", videoID).Delete(&models.HookDetection{}).Error; err != nil {
			log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to delete previous hook detections")
			return err
		}

		if len(hooks) == 0 {
			return nil
		}

		// Stamp video/user IDs on every row before bulk-inserting.
		for i := range hooks {
			hooks[i].VideoID = videoID
			hooks[i].UserID = userID
		}

		if err := tx.Create(&hooks).Error; err != nil {
			log.Error().Err(err).Str("video_id", videoID.String()).Msg("Failed to save hook detections")
			return err
		}

		log.Info().
			Str("video_id", videoID.String()).
			Int("count", len(hooks)).
			Msg("Hook detections saved")
		return nil
	})
}

// FindByVideo returns all detections for a video ordered by score descending.
func (r *gormHookRepository) FindByVideo(
	ctx context.Context,
	videoID uuid.UUID,
) ([]models.HookDetection, error) {
	var detections []models.HookDetection
	if err := r.db.WithContext(ctx).
		Where("video_id = ?", videoID).
		Order("score DESC").
		Find(&detections).Error; err != nil {
		return nil, err
	}
	return detections, nil
}

// FindByVideoAndType returns detections for a video filtered by hook type.
func (r *gormHookRepository) FindByVideoAndType(
	ctx context.Context,
	videoID uuid.UUID,
	hookType string,
) ([]models.HookDetection, error) {
	var detections []models.HookDetection
	if err := r.db.WithContext(ctx).
		Where("video_id = ? AND hook_type = ?", videoID, hookType).
		Order("score DESC").
		Find(&detections).Error; err != nil {
		return nil, err
	}
	return detections, nil
}

// DeleteByVideo removes all hook detections for a video.
func (r *gormHookRepository) DeleteByVideo(ctx context.Context, videoID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("video_id = ?", videoID).
		Delete(&models.HookDetection{}).Error
}
