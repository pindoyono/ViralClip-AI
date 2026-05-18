/**
 * Tests for useVideos and useDeleteVideo hooks.
 */
import { renderHook, waitFor } from "@testing-library/react";
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
import { useVideos, useVideo, useDeleteVideo, useProcessVideo } from "@/hooks/useVideos";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_VIDEO = {
  id: "v1",
  user_id: "u1",
  title: "Test Video",
  description: "",
  original_filename: "test.mp4",
  storage_path: "/videos/v1.mp4",
  file_size: 1048576,
  duration: 120,
  resolution: "1920x1080",
  status: "completed" as const,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

describe("useVideos", () => {
  afterEach(() => jest.clearAllMocks());

  it("returns a paginated list of videos on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: { videos: [MOCK_VIDEO], total: 1, page: 1, limit: 20, total_pages: 1 } },
    });

    const { result } = renderHook(() => useVideos(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.videos).toHaveLength(1);
    expect(result.current.data?.videos[0].title).toBe("Test Video");
  });

  it("calls the correct endpoint with page and limit", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: { videos: [], total: 0, page: 2, limit: 5, total_pages: 0 } },
    });

    renderHook(() => useVideos(2, 5), { wrapper: createWrapper() });

    await waitFor(() => expect(apiClient.get).toHaveBeenCalledWith("/videos?page=2&limit=5"));
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useVideos(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useVideo", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches a single video by id", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_VIDEO },
    });

    const { result } = renderHook(() => useVideo("v1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe("v1");
    expect(apiClient.get).toHaveBeenCalledWith("/videos/v1");
  });

  it("does not fetch when id is empty", () => {
    renderHook(() => useVideo(""), { wrapper: createWrapper() });
    expect(apiClient.get).not.toHaveBeenCalled();
  });
});

describe("useDeleteVideo", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls DELETE /videos/:id", async () => {
    (apiClient.delete as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: { videos: [], total: 0 } } });

    const { result } = renderHook(() => useDeleteVideo(), { wrapper: createWrapper() });

    result.current.mutate("v1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.delete).toHaveBeenCalledWith("/videos/v1");
  });
});

describe("useProcessVideo", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls POST /videos/:id/process", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: { videos: [], total: 0 } } });

    const { result } = renderHook(() => useProcessVideo(), { wrapper: createWrapper() });

    result.current.mutate("v1");
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith("/videos/v1/process");
  });
});
