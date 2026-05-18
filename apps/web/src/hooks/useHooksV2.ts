import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type {
  HookDetectRequest,
  HookDetectResponse,
  HookDetection,
} from "@/types";

/**
 * Fetches stored hook detections for a video from the API.
 * GET /api/v1/videos/:videoId/hooks
 */
export function useHookDetections(videoId: string, hookType?: string) {
  return useQuery({
    queryKey: ["hookDetections", videoId, hookType],
    queryFn: async () => {
      const params = hookType ? `?type=${encodeURIComponent(hookType)}` : "";
      const { data } = await apiClient.get<{
        data: { video_id: string; hooks: HookDetection[]; total: number };
      }>(`/videos/${videoId}/hooks${params}`);
      return data.data;
    },
    enabled: !!videoId,
  });
}

/**
 * Calls the V2 hook detection endpoint for a video.
 * POST /api/v1/videos/:videoId/hooks/detect
 */
export function useDetectHooks(videoId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (req: HookDetectRequest) => {
      const { data } = await apiClient.post<{ data: HookDetectResponse }>(
        `/videos/${videoId}/hooks/detect`,
        req
      );
      return data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hookDetections", videoId] });
    },
  });
}
