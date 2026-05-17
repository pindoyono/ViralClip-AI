export interface StorageProvider {
  upload(key: string, data: Buffer | NodeJS.ReadableStream, contentType?: string): Promise<string>;
  download(key: string): Promise<Buffer>;
  delete(key: string): Promise<void>;
  getSignedUrl(key: string, expiresInSeconds?: number): Promise<string>;
  exists(key: string): Promise<boolean>;
}

export type StorageProviderType = "local" | "gcs" | "s3";

export interface StorageConfig {
  provider: StorageProviderType;
  localBasePath?: string;
  gcsBucket?: string;
  gcsProjectId?: string;
  s3Bucket?: string;
  s3Region?: string;
}

export function getStorageKey(videoId: string, filename: string): string {
  return `videos/${videoId}/${filename}`;
}

export function getClipKey(videoId: string, clipId: string, filename: string): string {
  return `clips/${videoId}/${clipId}/${filename}`;
}

export function getThumbnailKey(videoId: string): string {
  return `thumbnails/${videoId}/thumb.jpg`;
}
