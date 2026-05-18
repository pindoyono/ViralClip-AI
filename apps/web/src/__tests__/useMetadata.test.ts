/**
 * Tests for useEnhanceMetadata hook.
 */
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    post: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import { useEnhanceMetadata } from "@/hooks/useMetadata";
import type { MetadataEnhanceResponse } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_CLIP = {
  id: "clip-1",
  video_id: "vid-1",
  title: "Enhanced: Amazing Viral Moment",
  description: "Discover this incredible moment.",
  storage_path: "/clips/clip-1.mp4",
  thumbnail_url: "",
  duration: 45,
  start_time: 10,
  end_time: 55,
  viral_score: 0.87,
  hashtags: ["viral", "trending", "fyp"],
  suggested_platforms: ["tiktok"],
  has_subtitles: false,
  status: "ready" as const,
  created_at: new Date().toISOString(),
};

const MOCK_RESPONSE: MetadataEnhanceResponse = {
  clip: MOCK_CLIP,
  keywords: ["viral", "social", "trending"],
  category: "Entertainment",
  optimal_post_times: ["7:00 PM EST on Weekdays"],
};

describe("useEnhanceMetadata", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls the enhance endpoint and returns the response", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_RESPONSE },
    });

    const { result } = renderHook(() => useEnhanceMetadata(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        clipId: "clip-1",
        request: { platform: "tiktok" },
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(apiClient.post).toHaveBeenCalledWith(
      "/clips/clip-1/metadata/enhance",
      expect.objectContaining({ platform: "tiktok" })
    );
    expect(result.current.data?.category).toBe("Entertainment");
    expect(result.current.data?.keywords).toContain("viral");
    expect(result.current.data?.optimal_post_times[0]).toContain("EST");
  });

  it("uses an empty body when no request options are provided", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_RESPONSE },
    });

    const { result } = renderHook(() => useEnhanceMetadata(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ clipId: "clip-2" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/clips/clip-2/metadata/enhance",
      {}
    );
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.post as jest.Mock).mockRejectedValueOnce(
      new Error("AI service unavailable")
    );

    const { result } = renderHook(() => useEnhanceMetadata(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ clipId: "clip-err", request: { platform: "instagram" } });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });

  it("is idle before any mutation is triggered", () => {
    const { result } = renderHook(() => useEnhanceMetadata(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isPending).toBe(false);
    expect(result.current.isIdle).toBe(true);
  });

  it("returns the updated clip with the enhanced title", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_RESPONSE },
    });

    const { result } = renderHook(() => useEnhanceMetadata(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        clipId: "clip-1",
        request: { platform: "youtube", niche: "tech", tone: "educational" },
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.clip.title).toBe(
      "Enhanced: Amazing Viral Moment"
    );
    expect(result.current.data?.clip.id).toBe("clip-1");
  });
});
