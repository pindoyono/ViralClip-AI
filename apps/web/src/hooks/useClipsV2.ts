import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ClipV2GenerateRequest, ClipV2GenerateResponse } from "@/types";

/**
 * Calls the V2 Dynamic Clip Engine for a video.
 * POST /api/v1/videos/:videoId/clips/v2/generate
 */
export function useGenerateClipsV2(videoId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (req: ClipV2GenerateRequest) => {
      const { data } = await apiClient.post<{ data: ClipV2GenerateResponse }>(
        `/videos/${videoId}/clips/v2/generate`,
        req
      );
      return data.data;
    },
    onSuccess: () => {
      // Invalidate clip queries so any clip list for this video refreshes.
      queryClient.invalidateQueries({ queryKey: ["clips", videoId] });
    },
  });
}
