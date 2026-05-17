import type { TranscriptSegment } from "@viralclip/shared-types";
import axios from "axios";

export interface TranscriptEngineConfig {
  aiServiceUrl: string;
  timeoutMs?: number;
}

export async function transcribeVideo(
  config: TranscriptEngineConfig,
  videoId: string,
  storagePath: string,
  language?: string
): Promise<{ language: string; duration: number; segments: TranscriptSegment[]; full_text: string }> {
  const response = await axios.post(
    `${config.aiServiceUrl}/api/v1/transcript`,
    { video_id: videoId, storage_path: storagePath, language },
    { timeout: config.timeoutMs ?? 600_000 }
  );
  return response.data;
}

export function segmentsToText(segments: TranscriptSegment[]): string {
  return segments.map((s) => s.text).join(" ").trim();
}

export function findSegmentsInRange(
  segments: TranscriptSegment[],
  startTime: number,
  endTime: number
): TranscriptSegment[] {
  return segments.filter((s) => s.start >= startTime && s.end <= endTime);
}
