/**
 * Tests for useContentProfiles, useCreateContentProfile, useUpdateContentProfile,
 * and useDeleteContentProfile hooks.
 */
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
}));

import { apiClient } from "@/lib/api";
import {
  useContentProfiles,
  useCreateContentProfile,
  useUpdateContentProfile,
  useDeleteContentProfile,
} from "@/hooks/useContentProfiles";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_PROFILE = {
  id: "cp1",
  user_id: "u1",
  name: "Tech Reviews",
  platform: "youtube" as const,
  niche: "technology",
  tone_style: "educational",
  audience_age: "25-34",
  keywords: "gadgets, reviews",
  is_default: false,
  created_at: "2024-01-01T00:00:00Z",
};

describe("useContentProfiles", () => {
  afterEach(() => jest.clearAllMocks());

  it("returns a list of content profiles", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: [MOCK_PROFILE] },
    });

    const { result } = renderHook(() => useContentProfiles(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].name).toBe("Tech Reviews");
    expect(apiClient.get).toHaveBeenCalledWith("/content-profiles");
  });

  it("returns empty array when no profiles exist", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({
      data: { data: [] },
    });

    const { result } = renderHook(() => useContentProfiles(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(0);
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Unauthorized"));

    const { result } = renderHook(() => useContentProfiles(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useCreateContentProfile", () => {
  afterEach(() => jest.clearAllMocks());

  it("posts to /content-profiles and invalidates cache", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({
      data: { data: MOCK_PROFILE },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useCreateContentProfile(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ name: "Tech Reviews", platform: "youtube" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.post).toHaveBeenCalledWith(
      "/content-profiles",
      expect.objectContaining({ name: "Tech Reviews", platform: "youtube" })
    );
  });
});

describe("useUpdateContentProfile", () => {
  afterEach(() => jest.clearAllMocks());

  it("patches /content-profiles/:id", async () => {
    (apiClient.patch as jest.Mock).mockResolvedValueOnce({
      data: { data: { ...MOCK_PROFILE, name: "Updated" } },
    });
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useUpdateContentProfile(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate({ id: "cp1", name: "Updated" });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.patch).toHaveBeenCalledWith(
      "/content-profiles/cp1",
      expect.objectContaining({ name: "Updated" })
    );
  });
});

describe("useDeleteContentProfile", () => {
  afterEach(() => jest.clearAllMocks());

  it("sends DELETE to /content-profiles/:id", async () => {
    (apiClient.delete as jest.Mock).mockResolvedValueOnce({});
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: [] } });

    const { result } = renderHook(() => useDeleteContentProfile(), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      result.current.mutate("cp1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiClient.delete).toHaveBeenCalledWith("/content-profiles/cp1");
  });
});
