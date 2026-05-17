import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { Clip, PaginatedResponse } from "@/types";

export function useClips(videoId?: string, page = 1, limit = 20) {
  return useQuery({
    queryKey: ["clips", videoId, page, limit],
    queryFn: async () => {
      const params = new URLSearchParams({ page: String(page), limit: String(limit) });
      if (videoId) params.set("video_id", videoId);
      const { data } = await apiClient.get<{ data: PaginatedResponse<Clip> }>(`/clips?${params}`);
      return data.data;
    },
  });
}

export function useClip(id: string) {
  return useQuery({
    queryKey: ["clip", id],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: Clip }>(`/clips/${id}`);
      return data.data;
    },
    enabled: !!id,
  });
}

export function useDeleteClip() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/clips/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["clips"] });
    },
  });
}
