export type QueueName = "video-processing" | "social-publishing" | "analytics-sync" | "cleanup";

export interface QueueMessage<T = Record<string, unknown>> {
  id: string;
  queue: QueueName;
  payload: T;
  attempts: number;
  created_at: string;
  scheduled_at?: string;
}

export interface VideoProcessingJob {
  video_id: string;
  user_id: string;
  storage_path: string;
}

export interface PublishingJob {
  scheduled_post_id: string;
  clip_id: string;
  social_account_id: string;
  platform: string;
}

export interface AnalyticsSyncJob {
  clip_id: string;
  platform: string;
  platform_post_id: string;
}

export const QUEUE_KEYS: Record<QueueName, string> = {
  "video-processing": "queue:video_processing",
  "social-publishing": "queue:social_publishing",
  "analytics-sync": "queue:analytics_sync",
  "cleanup": "queue:cleanup",
};
