package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
//
// For files whose size is known and exceeds 32 MiB, Upload automatically uses
// the Drive resumable upload protocol (RFC-compliant chunked PUT), which
// supports files larger than 2 GiB without buffering the entire payload.
// Progress can be tracked via GetUploadProgress when UploadOptions.UploadID
// is set, making GoogleDriveStorageService a ResumableStorageService.
type GoogleDriveStorageService struct {
	cfg    GoogleDriveConfig
	svc    *drive.Service
	// folderCache caches resolved Drive folder IDs to minimise API round-trips.
	folderCache map[string]string
	// resumable handles large-file uploads via the Drive resumable protocol.
	resumable *ResumableUploadService
}

// NewGoogleDriveStorageServiceWithClient creates a GoogleDriveStorageService
// using an already-configured *drive.Service and optional ResumableUploadConfig.
//
// This constructor is intended for unit tests where the Drive client is wired
// to a mock HTTP server.  Pass an *http.Client that routes to the same mock
// server so resumable uploads are also intercepted.
func NewGoogleDriveStorageServiceWithClient(svc *drive.Service, httpClient *http.Client, cfg ...ResumableUploadConfig) *GoogleDriveStorageService {
	var resCfg ResumableUploadConfig
	if len(cfg) > 0 {
		resCfg = cfg[0]
	}

	tracker := NewUploadProgressTracker()
	var resumable *ResumableUploadService
	if httpClient != nil {
		resumable = NewResumableUploadService(httpClient, tracker, resCfg)
	}

	return &GoogleDriveStorageService{
		svc:         svc,
		folderCache: make(map[string]string),
		resumable:   resumable,
	}
}

// NewGoogleDriveStorageService creates and authenticates a new
// GoogleDriveStorageService using the supplied OAuth2 refresh-token credentials.
func NewGoogleDriveStorageService(ctx context.Context, cfg GoogleDriveConfig) (*GoogleDriveStorageService, error) {
	return NewGoogleDriveStorageServiceWithResumableConfig(ctx, cfg, ResumableUploadConfig{})
}

// NewGoogleDriveStorageServiceWithResumableConfig is like NewGoogleDriveStorageService
// but allows callers to supply a custom ResumableUploadConfig.
func NewGoogleDriveStorageServiceWithResumableConfig(
	ctx context.Context,
	cfg GoogleDriveConfig,
	resCfg ResumableUploadConfig,
) (*GoogleDriveStorageService, error) {
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

	// Use the same authenticated HTTP client for both the Drive service and
	// the ResumableUploadService so token refresh is handled uniformly.
	httpClient := oauth2.NewClient(ctx, tokenSource)

	svc, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("google drive storage: create Drive client: %w", err)
	}

	tracker := NewUploadProgressTracker()
	resumable := NewResumableUploadService(httpClient, tracker, resCfg)

	return &GoogleDriveStorageService{
		cfg:         cfg,
		svc:         svc,
		folderCache: make(map[string]string),
		resumable:   resumable,
	}, nil
}

// Upload uploads the content of r to Google Drive, placing it in the folder
// derived from opts.UserID and opts.Folder.
//
// When opts.FileSize > resumableThreshold (32 MiB) the upload uses the Drive
// resumable protocol, which supports files larger than 2 GiB.  Set
// opts.UploadID to track progress via GetUploadProgress.
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

	// Use resumable upload for large files when the ResumableUploadService is
	// available and the file size is known and above the threshold.
	if s.resumable != nil && opts.FileSize >= resumableThreshold {
		return s.uploadResumable(ctx, folderID, filename, contentType, r, opts)
	}

	return s.uploadSimple(ctx, folderID, filename, contentType, r)
}

// uploadSimple uses the standard Drive Files.Create call (suitable for small files).
func (s *GoogleDriveStorageService) uploadSimple(
	ctx context.Context,
	folderID, filename, contentType string,
	r io.Reader,
) (*FileInfo, error) {
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

// uploadResumable delegates to ResumableUploadService for large files.
func (s *GoogleDriveStorageService) uploadResumable(
	ctx context.Context,
	folderID, filename, contentType string,
	r io.Reader,
	opts UploadOptions,
) (*FileInfo, error) {
	fileID, err := s.resumable.UploadFile(ctx, folderID, filename, contentType, r, opts.FileSize, opts.UploadID)
	if err != nil {
		return nil, fmt.Errorf("google drive storage: resumable upload %q: %w", filename, err)
	}

	// Retrieve the file metadata to populate FileInfo.
	f, err := s.svc.Files.Get(fileID).
		Fields("id,name,size,createdTime,mimeType").
		Context(ctx).
		Do()
	if err != nil || f.Id == "" {
		// Non-fatal: return partial info rather than failing the whole upload.
		return &FileInfo{
			Key:         fileID,
			URL:         fmt.Sprintf(googleDriveViewURLBase, fileID),
			Size:        opts.FileSize,
			ContentType: contentType,
			CreatedAt:   time.Now().UTC(),
		}, nil
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

// GetUploadProgress returns the current progress for the given uploadID.
// Returns (zero, false) when no upload with that ID is tracked.
func (s *GoogleDriveStorageService) GetUploadProgress(uploadID string) (UploadProgress, bool) {
	if s.resumable == nil {
		return UploadProgress{}, false
	}
	return s.resumable.tracker.Get(uploadID)
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

// Compile-time proof that GoogleDriveStorageService satisfies both interfaces.
var _ StorageService = (*GoogleDriveStorageService)(nil)
var _ ResumableStorageService = (*GoogleDriveStorageService)(nil)
