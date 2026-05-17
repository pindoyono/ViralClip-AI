/**
 * Tests for the useAuth hook.
 */
import { renderHook, act } from "@testing-library/react";

// Mock zustand store before importing useAuth
const mockSetAuth = jest.fn();
const mockClearAuth = jest.fn();
const mockPush = jest.fn();

const mockStore = {
  user: null,
  token: null,
  isAuthenticated: false,
  setAuth: mockSetAuth,
  clearAuth: mockClearAuth,
};

jest.mock("@/lib/store", () => ({
  useAuthStore: jest.fn(() => mockStore),
}));

jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}));

jest.mock("@/lib/api", () => ({
  apiClient: {
    post: jest.fn(),
  },
}));

// Import AFTER mocks are set up
import { useAuth } from "@/hooks/useAuth";
import { apiClient } from "@/lib/api";
import { useAuthStore } from "@/lib/store";

describe("useAuth", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Reset store state
    (useAuthStore as jest.Mock).mockReturnValue({ ...mockStore });
  });

  it("returns user, token, and isAuthenticated from store", () => {
    const user = { id: "1", name: "Alice", email: "alice@example.com" };
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      user,
      token: "access-token-123",
      isAuthenticated: true,
    });

    const { result } = renderHook(() => useAuth());

    expect(result.current.user).toEqual(user);
    expect(result.current.token).toBe("access-token-123");
    expect(result.current.isAuthenticated).toBe(true);
  });

  it("returns null user and false isAuthenticated when not logged in", () => {
    const { result } = renderHook(() => useAuth());

    expect(result.current.user).toBeNull();
    expect(result.current.token).toBeNull();
    expect(result.current.isAuthenticated).toBe(false);
  });

  it("logout calls apiClient.post with /auth/logout", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({});
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      clearAuth: mockClearAuth,
    });

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      await result.current.logout();
    });

    expect(apiClient.post).toHaveBeenCalledWith("/auth/logout");
  });

  it("logout calls clearAuth after successful API call", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({});
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      clearAuth: mockClearAuth,
    });

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      await result.current.logout();
    });

    expect(mockClearAuth).toHaveBeenCalledTimes(1);
  });

  it("logout redirects to /login after clearing auth", async () => {
    (apiClient.post as jest.Mock).mockResolvedValueOnce({});
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      clearAuth: mockClearAuth,
    });

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      await result.current.logout();
    });

    expect(mockPush).toHaveBeenCalledWith("/login");
  });

  it("logout still clears auth and redirects even when API call fails", async () => {
    (apiClient.post as jest.Mock).mockRejectedValueOnce(new Error("Network error"));
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      clearAuth: mockClearAuth,
    });

    const { result } = renderHook(() => useAuth());

    await act(async () => {
      await result.current.logout();
    });

    // Should still clear auth and redirect on failure
    expect(mockClearAuth).toHaveBeenCalledTimes(1);
    expect(mockPush).toHaveBeenCalledWith("/login");
  });

  it("exposes setAuth from store", () => {
    (useAuthStore as jest.Mock).mockReturnValue({
      ...mockStore,
      setAuth: mockSetAuth,
    });

    const { result } = renderHook(() => useAuth());

    expect(result.current.setAuth).toBe(mockSetAuth);
  });
});
