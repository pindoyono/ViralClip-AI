import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { EnhanceMetadataRequest, MetadataEnhanceResponse } from "@/types";

/**
 * useEnhanceMetadata returns a mutation that calls
 * POST /api/v1/clips/:id/metadata/enhance.
 *
 * On success the cache for the affected clip and its parent video's clip list
 * is invalidated so the UI reflects the updated title, description, and hashtags.
 */
export function useEnhanceMetadata() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      clipId,
      request,
    }: {
      clipId: string;
      request?: EnhanceMetadataRequest;
    }) => {
      const { data } = await apiClient.post<{ data: MetadataEnhanceResponse }>(
        `/clips/${clipId}/metadata/enhance`,
        request ?? {}
      );
      return data.data;
    },
    onSuccess: (data) => {
      // Invalidate the individual clip query.
      queryClient.invalidateQueries({ queryKey: ["clip", data.clip.id] });
      // Invalidate the clips list for the parent video.
      queryClient.invalidateQueries({
        queryKey: ["clips", "video", data.clip.video_id],
      });
      // Invalidate the generic clips list.
      queryClient.invalidateQueries({ queryKey: ["clips"] });
    },
  });
}
