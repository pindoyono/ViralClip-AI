import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";

export interface ContentProfile {
  id: string;
  user_id: string;
  name: string;
  platform: "youtube" | "tiktok" | "instagram" | "general";
  niche: string;
  tone_style: string;
  audience_age: string;
  keywords: string;
  is_default: boolean;
  created_at: string;
}

interface CreateContentProfilePayload {
  name: string;
  platform: string;
  niche?: string;
  tone_style?: string;
  audience_age?: string;
  keywords?: string;
  is_default?: boolean;
}

interface UpdateContentProfilePayload {
  name?: string;
  niche?: string;
  tone_style?: string;
  audience_age?: string;
  keywords?: string;
  is_default?: boolean;
}

export function useContentProfiles() {
  return useQuery({
    queryKey: ["content_profiles"],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: ContentProfile[] }>("/content-profiles");
      return data.data;
    },
  });
}

export function useCreateContentProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreateContentProfilePayload) =>
      apiClient.post<{ data: ContentProfile }>("/content-profiles", payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["content_profiles"] });
    },
  });
}

export function useUpdateContentProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...payload }: UpdateContentProfilePayload & { id: string }) =>
      apiClient.patch<{ data: ContentProfile }>(`/content-profiles/${id}`, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["content_profiles"] });
    },
  });
}

export function useDeleteContentProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/content-profiles/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["content_profiles"] });
    },
  });
}
