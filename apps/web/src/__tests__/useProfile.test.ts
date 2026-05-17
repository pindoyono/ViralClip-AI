/**
 * Tests for useProfile and useUpdateProfile hooks.
 */
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

jest.mock("@/lib/api", () => ({
  apiClient: {
    get: jest.fn(),
    patch: jest.fn(),
  },
}));

jest.mock("@/lib/store", () => ({
  useAuthStore: jest.fn((selector: (s: { token: string | null; setAuth: jest.Mock }) => unknown) =>
    selector({ token: "test-token", setAuth: jest.fn() })
  ),
}));

import { apiClient } from "@/lib/api";
import { useProfile, useUpdateProfile } from "@/hooks/useProfile";
import type { User } from "@/types";

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const MOCK_USER: User = {
  id: "u1",
  name: "Alice",
  email: "alice@example.com",
  subscription_tier: "pro",
  created_at: "2024-01-01T00:00:00Z",
};

describe("useProfile", () => {
  afterEach(() => jest.clearAllMocks());

  it("fetches the current user profile on success", async () => {
    (apiClient.get as jest.Mock).mockResolvedValueOnce({ data: { data: MOCK_USER } });

    const { result } = renderHook(() => useProfile(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.name).toBe("Alice");
    expect(result.current.data?.email).toBe("alice@example.com");
    expect(apiClient.get).toHaveBeenCalledWith("/auth/me");
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.get as jest.Mock).mockRejectedValueOnce(new Error("Unauthorized"));

    const { result } = renderHook(() => useProfile(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });
});

describe("useUpdateProfile", () => {
  afterEach(() => jest.clearAllMocks());

  it("calls PATCH /auth/me with the payload", async () => {
    const updatedUser: User = { ...MOCK_USER, name: "Alice Updated" };
    (apiClient.patch as jest.Mock).mockResolvedValueOnce({ data: { data: updatedUser } });
    (apiClient.get as jest.Mock).mockResolvedValue({ data: { data: MOCK_USER } });

    const { result } = renderHook(() => useUpdateProfile(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync({ name: "Alice Updated" });
    });

    expect(apiClient.patch).toHaveBeenCalledWith("/auth/me", { name: "Alice Updated" });
    await waitFor(() => expect(result.current.data?.name).toBe("Alice Updated"));
  });

  it("surfaces an error when the API fails", async () => {
    (apiClient.patch as jest.Mock).mockRejectedValueOnce(new Error("Validation error"));

    const { result } = renderHook(() => useUpdateProfile(), { wrapper: createWrapper() });

    await act(async () => {
      try {
        await result.current.mutateAsync({ name: "" });
      } catch {
        // expected
      }
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
  });

  it("updates avatar_url correctly", async () => {
    const updatedUser: User = { ...MOCK_USER, avatar_url: "https://example.com/new.jpg" };
    (apiClient.patch as jest.Mock).mockResolvedValueOnce({ data: { data: updatedUser } });

    const { result } = renderHook(() => useUpdateProfile(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync({ avatar_url: "https://example.com/new.jpg" });
    });

    await waitFor(() => expect(result.current.data?.avatar_url).toBe("https://example.com/new.jpg"));
  });
});
