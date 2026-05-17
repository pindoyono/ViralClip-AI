/**
 * Tests for useSocialAccounts and useDisconnectSocialAccount hooks.
 */
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
    delete: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import { useSocialAccounts, useDisconnectSocialAccount } from "@/hooks/useSocialAccounts";
import type { SocialAccount } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_ACCOUNT: SocialAccount = {
  id: "a1",
  user_id: "u1",
  platform: "tiktok",
  username: "my_tiktok",
  display_name: "My TikTok",
  avatar_url: "",
  is_active: true,
  is_connected: true,
  followers_count: 5000,
  connected_at: "2024-01-01T00:00:00Z",
};

describe("useSocialAccounts", () => {
  afterEach(() => jest.clearAllMocks());

  it("returns connected social accounts on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: [MOCK_ACCOUNT] },
    });

    const { result } = renderHook(() => useSocialAccounts(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].platform).toBe("tiktok");
    expect(apiClient.get).toHaveBeenCalledWith("/social/accounts");
  });

  it("returns an empty array when no accounts are connected", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({ data: { data: [] } });

    const { result } = renderHook(() => useSocialAccounts(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Unauthorized"));

    const { result } = renderHook(() => useSocialAccounts(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useDisconnectSocialAccount", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls DELETE /social/accounts/:id", async () => {
    (apiClient.delete as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useDisconnectSocialAccount(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("a1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.delete).toHaveBeenCalledWith("/social/accounts/a1");
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.delete as jest.Mock).mockRejectedValueOnce(new Error("Not found"));
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useDisconnectSocialAccount(), {
      wrapper: createWrapper(),
    });

    result.current.mutate("nonexistent");
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});
