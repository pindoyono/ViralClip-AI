package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	// rootFolderName is the top-level Google Drive folder created under the
	// account associated with the OAuth credentials.
	rootFolderName = "ViralClipAI"

	// driveFileMimeType is the Google Drive MIME type for folders.
	driveFolderMimeType = "application/vnd.google-apps.folder"

	// googleDriveDownloadURLBase is the base URL used to construct direct
	// download links (requires the file to be publicly shared or downloaded
	// via the API with proper auth, used as informational URL).
	googleDriveViewURLBase = "https://drive.google.com/file/d/%s/view"
)

// GoogleDriveConfig holds the OAuth2 credentials required to access the
// Google Drive API using an offline refresh-token flow.
type GoogleDriveConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	// RootFolderID is the Drive folder ID that should be used as the parent
	// for the "ViralClipAI" folder tree. An empty string means the user's
	// root "My Drive".
	RootFolderID string
}

// GoogleDriveStorageService uploads and manages files on Google Drive.
//
// It is designed for media assets (source videos, generated clips and export
// files) that benefit from cloud storage. Files are organised in a folder tree:
//
//	ViralClipAI/
//	  Users/
//	    {userID}/
//	      uploads/
//	      clips/
//	      exports/
type GoogleDriveStorageService struct {
	cfg    GoogleDriveConfig
	svc    *drive.Service
	// folderCache caches resolved Drive folder IDs to minimise API round-trips.
	folderCache map[string]string
}

// NewGoogleDriveStorageServiceWithClient creates a GoogleDriveStorageService
// using an already-configured *drive.Service. This constructor is primarily
// intended for unit tests where the Drive client is wired to a mock HTTP
// server.
func NewGoogleDriveStorageServiceWithClient(svc *drive.Service) *GoogleDriveStorageService {
	return &GoogleDriveStorageService{
		svc:         svc,
		folderCache: make(map[string]string),
	}
}
// NewGoogleDriveStorageService creates and authenticates a new
// GoogleDriveStorageService using the supplied OAuth2 refresh-token credentials.
func NewGoogleDriveStorageService(ctx context.Context, cfg GoogleDriveConfig) (*GoogleDriveStorageService, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil, fmt.Errorf("google drive storage: ClientID, ClientSecret and RefreshToken are required")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
	}

	token := &oauth2.Token{RefreshToken: cfg.RefreshToken}
	tokenSource := oauthCfg.TokenSource(ctx, token)

	svc, err := drive.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, fmt.Errorf("google drive storage: create Drive client: %w", err)
	}

	return &GoogleDriveStorageService{
		cfg:         cfg,
		svc:         svc,
		folderCache: make(map[string]string),
	}, nil
}

// Upload uploads the content of r to Google Drive, placing it in the folder
// derived from opts.UserID and opts.Folder.
//
// The key parameter is used only as a fallback filename when opts.Filename is
// empty. The returned FileInfo.Key contains the Drive file ID, which must be
// passed to Download, Delete and GetURL.
func (s *GoogleDriveStorageService) Upload(ctx context.Context, key string, r io.Reader, opts UploadOptions) (*FileInfo, error) {
	folderID, err := s.resolveFolder(ctx, opts.UserID, opts.Folder)
	if err != nil {
		return nil, fmt.Errorf("google drive storage: resolve folder: %w", err)
	}

	filename := opts.Filename
	if filename == "" {
		parts := strings.Split(key, "/")
		filename = parts[len(parts)-1]
	}

	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	fileMeta := &drive.File{
		Name:    filename,
		Parents: []string{folderID},
	}

	f, err := s.svc.Files.Create(fileMeta).
		Media(r).
		Fields("id,name,size,createdTime,mimeType").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("google drive storage: upload %q: %w", filename, err)
	}

	createdAt, _ := time.Parse(time.RFC3339, f.CreatedTime)

	return &FileInfo{
		Key:         f.Id,
		URL:         fmt.Sprintf(googleDriveViewURLBase, f.Id),
		Size:        f.Size,
		ContentType: f.MimeType,
		CreatedAt:   createdAt,
	}, nil
}

// Download retrieves the file identified by the Drive file ID (key) and
// returns its content as a ReadCloser.
func (s *GoogleDriveStorageService) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.svc.Files.Get(key).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("google drive storage: download %q: %w", key, err)
	}

	return resp.Body, nil
}

// Delete permanently deletes the Drive file identified by key (Drive file ID).
func (s *GoogleDriveStorageService) Delete(ctx context.Context, key string) error {
	if err := s.svc.Files.Delete(key).Context(ctx).Do(); err != nil {
		return fmt.Errorf("google drive storage: delete %q: %w", key, err)
	}

	return nil
}

// GetURL returns the Google Drive view URL for the file identified by key.
func (s *GoogleDriveStorageService) GetURL(_ context.Context, key string) (string, error) {
	return fmt.Sprintf(googleDriveViewURLBase, key), nil
}

// ---------------------------------------------------------------------------
// Internal folder resolution helpers
// ---------------------------------------------------------------------------

// resolveFolder returns (creating if necessary) the Drive folder ID for the
// path ViralClipAI/Users/{userID}/{subFolder}. Resolved IDs are cached in
// memory to avoid redundant API calls within the same process lifetime.
func (s *GoogleDriveStorageService) resolveFolder(ctx context.Context, userID, subFolder string) (string, error) {
	// Build a deterministic cache key for this path.
	cacheKey := fmt.Sprintf("%s/%s", userID, subFolder)
	if id, ok := s.folderCache[cacheKey]; ok {
		return id, nil
	}

	// Walk / create the folder hierarchy:
	//   root (My Drive or cfg.RootFolderID)
	//     └─ ViralClipAI
	//          └─ Users
	//               └─ {userID}
	//                    └─ {subFolder}
	parentID := s.cfg.RootFolderID

	segments := []string{rootFolderName, "Users", userID}
	if subFolder != "" {
		segments = append(segments, subFolder)
	}

	for _, seg := range segments {
		id, err := s.ensureFolder(ctx, seg, parentID)
		if err != nil {
			return "", fmt.Errorf("ensure folder %q: %w", seg, err)
		}
		parentID = id
	}

	s.folderCache[cacheKey] = parentID
	return parentID, nil
}

// ensureFolder returns the Drive folder ID of a child named name inside
// parentID, creating it when it does not exist.
func (s *GoogleDriveStorageService) ensureFolder(ctx context.Context, name, parentID string) (string, error) {
	// Determine the parent query clause.
	parentClause := "'root' in parents"
	if parentID != "" {
		parentClause = fmt.Sprintf("'%s' in parents", parentID)
	}

	q := fmt.Sprintf(
		"mimeType='%s' and name='%s' and %s and trashed=false",
		driveFolderMimeType, name, parentClause,
	)

	list, err := s.svc.Files.List().
		Q(q).
		Fields("files(id,name)").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("list folders: %w", err)
	}

	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}

	// Folder does not exist – create it.
	meta := &drive.File{
		Name:     name,
		MimeType: driveFolderMimeType,
	}
	if parentID != "" {
		meta.Parents = []string{parentID}
	}

	folder, err := s.svc.Files.Create(meta).Fields("id").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("create folder %q: %w", name, err)
	}

	return folder.Id, nil
}
