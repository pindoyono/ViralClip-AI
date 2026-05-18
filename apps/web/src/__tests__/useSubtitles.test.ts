/**
 * Tests for useBurnSubtitles hook.
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
import { useBurnSubtitles } from "@/hooks/useSubtitles";
import type { SubtitleBurnResponse } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_RESPONSE: SubtitleBurnResponse = {
  video_id: "vid-1",
  clips_processed: 3,
};

describe("useBurnSubtitles", () => {
  afterEach(() => jest.clearAllMocks());

  it("posts to the burn endpoint and returns the response", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_RESPONSE },
    });

    const { result } = renderHook(() => useBurnSubtitles("vid-1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ style: "bold", font_size: 28 });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(apiClient.post).toHaveBeenCalledWith(
      "/videos/vid-1/subtitles/burn",
      expect.objectContaining({ style: "bold", font_size: 28 })
    );
    expect(result.current.data?.clips_processed).toBe(3);
    expect(result.current.data?.video_id).toBe("vid-1");
  });

  it("posts with an empty body when no options provided", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: { video_id: "vid-2", clips_processed: 1 } },
    });

    const { result } = renderHook(() => useBurnSubtitles("vid-2"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/videos/vid-2/subtitles/burn",
      {}
    );
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.post as jest.Mock).mockRejectedValueOnce(
      new Error("AI unavailable")
    );

    const { result } = renderHook(() => useBurnSubtitles("vid-3"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ style: "default" });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });

  it("is not pending before any mutation is triggered", () => {
    const { result } = renderHook(() => useBurnSubtitles("vid-4"), {
      wrapper: createWrapper(),
    });

    expect(result.current.isPending).toBe(false);
    expect(result.current.isIdle).toBe(true);
  });
});
