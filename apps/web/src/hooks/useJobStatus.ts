"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { JobStatusResponse, WSMessage } from "@/types";

const POLL_INTERVAL_MS = 3000;
const TERMINAL_VIDEO_STATUSES = new Set(["completed", "failed"]);

/**
 * useJobStatus polls GET /api/v1/videos/:videoId/job-status every 3 s while
 * the video is in a non-terminal state. Returns the latest JobStatusResponse.
 */
export function useJobStatus(videoId: string | undefined) {
  return useQuery<JobStatusResponse>({
    queryKey: ["job-status", videoId],
    queryFn: async () => {
      const resp = await api.get<{ data: JobStatusResponse }>(
        `/api/v1/videos/${videoId}/job-status`
      );
      return resp.data.data;
    },
    enabled: !!videoId,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return POLL_INTERVAL_MS;
      return TERMINAL_VIDEO_STATUSES.has(data.video_status)
        ? false
        : POLL_INTERVAL_MS;
    },
    staleTime: 0,
  });
}

interface UseVideoProcessingWSOptions {
  /** Called each time the server pushes a status_update message. */
  onUpdate?: (payload: JobStatusResponse) => void;
}

/**
 * useVideoProcessingWS opens a WebSocket connection to /ws and subscribes
 * to real-time processing updates for videoId. It automatically closes the
 * socket when the video reaches a terminal state or the component unmounts.
 *
 * Returns `{ connected }` so callers can show connection state.
 */
export function useVideoProcessingWS(
  videoId: string | undefined,
  options: UseVideoProcessingWSOptions = {}
) {
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const { onUpdate } = options;

  useEffect(() => {
    if (!videoId) return;

    // Retrieve the JWT from localStorage (set by the auth store on login).
    const token =
      typeof window !== "undefined"
        ? localStorage.getItem("access_token") ?? ""
        : "";
    if (!token) return;

    const apiBase =
      process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
    const wsBase = apiBase.replace(/^http/, "ws");
    const url = `${wsBase}/ws?token=${encodeURIComponent(token)}`;

    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => setConnected(false);
    ws.onerror = () => setConnected(false);

    ws.onmessage = (event: MessageEvent) => {
      try {
        const msg: WSMessage = JSON.parse(event.data as string);
        if (
          msg.type === "status_update" &&
          msg.video_id === videoId &&
          msg.payload
        ) {
          onUpdate?.(msg.payload);

          // Close the socket once processing finishes.
          if (TERMINAL_VIDEO_STATUSES.has(msg.payload.video_status)) {
            ws.close();
          }
        }
      } catch {
        // Ignore malformed frames.
      }
    };

    return () => {
      ws.close();
    };
    // Re-open if videoId or token changes; onUpdate is intentionally excluded
    // from deps to avoid re-opening on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [videoId]);

  return { connected };
}
