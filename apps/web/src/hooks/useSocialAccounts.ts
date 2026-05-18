import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { SocialAccount } from "@/types";

interface ConnectSocialAccountPayload {
  platform: string;
  username: string;
  display_name?: string;
  access_token?: string;
  followers_count?: number;
}

export function useSocialAccounts() {
  return useQuery({
    queryKey: ["social_accounts"],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: SocialAccount[] }>("/social/accounts");
      return data.data;
    },
  });
}

export function useConnectSocialAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: ConnectSocialAccountPayload) =>
      apiClient.post<{ data: SocialAccount }>("/social/accounts", payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["social_accounts"] });
    },
  });
}

export function useDisconnectSocialAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiClient.delete(`/social/accounts/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["social_accounts"] });
    },
  });
}
