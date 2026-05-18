package storage

import (
	"context"
	"fmt"

	"github.com/pindoyono/viralclip-ai/apps/api/internal/config"
)

// NewStorageService constructs and returns the appropriate StorageService
// implementation based on the application configuration.
//
// When cfg.Storage.Provider is "google_drive" (case-insensitive) and the
// required Drive credentials are present, a GoogleDriveStorageService is
// returned. In all other cases a LocalStorageService is returned.
func NewStorageService(ctx context.Context, cfg *config.Config) (StorageService, error) {
	if cfg.Storage.Provider == "google_drive" {
		driveCfg := GoogleDriveConfig{
			ClientID:     cfg.Storage.GoogleDriveClientID,
			ClientSecret: cfg.Storage.GoogleDriveClientSecret,
			RefreshToken: cfg.Storage.GoogleDriveRefreshToken,
			RootFolderID: cfg.Storage.GoogleDriveFolderID,
		}

		svc, err := NewGoogleDriveStorageService(ctx, driveCfg)
		if err != nil {
			return nil, fmt.Errorf("storage factory: google drive: %w", err)
		}

		return svc, nil
	}

	return NewLocalStorageService(cfg.Storage.LocalPath, cfg.Storage.LocalURL), nil
}
