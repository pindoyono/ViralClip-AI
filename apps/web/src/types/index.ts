export interface User {
  id: string;
  name: string;
  email: string;
  avatar_url?: string;
  subscription_tier: "free" | "starter" | "pro" | "enterprise";
  created_at: string;
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
  resolution: string;
  status: "pending" | "processing" | "completed" | "failed";
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
  status: "pending" | "processing" | "ready" | "failed";
  created_at: string;
}

export interface SocialAccount {
  id: string;
  user_id: string;
  platform: "tiktok" | "instagram" | "youtube" | "twitter";
  username: string;
  display_name: string;
  avatar_url: string;
  is_active: boolean;
  is_connected: boolean;
  followers_count: number;
  connected_at: string;
  last_synced_at?: string;
  expires_at?: string;
}

export interface ScheduledPost {
  id: string;
  clip_id: string;
  social_account_id: string;
  platform: string;
  scheduled_at: string;
  status: "pending" | "published" | "failed" | "cancelled";
  caption?: string;
  hashtags: string[];
}

export interface ClipAnalytics {
  id: string;
  clip_id: string;
  platform: string;
  views: number;
  likes: number;
  comments: number;
  shares: number;
  saves: number;
  reach: number;
  engagement_rate: number;
  synced_at: string;
}

export interface TrendingTopic {
  id: string;
  platform: string;
  topic: string;
  hashtag: string;
  category: string;
  trend_score: number;
  post_count: number;
  view_count: number;
  growth_rate: number;
  region: string;
  expires_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface ApiResponse<T> {
  success: boolean;
  data: T;
  error?: string;
}

// =============================================================================
// V2 — Hook Detection
// =============================================================================

export interface TranscriptSegment {
  text: string;
  start: number;
  end: number;
}

export interface HookDetection {
  start: number;
  end: number;
  type: string;
  score: number;
  matched_pattern: string;
}

export interface HookDetectRequest {
  segments: TranscriptSegment[];
  min_score?: number;
}

export interface HookDetectResponse {
  video_id: string;
  hooks: HookDetection[];
  total: number;
}

// =============================================================================
// V2 — Dynamic Clip Engine
// =============================================================================

export type ClipV2ProfileType =
  | "gaming"
  | "comedy"
  | "education"
  | "politics"
  | "podcast"
  | "general";

export interface ClipV2ResultItem {
  start: string;
  end: string;
  start_seconds: number;
  end_seconds: number;
  score: number;
  hook_score: number;
  emotion_score: number;
  story_score: number;
  retention_score: number;
  profile_type: string;
}

export interface ClipV2GenerateRequest {
  segments: TranscriptSegment[];
  profile_type?: ClipV2ProfileType;
  min_clip_score?: number;
  max_clips?: number;
}

export interface ClipV2GenerateResponse {
  video_id: string;
  profile_type: string;
  clips: ClipV2ResultItem[];
  total: number;
}

// =============================================================================
// Subtitle Burning
// =============================================================================

export type SubtitleStyle = "default" | "bold" | "outline" | "shadow";

/** Optional style overrides for POST /api/v1/videos/:id/subtitles/burn */
export interface SubtitleBurnRequest {
  style?: SubtitleStyle;
  font_size?: number;
  primary_color?: string;
  outline_color?: string;
}

/** Response from POST /api/v1/videos/:id/subtitles/burn */
export interface SubtitleBurnResponse {
  video_id: string;
  clips_processed: number;
}
