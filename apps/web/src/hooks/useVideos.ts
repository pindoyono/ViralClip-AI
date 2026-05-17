import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { Video, PaginatedResponse } from "@/types";

export function useVideos(page = 1, limit = 20) {
  return useQuery({
    queryKey: ["videos", page, limit],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: PaginatedResponse<Video> }>(
        `/videos?page=${page}&limit=${limit}`
      );
      return data.data;
    },
  });
}

export function useVideo(id: string) {
  return useQuery({
    queryKey: ["video", id],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: Video }>(`/videos/${id}`);
      return data.data;
    },
    enabled: !!id,
  });
}

export function useDeleteVideo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/videos/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["videos"] });
    },
  });
}

export function useProcessVideo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.post(`/videos/${id}/process`),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: ["video", id] });
    },
  });
}
