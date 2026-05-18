import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { ScheduledPost } from "@/types";

interface CreateScheduledPostPayload {
  clip_id: string;
  social_account_id: string;
  scheduled_at: string;
  caption?: string;
  hashtags?: string[];
}

export function useScheduledPosts(page = 1, limit = 20) {
  return useQuery({
    queryKey: ["scheduled_posts", page, limit],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: ScheduledPost[] }>(
        `/social/schedule?page=${page}&limit=${limit}`
      );
      return data.data;
    },
  });
}

export function useCreateScheduledPost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateScheduledPostPayload) =>
      apiClient.post<{ data: ScheduledPost }>("/social/schedule", payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scheduled_posts"] });
    },
  });
}

export function useCancelScheduledPost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/social/schedule/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scheduled_posts"] });
    },
  });
}
