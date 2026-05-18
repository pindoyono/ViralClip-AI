import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { SubtitleBurnRequest, SubtitleBurnResponse } from "@/types";

/**
 * Triggers subtitle burning for all extracted clips of a video.
 * POST /api/v1/videos/:videoId/subtitles/burn
 *
 * On success the query cache for the video's clips is invalidated so that
 * the updated `has_subtitles` flag is reflected immediately.
 */
export function useBurnSubtitles(videoId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (req: SubtitleBurnRequest = {}) => {
      const { data } = await apiClient.post<{ data: SubtitleBurnResponse }>(
        `/videos/${videoId}/subtitles/burn`,
        req
      );
      return data.data;
    },
    onSuccess: () => {
      // Refresh the clips list so has_subtitles updates in the UI.
      queryClient.invalidateQueries({ queryKey: ["clips", videoId] });
    },
  });
}
