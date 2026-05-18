/**
 * Tests for useClips and useDeleteClip hooks.
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
import { useClips, useClip, useDeleteClip } from "@/hooks/useClips";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_CLIP = {
  id: "c1",
  video_id: "v1",
  title: "Amazing Clip",
  storage_path: "/clips/c1.mp4",
  duration: 30,
  start_time: 5,
  end_time: 35,
  viral_score: 0.88,
  hashtags: ["#viral"],
  suggested_platforms: ["tiktok"],
  has_subtitles: false,
  status: "ready" as const,
  created_at: "2024-01-01T00:00:00Z",
};

describe("useClips", () => {
  afterEach(() => jest.clearAllMocks());

  it("returns a paginated list of clips on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: { data: [MOCK_CLIP], total: 1, page: 1, limit: 20, total_pages: 1 } },
    });

    const { result } = renderHook(() => useClips(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.data).toHaveLength(1);
    expect(result.current.data?.data[0].title).toBe("Amazing Clip");
  });

  it("passes video_id filter when provided", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: { data: [], total: 0, page: 1, limit: 20, total_pages: 0 } },
    });

    renderHook(() => useClips("v1"), { wrapper: createWrapper() });

    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    const url: string = (apiClient.get as jest.Mock).mock.calls[0][0];
    expect(url).toContain("video_id=v1");
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Not found"));

    const { result } = renderHook(() => useClips(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useClip", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches a single clip by id", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_CLIP },
    });

    const { result } = renderHook(() => useClip("c1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe("c1");
    expect(apiClient.get).toHaveBeenCalledWith("/clips/c1");
  });

  it("does not fetch when id is empty", () => {
    renderHook(() => useClip(""), { wrapper: createWrapper() });
    expect(apiClient.get).not.toHaveBeenCalled();
  });
});

describe("useDeleteClip", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls DELETE /clips/:id", async () => {
    (apiClient.delete as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({
      data: { data: { data: [], total: 0 } },
    });

    const { result } = renderHook(() => useDeleteClip(), { wrapper: createWrapper() });

    result.current.mutate("c1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.delete).toHaveBeenCalledWith("/clips/c1");
  });
});
