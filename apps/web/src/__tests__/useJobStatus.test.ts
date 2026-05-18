/**
 * Tests for useJobStatus and useVideoProcessingWS hooks.
 */
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock the axios api module used by hooks.
jest.mock("@/lib/api", () => ({
  api: {
    get: jest.fn(),
  },
}));

import { api } from "@/lib/api";
import { useJobStatus } from "@/hooks/useJobStatus";
import type { JobStatusResponse } from "@/types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false, refetchInterval: false },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  };
}

const PENDING_STATUS: JobStatusResponse = {
  video_id: "vid-1",
  video_status: "pending",
  job_status: "",
  current_stage: "transcript",
  stages: [
    { stage: "transcript", status: "pending", label: "Transcription" },
    { stage: "clip", status: "pending", label: "Clip Generation" },
    { stage: "subtitle", status: "pending", label: "Subtitle Burning" },
    { stage: "upload", status: "pending", label: "Finalising" },
  ],
};

const PROCESSING_STATUS: JobStatusResponse = {
  video_id: "vid-1",
  video_status: "processing",
  job_status: "clip:processing",
  current_stage: "clip",
  stages: [
    { stage: "transcript", status: "done", label: "Transcription" },
    { stage: "clip", status: "processing", label: "Clip Generation" },
    { stage: "subtitle", status: "pending", label: "Subtitle Burning" },
    { stage: "upload", status: "pending", label: "Finalising" },
  ],
};

const COMPLETED_STATUS: JobStatusResponse = {
  video_id: "vid-1",
  video_status: "completed",
  job_status: "upload:done",
  current_stage: "completed",
  stages: [
    { stage: "transcript", status: "done", label: "Transcription" },
    { stage: "clip", status: "done", label: "Clip Generation" },
    { stage: "subtitle", status: "done", label: "Subtitle Burning" },
    { stage: "upload", status: "done", label: "Finalising" },
  ],
};

// ---------------------------------------------------------------------------
// useJobStatus tests
// ---------------------------------------------------------------------------

describe("useJobStatus", () => {
  afterEach(() => jest.clearAllMocks());

  it("returns undefined when videoId is not provided", () => {
    const { result } = renderHook(() => useJobStatus(undefined), {
      wrapper: createWrapper(),
    });
    expect(result.current.data).toBeUndefined();
    expect(api.get).not.toHaveBeenCalled();
  });

  it("fetches job status for the given videoId", async () => {
    (api.get as jest.Mock).mockResolvedValueOnce({
      data: { data: PENDING_STATUS },
    });

    const { result } = renderHook(() => useJobStatus("vid-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(api.get).toHaveBeenCalledWith("/api/v1/videos/vid-1/job-status");
    expect(result.current.data).toEqual(PENDING_STATUS);
  });

  it("returns processing stage info correctly", async () => {
    (api.get as jest.Mock).mockResolvedValueOnce({
      data: { data: PROCESSING_STATUS },
    });

    const { result } = renderHook(() => useJobStatus("vid-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.current_stage).toBe("clip");
    expect(result.current.data?.stages[0].status).toBe("done");
    expect(result.current.data?.stages[1].status).toBe("processing");
  });

  it("returns completed status correctly", async () => {
    (api.get as jest.Mock).mockResolvedValueOnce({
      data: { data: COMPLETED_STATUS },
    });

    const { result } = renderHook(() => useJobStatus("vid-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.video_status).toBe("completed");
    expect(result.current.data?.current_stage).toBe("completed");
    for (const s of result.current.data!.stages) {
      expect(s.status).toBe("done");
    }
  });

  it("handles API error gracefully", async () => {
    (api.get as jest.Mock).mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useJobStatus("vid-error"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});
