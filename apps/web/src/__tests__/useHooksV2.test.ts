/**
 * Tests for useHookDetections and useDetectHooks hooks.
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
import { useHookDetections, useDetectHooks } from "@/hooks/useHooksV2";
import type { HookDetection, HookDetectResponse } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_HOOK: HookDetection = {
  start: 10.0,
  end: 15.0,
  type: "curiosity",
  score: 82,
  matched_pattern: "you won't believe",
};

const MOCK_HOOK_RESPONSE = {
  video_id: "v1",
  hooks: [MOCK_HOOK],
  total: 1,
};

describe("useHookDetections", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches stored hook detections for a video", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_HOOK_RESPONSE },
    });

    const { result } = renderHook(() => useHookDetections("v1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.hooks).toHaveLength(1);
    expect(result.current.data?.hooks[0].type).toBe("curiosity");
    expect(apiClient.get).toHaveBeenCalledWith("/videos/v1/hooks");
  });

  it("appends type filter when hookType is provided", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: { video_id: "v1", hooks: [], total: 0 } },
    });

    renderHook(() => useHookDetections("v1", "emotion"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    const url: string = (apiClient.get as jest.Mock).mock.calls[0][0];
    expect(url).toContain("type=emotion");
  });

  it("does not fetch when videoId is empty", () => {
    renderHook(() => useHookDetections(""), { wrapper: createWrapper() });
    expect(apiClient.get).not.toHaveBeenCalled();
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useHookDetections("v1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useDetectHooks", () => {
  afterEach(() => jest.clearAllMocks());

  it("posts to the detect endpoint and returns the response", async () => {
    const expectedResp: HookDetectResponse = MOCK_HOOK_RESPONSE;
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: expectedResp },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({
      data: { data: { video_id: "v1", hooks: [], total: 0 } },
    });

    const { result } = renderHook(() => useDetectHooks("v1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({
        segments: [{ text: "you won't believe this", start: 10.0, end: 15.0 }],
        min_score: 60,
      });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/videos/v1/hooks/detect",
      expect.objectContaining({
        segments: expect.any(Array),
        min_score: 60,
      })
    );
    expect(result.current.data?.total).toBe(1);
  });

  it("surfaces an error when the AI service fails", async () => {
    (apiClient.post as jest.Mock).mockRejectedValueOnce(new Error("AI unavailable"));

    const { result } = renderHook(() => useDetectHooks("v1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ segments: [{ text: "test", start: 0, end: 3 }] });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});
