export type SubscriptionTier = "free" | "starter" | "pro" | "enterprise";
export type VideoStatus = "pending" | "processing" | "completed" | "failed";
export type ClipStatus = "pending" | "processing" | "ready" | "failed";
export type PostStatus = "pending" | "published" | "failed" | "cancelled";
export type SocialPlatform = "tiktok" | "instagram" | "youtube" | "twitter";

export interface User {
  id: string;
  name: string;
  email: string;
  avatar_url?: string;
  subscription_tier: SubscriptionTier;
  created_at: string;
  updated_at: string;
}

export interface Video {
  id: string;
  user_id: string;
  title: string;
  description?: string;
  original_filename: string;
  storage_path: string;
  thumbnail_url?: string;
  duration: number;
  file_size: number;
  resolution?: string;
  status: VideoStatus;
  transcript?: string;
  language?: string;
  created_at: string;
  updated_at: string;
}

export interface Clip {
  id: string;
  video_id: string;
  title: string;
  description?: string;
  storage_path: string;
  thumbnail_url?: string;
  duration: number;
  start_time: number;
  end_time: number;
  viral_score: number;
  hashtags: string[];
  suggested_platforms: string[];
  has_subtitles: boolean;
  status: ClipStatus;
  created_at: string;
  updated_at: string;
}

export interface SocialAccount {
  id: string;
  user_id: string;
  platform: SocialPlatform;
  platform_user_id: string;
  username: string;
  access_token?: string;
  refresh_token?: string;
  expires_at?: string;
  is_connected: boolean;
  created_at: string;
}

export interface ScheduledPost {
  id: string;
  clip_id: string;
  social_account_id: string;
  platform: SocialPlatform;
  scheduled_at: string;
  published_at?: string;
  status: PostStatus;
  caption?: string;
  hashtags: string[];
  platform_post_id?: string;
  error_message?: string;
  created_at: string;
}

export interface ClipAnalytics {
  id: string;
  clip_id: string;
  platform: SocialPlatform;
  views: number;
  likes: number;
  comments: number;
  shares: number;
  saves: number;
  reach: number;
  engagement_rate: number;
  synced_at: string;
}

export interface TranscriptSegment {
  start: number;
  end: number;
  text: string;
  confidence: number;
}

export interface ViralHook {
  text: string;
  type: "question" | "statement" | "statistic" | "story" | "challenge";
  viral_score: number;
  rationale: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface ApiResponse<T = unknown> {
  success: boolean;
  data: T;
  error?: string;
  message?: string;
}

export interface WebSocketMessage {
  type: string;
  payload: Record<string, unknown>;
  timestamp: string;
}

export interface VideoProcessingEvent extends WebSocketMessage {
  type: "video.processing" | "video.completed" | "video.failed";
  payload: {
    video_id: string;
    status: VideoStatus;
    progress?: number;
    clips_count?: number;
    error?: string;
  };
}
