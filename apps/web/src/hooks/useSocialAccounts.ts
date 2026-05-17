import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "@/lib/api";
import type { SocialAccount } from "@/types";

export function useSocialAccounts() {
  return useQuery({
    queryKey: ["social_accounts"],
    queryFn: async () => {
      const { data } = await apiClient.get<{ data: SocialAccount[] }>("/social/accounts");
      return data.data;
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
