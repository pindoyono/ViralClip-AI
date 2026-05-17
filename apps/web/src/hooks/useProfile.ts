import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import { useAuthStore } from "@/lib/store";
import type { User } from "@/types";

interface UpdateProfilePayload {
  name?: string;
  avatar_url?: string;
}

export function useProfile() {
  return useQuery({
    queryKey: ["profile"],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: User }>("/auth/me");
      return data.data;
    },
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  const setAuth = useAuthStore((s) => s.setAuth);
  const token = useAuthStore((s) => s.token);

  return useMutation({
    mutationFn: async (payload: UpdateProfilePayload) => {
      const { data } = await apiClient.patch<{ data: User }>("/auth/me", payload);
      return data.data;
    },
    onSuccess: (updatedUser) => {
      queryClient.setQueryData(["profile"], updatedUser);
      if (token) {
        setAuth(updatedUser, token);
      }
    },
  });
}
