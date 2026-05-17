"use client";

import { useRouter } from "next/navigation";
import { apiClient } from "@/lib/api";
import { useAuthStore } from "@/lib/store";

export function useAuth() {
  const router = useRouter();
  const { user, token, isAuthenticated, setAuth, clearAuth } = useAuthStore();

  const logout = async () => {
    try {
      await apiClient.post("/auth/logout");
    } catch {
      // ignore errors on logout
    } finally {
      clearAuth();
      router.push("/login");
    }
  };

  return { user, token, isAuthenticated, setAuth, logout };
}
