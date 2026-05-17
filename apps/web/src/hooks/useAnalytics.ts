import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ClipAnalytics } from "@/types";

interface AnalyticsSummary {
  total_views: number;
  total_likes: number;
  total_shares: number;
  avg_engagement_rate: number;
  top_platform: string;
  clips_published: number;
}

export function useAnalyticsSummary() {
  return useQuery({
    queryKey: ["analytics", "summary"],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: AnalyticsSummary }>("/analytics/summary");
      return data.data;
    },
  });
}

export function useClipAnalytics(clipId: string) {
  return useQuery({
    queryKey: ["analytics", "clip", clipId],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: ClipAnalytics[] }>(`/clips/${clipId}/analytics`);
      return data.data;
    },
    enabled: !!clipId,
  });
}
