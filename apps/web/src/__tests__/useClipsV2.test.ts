/**
 * Tests for useGenerateClipsV2 hook.
 */
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import { useGenerateClipsV2 } from "@/hooks/useClipsV2";
import type { ClipV2ResultItem, ClipV2GenerateResponse } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_CLIP_V2: ClipV2ResultItem = {
  start: "00:00:10",
  end: "00:00:45",
  start_seconds: 10.0,
  end_seconds: 45.0,
  score: 78,
  hook_score: 0.85,
  emotion_score: 0.72,
  story_score: 0.68,
  retention_score: 0.90,
  profile_type: "podcast",
};

const MOCK_RESPONSE: ClipV2GenerateResponse = {
  video_id: "v1",
  profile_type: "podcast",
  clips: [MOCK_CLIP_V2],
  total: 1,
};

describe("useGenerateClipsV2", () => {
  afterEach(() => jest.clearAllMocks());

  it("posts to the V2 generate endpoint and returns clip candidates", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_RESPONSE },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({
      data: { data: { data: [], total: 0 } },
    });

    const { result } = renderHook(() => useGenerateClipsV2("v1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        segments: [{ text: "Great insight here", start: 10.0, end: 45.0 }],
        profile_type: "podcast",
        min_clip_score: 60,
        max_clips: 5,
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/videos/v1/clips/v2/generate",
      expect.objectContaining({
        segments: expect.any(Array),
        profile_type: "podcast",
        min_clip_score: 60,
        max_clips: 5,
      })
    );
    expect(result.current.data?.total).toBe(1);
    expect(result.current.data?.clips[0].score).toBe(78);
  });

  it("uses default profile when profile_type is omitted", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: {
        data: { video_id: "v1", profile_type: "general", clips: [], total: 0 },
      },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({
      data: { data: { data: [], total: 0 } },
    });

    const { result } = renderHook(() => useGenerateClipsV2("v1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        segments: [{ text: "Hello world", start: 0, end: 5 }],
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // profile_type omitted in the client request; server defaults to "general"
    const postArg = (apiClient.post as jest.Mock).mock.calls[0][1];
    expect(postArg.profile_type).toBeUndefined();
  });

  it("surfaces an error when the AI service returns an error", async () => {
    (apiClient.post as jest.Mock).mockRejectedValueOnce(new Error("AI service unavailable"));

    const { result } = renderHook(() => useGenerateClipsV2("v1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        segments: [{ text: "test", start: 0, end: 3 }],
      });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});
