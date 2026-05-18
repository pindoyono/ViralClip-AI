/**
 * Tests for useAnalyticsSummary, useClipAnalytics and useTrendingTopics hooks.
 */
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import { useAnalyticsSummary, useClipAnalytics, useTrendingTopics } from "@/hooks/useAnalytics";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_SUMMARY = {
  total_views: 15000,
  total_likes: 3200,
  total_shares: 480,
  avg_engagement_rate: 0.12,
  top_platform: "tiktok",
  clips_published: 8,
};

const MOCK_CLIP_ANALYTICS = [
  {
    id: "a1",
    clip_id: "c1",
    platform: "tiktok",
    views: 5000,
    likes: 800,
    comments: 45,
    shares: 120,
    saves: 200,
    reach: 7500,
    engagement_rate: 0.14,
    synced_at: "2024-01-02T00:00:00Z",
  },
];

describe("useAnalyticsSummary", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches analytics summary on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_SUMMARY },
    });

    const { result } = renderHook(() => useAnalyticsSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.total_views).toBe(15000);
    expect(result.current.data?.top_platform).toBe("tiktok");
    expect(apiClient.get).toHaveBeenCalledWith("/analytics/summary");
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Unauthorized"));

    const { result } = renderHook(() => useAnalyticsSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useClipAnalytics", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches per-clip analytics", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_CLIP_ANALYTICS },
    });

    const { result } = renderHook(() => useClipAnalytics("c1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].views).toBe(5000);
    expect(apiClient.get).toHaveBeenCalledWith("/clips/c1/analytics");
  });

  it("does not fetch when clipId is empty", () => {
    renderHook(() => useClipAnalytics(""), { wrapper: createWrapper() });
    expect(apiClient.get).not.toHaveBeenCalled();
  });
});

const MOCK_TRENDING = [
  {
    id: "t1",
    platform: "tiktok",
    topic: "AI Tips",
    hashtag: "aitips",
    category: "tech",
    trend_score: 87.5,
    post_count: 12000,
    view_count: 5000000,
    growth_rate: 24.3,
    region: "global",
    expires_at: "2024-02-01T00:00:00Z",
  },
];

describe("useTrendingTopics", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches trending topics without platform filter", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_TRENDING },
    });

    const { result } = renderHook(() => useTrendingTopics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].topic).toBe("AI Tips");
    expect(apiClient.get).toHaveBeenCalledWith("/trending");
  });

  it("fetches trending topics with platform filter", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_TRENDING },
    });

    const { result } = renderHook(() => useTrendingTopics("tiktok"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.get).toHaveBeenCalledWith("/trending?platform=tiktok");
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useTrendingTopics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

