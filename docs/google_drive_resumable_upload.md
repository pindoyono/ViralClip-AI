# Google Drive Resumable Upload

## Overview

ViralClip AI automatically uses the [Google Drive Resumable Upload protocol](https://developers.google.com/drive/api/guides/manage-uploads#resumable) for large video files. Files larger than **32 MiB** are uploaded in chunks, enabling:

- **2 GiB+ file support** — no buffering of the entire payload
- **Interrupted upload resume** — a session URI is kept in memory; if a chunk fails, the service queries Drive for the last confirmed byte and continues from there
- **Per-chunk retry** — transient errors (HTTP 408, 429, 5xx, network timeouts) are retried up to 5 times with exponential back-off and ±30 % jitter
- **Progress tracking** — bytes uploaded are exposed through `GetUploadProgress` / `UploadProgress` while the upload is in flight

---

## Architecture

```
VideoHandler.Upload()
  │  opts.FileSize  = file.Size      ← set by handler
  │  opts.UploadID  = videoID        ← used for progress tracking
  ↓
GoogleDriveStorageService.Upload()
  │  FileSize ≥ 32 MiB?
  ├─ NO  → uploadSimple()            (drive.Files.Create.Media – buffered)
  └─ YES → uploadResumable()
              │
              ↓
         ResumableUploadService.UploadFile()
              │
              ├── initiateSession()  POST /upload/drive/v3/files?uploadType=resumable
              │                       → Drive returns Location URI
              │
              └── uploadChunks()    PUT {Location} for each 8 MiB chunk
                    │  on 308 → next chunk
                    │  on 200/201 → upload complete, return fileID
                    │  on 5xx/429 → retry with exponential back-off
                    └── UploadProgressTracker.update(uploadID, bytesUploaded)
```

---

## New Types

### `UploadStatus` (string enum)

| Value         | Meaning                          |
|---------------|----------------------------------|
| `uploading`   | Upload is in progress            |
| `completed`   | Upload finished successfully     |
| `failed`      | Upload failed permanently        |

### `UploadProgress`

```go
type UploadProgress struct {
    UploadID      string
    TotalBytes    int64
    UploadedBytes int64
    Status        UploadStatus
    Error         string        // non-empty when Status == failed
    StartedAt     time.Time
    CompletedAt   *time.Time   // non-nil when Status == completed
}
```

### `ResumableStorageService` (interface)

Extends `StorageService` with:

```go
GetUploadProgress(uploadID string) (UploadProgress, bool)
```

`GoogleDriveStorageService` satisfies both `StorageService` and `ResumableStorageService`.

---

## New Components

### `UploadProgressTracker`

Thread-safe in-memory store of active upload sessions.

```go
tracker := storage.NewUploadProgressTracker()

// Read progress from anywhere in the application:
progress, ok := tracker.Get("my-upload-id")
if ok {
    fmt.Printf("%.1f%%\n", float64(progress.UploadedBytes)/float64(progress.TotalBytes)*100)
}
```

### `ResumableUploadConfig`

Controls chunking, retry policy and upload endpoint.

| Field             | Default                                        | Description                              |
|-------------------|------------------------------------------------|------------------------------------------|
| `ChunkSize`       | 8 MiB (must be a multiple of 256 KiB)         | Bytes per chunk PUT request              |
| `MaxRetries`      | 5                                              | Per-chunk retry limit on transient errors|
| `RetryBaseDelay`  | 1 s                                            | Base delay for exponential back-off      |
| `UploadEndpoint`  | `https://www.googleapis.com/upload/drive/v3/files` | Override in tests to mock the API    |

### `ResumableUploadService`

Low-level service that owns the Drive resumable protocol, retry logic and progress tracking.

```go
svc := storage.NewResumableUploadService(httpClient, tracker, cfg)
fileID, err := svc.UploadFile(ctx, folderID, filename, contentType, reader, totalSize, uploadID)
```

---

## How to Use

### Automatic (recommended)

Set `UploadOptions.FileSize` when calling `StorageService.Upload`. If the backend is Google Drive and the size exceeds the threshold, resumable upload is used automatically.

```go
opts := storage.UploadOptions{
    ContentType: "video/mp4",
    UserID:      userID,
    Folder:      "uploads",
    Filename:    file.Filename,
    FileSize:    file.Size,   // ← triggers resumable for files > 32 MiB
    UploadID:    videoID,     // ← optional, enables progress tracking
}
info, err := storageSvc.Upload(ctx, key, src, opts)
```

### Check Progress

Type-assert to `ResumableStorageService` to read progress:

```go
if rsvc, ok := storageSvc.(storage.ResumableStorageService); ok {
    p, found := rsvc.GetUploadProgress(videoID)
    if found {
        fmt.Printf("status=%s uploaded=%d/%d\n", p.Status, p.UploadedBytes, p.TotalBytes)
    }
}
```

### Resume an Interrupted Upload

Use `ResumableUploadService.ResumeUpload` directly when a session is interrupted:

```go
fileID, err := resumableSvc.ResumeUpload(ctx, uploadID, reader, totalSize)
```

The service queries Drive for the confirmed byte offset and continues from there without re-uploading already-received bytes.

---

## Configuration

The resumable upload feature requires no additional environment variables beyond the existing Google Drive credentials:

| Variable                       | Description                                   |
|--------------------------------|-----------------------------------------------|
| `STORAGE_PROVIDER`             | Set to `google_drive`                         |
| `GOOGLE_DRIVE_CLIENT_ID`       | OAuth2 client ID                              |
| `GOOGLE_DRIVE_CLIENT_SECRET`   | OAuth2 client secret                          |
| `GOOGLE_DRIVE_REFRESH_TOKEN`   | OAuth2 refresh token (offline access)         |
| `GOOGLE_DRIVE_FOLDER_ID`       | Optional root folder ID (empty = My Drive)    |

---

## Error Handling

| Scenario                          | Behaviour                                                      |
|-----------------------------------|----------------------------------------------------------------|
| Transient network error           | Retry up to `MaxRetries` times with exponential back-off       |
| HTTP 429 Too Many Requests        | Retry with back-off                                            |
| HTTP 5xx Server Error             | Retry with back-off                                            |
| HTTP 4xx (non-429)                | Non-retriable; upload fails immediately                        |
| Upload URI expired (Drive 404)    | Cannot resume; must start a new upload session                 |
| Context cancelled                 | Upload stops immediately; progress is marked as `failed`       |

---

## Testing

All components are covered by unit and integration tests using a mock HTTP server:

```bash
cd apps/api
go test ./internal/storage/... -v
```

Key test scenarios:

| Test                                                     | What it covers                              |
|----------------------------------------------------------|---------------------------------------------|
| `TestResumableUploadService_SmallFile`                   | Single-chunk upload                         |
| `TestResumableUploadService_MultiChunk`                  | Multi-chunk upload (size-aligned)           |
| `TestResumableUploadService_RetryOnTransientError`       | 503 on first chunk → retry succeeds         |
| `TestResumableUploadService_ProgressTracking`            | Progress reaches 100% and status=completed  |
| `TestResumableUploadService_ContextCancellation`         | Upload stops on ctx cancel                  |
| `TestResumableUploadService_QueryUploadStatus`           | Status query parses Drive Range header      |
| `TestGoogleDriveStorageService_Upload_LargeFile_UsesResumable` | Files > 32 MiB route to resumable     |
| `TestGoogleDriveStorageService_Upload_SmallFile_SkipsResumable`| Files ≤ 32 MiB use simple upload      |
| `TestGoogleDriveStorageService_GetUploadProgress`        | `GetUploadProgress` returns completed state |
| `TestUploadProgressTracker_ConcurrentSafe`               | 50 concurrent goroutines, no data race      |
