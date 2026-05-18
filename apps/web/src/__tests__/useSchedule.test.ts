/**
 * Tests for useScheduledPosts, useCreateScheduledPost, and useCancelScheduledPost hooks.
 */
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
    post: jest.fn(),
    delete: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import {
  useScheduledPosts,
  useCreateScheduledPost,
  useCancelScheduledPost,
} from "@/hooks/useSchedule";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_POST = {
  id: "p1",
  clip_id: "c1",
  social_account_id: "sa1",
  platform: "tiktok",
  scheduled_at: "2025-06-01T12:00:00Z",
  status: "pending" as const,
  caption: "Check this out!",
  hashtags: ["#viral"],
};

describe("useScheduledPosts", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches scheduled posts on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: [MOCK_POST] },
    });

    const { result } = renderHook(() => useScheduledPosts(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].platform).toBe("tiktok");
    expect(apiClient.get).toHaveBeenCalledWith(
      "/social/schedule?page=1&limit=20"
    );
  });

  it("calls the correct endpoint with page and limit", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: [] },
    });

    renderHook(() => useScheduledPosts(2, 5), { wrapper: createWrapper() });

    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(apiClient.get).toHaveBeenCalledWith(
      "/social/schedule?page=2&limit=5"
    );
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(
      new Error("Unauthorized")
    );

    const { result } = renderHook(() => useScheduledPosts(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useCreateScheduledPost", () => {
  afterEach(() => jest.clearAllMocks());

  it("posts to /social/schedule and invalidates cache on success", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_POST },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useCreateScheduledPost(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        clip_id: "c1",
        social_account_id: "sa1",
        scheduled_at: "2025-06-01T12:00:00Z",
        caption: "Check this out!",
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/social/schedule",
      expect.objectContaining({ clip_id: "c1" })
    );
  });
});

describe("useCancelScheduledPost", () => {
  afterEach(() => jest.clearAllMocks());

  it("sends DELETE to /social/schedule/:id", async () => {
    (apiClient.delete as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useCancelScheduledPost(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate("p1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.delete).toHaveBeenCalledWith("/social/schedule/p1");
  });
});
