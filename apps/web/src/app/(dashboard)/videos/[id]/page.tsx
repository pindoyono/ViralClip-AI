"use client";

import { useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { useVideo, useDeleteVideo, useProcessVideo } from "@/hooks/useVideos";
import { useClips } from "@/hooks/useClips";
import { useHookDetections, useDetectHooks } from "@/hooks/useHooksV2";
import { useGenerateClipsV2 } from "@/hooks/useClipsV2";
import { useBurnSubtitles } from "@/hooks/useSubtitles";
import { useJobStatus, useVideoProcessingWS } from "@/hooks/useJobStatus";
import { formatBytes, formatDuration } from "@/lib/utils";
import type {
  ClipV2ProfileType,
  ClipV2ResultItem,
  JobStatusResponse,
  PipelineStageInfo,
  SubtitleStyle,
} from "@/types";

const VIDEO_STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  processing: "text-blue-400 bg-blue-400/10",
  completed: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
};

const CLIP_STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  processing: "text-blue-400 bg-blue-400/10",
  ready: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
};

const PROFILE_TYPES: { value: ClipV2ProfileType; label: string }[] = [
  { value: "general", label: "General" },
  { value: "gaming", label: "Gaming" },
  { value: "comedy", label: "Comedy" },
  { value: "education", label: "Education" },
  { value: "politics", label: "Politics" },
  { value: "podcast", label: "Podcast" },
];

function ScoreBar({ label, value }: { label: string; value: number }) {
  const pct = Math.round(value * 100);
  return (
    <div className="flex items-center gap-2 text-xs">
      <span className="w-20 shrink-0 text-slate-400">{label}</span>
      <div className="flex-1 bg-slate-700 rounded-full h-1.5">
        <div
          className="bg-purple-500 h-1.5 rounded-full"
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="w-8 text-right text-slate-300">{pct}%</span>
    </div>
  );
}

function ClipV2Card({ clip, index }: { clip: ClipV2ResultItem; index: number }) {
  return (
    <div className="bg-slate-750 border border-slate-600 rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-white font-medium text-sm">Clip #{index + 1}</span>
        <span className="text-xs bg-purple-600/20 text-purple-300 px-2 py-0.5 rounded-full font-medium">
          Score {clip.score}/100
        </span>
      </div>
      <p className="text-xs text-slate-400">
        {clip.start} → {clip.end}{" "}
        <span className="text-slate-500">
          ({Math.round(clip.end_seconds - clip.start_seconds)}s)
        </span>
      </p>
      <div className="space-y-1.5">
        <ScoreBar label="Hook" value={clip.hook_score} />
        <ScoreBar label="Emotion" value={clip.emotion_score} />
        <ScoreBar label="Story" value={clip.story_score} />
        <ScoreBar label="Retention" value={clip.retention_score} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ProcessingPipeline — real-time pipeline progress bar
// ---------------------------------------------------------------------------

const STAGE_ICONS: Record<string, string> = {
  transcript: "🎙",
  clip: "✂️",
  subtitle: "💬",
  upload: "☁️",
};

function StageStep({ info }: { info: PipelineStageInfo }) {
  const isProcessing = info.status === "processing";
  const isDone = info.status === "done";
  const isFailed = info.status === "failed";

  const ringColor = isDone
    ? "bg-green-500 border-green-500"
    : isFailed
    ? "bg-red-500 border-red-500"
    : isProcessing
    ? "border-blue-400 bg-blue-400/20"
    : "border-slate-600 bg-transparent";

  const labelColor = isFailed
    ? "text-red-400"
    : isProcessing
    ? "text-blue-400"
    : isDone
    ? "text-green-400"
    : "text-slate-500";

  return (
    <div className="flex flex-col items-center gap-1 flex-1 min-w-0">
      <div
        className={`w-8 h-8 rounded-full border-2 flex items-center justify-center text-sm transition-all ${ringColor}`}
        aria-label={info.label}
      >
        {isDone ? "✓" : isFailed ? "✗" : STAGE_ICONS[info.stage] ?? "·"}
      </div>
      <span className={`text-xs text-center truncate w-full ${labelColor}`}>
        {info.label}
      </span>
      {isProcessing && (
        <span className="text-xs text-blue-400 animate-pulse">running…</span>
      )}
    </div>
  );
}

function ProcessingPipeline({
  status,
  wsConnected,
}: {
  status: JobStatusResponse;
  wsConnected: boolean;
}) {
  return (
    <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-white">
          Processing Pipeline
        </h2>
        <div className="flex items-center gap-1.5 text-xs text-slate-400">
          <span
            className={`w-2 h-2 rounded-full ${
              wsConnected ? "bg-green-400" : "bg-slate-600"
            }`}
          />
          {wsConnected ? "Live" : "Polling"}
        </div>
      </div>

      <div className="flex items-start gap-2">
        {status.stages.map((s, i) => (
          <div key={s.stage} className="flex items-start flex-1 min-w-0">
            <StageStep info={s} />
            {i < status.stages.length - 1 && (
              <div className="h-0.5 flex-1 bg-slate-600 mt-4 mx-1 shrink-0" />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

export default function VideoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();

  const [profileType, setProfileType] = useState<ClipV2ProfileType>("general");
  const [v2Clips, setV2Clips] = useState<ClipV2ResultItem[]>([]);
  const [v2ProfileUsed, setV2ProfileUsed] = useState("");
  const [subtitleStyle, setSubtitleStyle] = useState<SubtitleStyle>("default");
  const [subtitleFontSize, setSubtitleFontSize] = useState(24);

  const { data: video, isLoading: videoLoading } = useVideo(id);
  const { data: clipsData, isLoading: clipsLoading } = useClips(id);
  const { data: hookData } = useHookDetections(id);
  const detectHooks = useDetectHooks(id);
  const generateClipsV2 = useGenerateClipsV2(id);
  const burnSubtitles = useBurnSubtitles(id);
  const deleteVideo = useDeleteVideo();
  const processVideo = useProcessVideo();

  // Job status polling (HTTP fallback).
  const { data: jobStatus } = useJobStatus(
    video?.status === "processing" ? id : undefined
  );

  // Real-time WebSocket updates.
  const handleWSUpdate = useCallback(
    (payload: JobStatusResponse) => {
      // Invalidate queries so the UI reflects the completed state immediately.
      if (payload.video_status === "completed") {
        queryClient.invalidateQueries({ queryKey: ["video", id] });
        queryClient.invalidateQueries({ queryKey: ["clips", id] });
      }
      queryClient.setQueryData(["job-status", id], payload);
    },
    [id, queryClient]
  );

  const { connected: wsConnected } = useVideoProcessingWS(
    video?.status === "processing" ? id : undefined,
    { onUpdate: handleWSUpdate }
  );

  if (videoLoading) {
    return (
      <div className="p-6 text-center text-slate-400">Loading video…</div>
    );
  }

  if (!video) {
    return (
      <div className="space-y-4">
        <div className="p-6 text-center text-slate-400">Video not found.</div>
        <div className="text-center">
          <Link href="/videos" className="text-purple-400 hover:text-purple-300 text-sm transition-colors">
            ← Back to Videos
          </Link>
        </div>
      </div>
    );
  }

  const clips = (clipsData as { data?: unknown[] } | undefined)?.data ?? [];

  // hasTranscript is true when the video has a transcribed text that can be
  // used as input to the V2 clip generation pipeline.
  const hasTranscript = !!(video.transcript && video.transcript.trim().length > 0);

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div>
        <Link
          href="/videos"
          className="text-sm text-purple-400 hover:text-purple-300 transition-colors"
        >
          ← Back to Videos
        </Link>
      </div>

      {/* Video Info */}
      <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1 min-w-0">
            <h1 className="text-2xl font-bold text-white truncate">{video.title}</h1>
            {video.description && (
              <p className="text-slate-400 mt-1 text-sm">{video.description}</p>
            )}
          </div>
          <span
            className={`text-xs font-medium px-3 py-1 rounded-full capitalize shrink-0 ${
              VIDEO_STATUS_COLORS[video.status] ?? "text-slate-400"
            }`}
          >
            {video.status}
          </span>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 pt-2">
          <div>
            <p className="text-xs text-slate-500 uppercase tracking-wide">Duration</p>
            <p className="text-white font-medium mt-0.5">{formatDuration(video.duration)}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500 uppercase tracking-wide">File Size</p>
            <p className="text-white font-medium mt-0.5">{formatBytes(video.file_size)}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500 uppercase tracking-wide">Uploaded</p>
            <p className="text-white font-medium mt-0.5">
              {new Date(video.created_at).toLocaleDateString()}
            </p>
          </div>
          <div>
            <p className="text-xs text-slate-500 uppercase tracking-wide">Clips</p>
            <p className="text-white font-medium mt-0.5">{clips.length}</p>
          </div>
        </div>

        <div className="flex gap-3 pt-2 flex-wrap">
          {(video.status === "pending" || video.status === "failed") && (
            <button
              onClick={() => processVideo.mutate(video.id)}
              disabled={processVideo.isPending}
              className="bg-purple-600 hover:bg-purple-700 disabled:opacity-50 px-4 py-2 rounded-lg text-white text-sm font-medium transition-colors"
            >
              {processVideo.isPending ? "Processing…" : "Process with AI"}
            </button>
          )}
          <button
            onClick={async () => {
              if (!confirm("Delete this video and all its clips?")) return;
              await deleteVideo.mutateAsync(video.id);
              router.push("/videos");
            }}
            disabled={deleteVideo.isPending || video.status === "processing"}
            className="border border-red-700 text-red-400 hover:bg-red-900/30 disabled:opacity-40 px-4 py-2 rounded-lg text-sm font-medium transition-colors"
          >
            Delete Video
          </button>
        </div>
      </div>

      {/* Processing Pipeline (visible while video is being processed) */}
      {video.status === "processing" && jobStatus && (
        <ProcessingPipeline status={jobStatus} wsConnected={wsConnected} />
      )}

      {/* Clips Section */}
      <div className="space-y-3">
        <h2 className="text-lg font-semibold text-white">Generated Clips</h2>

        <div className="bg-slate-800 border border-slate-700 rounded-xl">
          {clipsLoading ? (
            <div className="p-6 text-center text-slate-400">Loading clips…</div>
          ) : !clips.length ? (
            <div className="p-6 text-center text-slate-400">
              <p className="text-slate-300 font-medium">No clips yet</p>
              <p className="mt-1 text-sm">
                {video.status === "completed"
                  ? "No clips were generated for this video."
                  : "Process the video to generate AI-powered viral clips."}
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-slate-700">
              {(clips as Array<{
                id: string;
                title: string;
                duration: number;
                viral_score: number;
                hashtags?: string[];
                hook_text?: string;
                status: string;
                has_subtitles?: boolean;
              }>).map((clip) => (
                <li key={clip.id} className="flex items-center gap-4 px-6 py-4">
                  <div className="flex-1 min-w-0">
                    <p className="text-white font-medium truncate">{clip.title}</p>
                    {clip.hook_text && (
                      <p className="text-xs text-purple-300 mt-0.5 truncate italic">
                        &ldquo;{clip.hook_text}&rdquo;
                      </p>
                    )}
                    <p className="text-xs text-slate-500 mt-0.5">
                      {formatDuration(clip.duration)} · Viral score:{" "}
                      <span className="text-purple-400 font-medium">
                        {Math.round(clip.viral_score * 100)}%
                      </span>
                    </p>
                    {clip.hashtags && clip.hashtags.length > 0 && (
                      <p className="text-xs text-slate-500 mt-1 truncate">
                        {clip.hashtags.slice(0, 4).join(" ")}
                      </p>
                    )}
                  </div>

                  <div className="flex items-center gap-2 shrink-0 flex-wrap">
                    {clip.has_subtitles && (
                      <span className="text-xs bg-cyan-600/20 text-cyan-300 px-2 py-0.5 rounded-full font-medium">
                        CC
                      </span>
                    )}
                    <span
                      className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize ${
                        CLIP_STATUS_COLORS[clip.status] ?? "text-slate-400"
                      }`}
                    >
                      {clip.status}
                    </span>
                    {clip.status === "ready" && (
                      <Link
                        href={`/schedule?clip_id=${clip.id}`}
                        className="text-xs bg-purple-600 hover:bg-purple-700 px-3 py-1.5 rounded-lg text-white transition-colors"
                      >
                        Schedule
                      </Link>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Subtitle Burning Section */}
      {video.status === "completed" && clips.length > 0 && (
        <div className="space-y-3">
          <div>
            <h2 className="text-lg font-semibold text-white">
              Subtitle Burning{" "}
              <span className="text-xs font-normal bg-cyan-700/30 text-cyan-300 px-2 py-0.5 rounded-full ml-1">
                CC
              </span>
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Burn captions into your clips so they display on every platform without manual editing.
            </p>
          </div>

          <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">
            <div className="flex flex-wrap gap-3 items-end">
              {/* Style selector */}
              <div>
                <label className="block text-xs text-slate-400 mb-1.5">Style</label>
                <select
                  value={subtitleStyle}
                  onChange={(e) => setSubtitleStyle(e.target.value as SubtitleStyle)}
                  className="bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:ring-2 focus:ring-cyan-500"
                >
                  <option value="default">Default</option>
                  <option value="bold">Bold</option>
                  <option value="outline">Outline</option>
                  <option value="shadow">Shadow</option>
                </select>
              </div>

              {/* Font size */}
              <div>
                <label className="block text-xs text-slate-400 mb-1.5">
                  Font Size <span className="text-slate-500">({subtitleFontSize}pt)</span>
                </label>
                <input
                  type="range"
                  min={12}
                  max={72}
                  step={2}
                  value={subtitleFontSize}
                  onChange={(e) => setSubtitleFontSize(Number(e.target.value))}
                  className="w-28 accent-cyan-500"
                />
              </div>

              <button
                onClick={() =>
                  burnSubtitles.mutate({
                    style: subtitleStyle,
                    font_size: subtitleFontSize,
                  })
                }
                disabled={burnSubtitles.isPending}
                className="bg-cyan-700 hover:bg-cyan-600 disabled:opacity-50 disabled:cursor-not-allowed px-4 py-2 rounded-lg text-white text-sm font-medium transition-colors shrink-0"
              >
                {burnSubtitles.isPending ? "Burning…" : "Burn Subtitles"}
              </button>
            </div>

            {burnSubtitles.isError && (
              <p className="text-xs text-red-400 bg-red-400/10 px-3 py-2 rounded-lg">
                Failed to burn subtitles. Make sure the AI service is running and the video has been
                processed.
              </p>
            )}

            {burnSubtitles.isSuccess && (
              <p className="text-xs text-cyan-400 bg-cyan-400/10 px-3 py-2 rounded-lg">
                ✓ Subtitles burned into{" "}
                <span className="font-semibold">
                  {burnSubtitles.data?.clips_processed ?? 0} clip
                  {(burnSubtitles.data?.clips_processed ?? 0) !== 1 ? "s" : ""}
                </span>
                . The <span className="font-semibold text-cyan-300">CC</span> badge will appear on
                each clip above.
              </p>
            )}

            {!burnSubtitles.isPending && !burnSubtitles.isError && !burnSubtitles.isSuccess && (
              <p className="text-xs text-slate-500 text-center py-2">
                Select a subtitle style and click <em>Burn Subtitles</em> to burn captions into all
                extracted clips.
              </p>
            )}
          </div>
        </div>
      )}

      {/* V2 Clip Generator Section */}      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-white">
              V2 Clip Generator{" "}
              <span className="text-xs font-normal bg-purple-700/30 text-purple-300 px-2 py-0.5 rounded-full ml-1">
                Beta
              </span>
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Profile-aware scoring: Hook×50% + Emotion×20% + Story×20% + Retention×10%
            </p>
          </div>
          {hookData && hookData.total > 0 && (
            <span className="text-xs text-green-400 bg-green-400/10 px-2 py-0.5 rounded-full">
              {hookData.total} hook{hookData.total !== 1 ? "s" : ""} detected
            </span>
          )}
        </div>

        <div className="bg-slate-800 border border-slate-700 rounded-xl p-5 space-y-4">
          <div className="flex flex-wrap gap-3 items-end">
            <div className="flex-1 min-w-[160px]">
              <label className="block text-xs text-slate-400 mb-1.5">
                Content Profile
              </label>
              <select
                value={profileType}
                onChange={(e) =>
                  setProfileType(e.target.value as ClipV2ProfileType)
                }
                className="w-full bg-slate-700 border border-slate-600 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
              >
                {PROFILE_TYPES.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>
            </div>

            <button
              onClick={() => {
                if (!hasTranscript) return;
                // Split the stored transcript into ~30-second segments for the
                // V2 engine. Each line of the stored transcript becomes one
                // segment; timestamps are approximated uniformly across the
                // video duration.
                const lines = (video.transcript as string)
                  .split(/\n+/)
                  .map((t) => t.trim())
                  .filter(Boolean);
                const totalDuration = video.duration ?? 60;
                const segDuration = lines.length > 0 ? totalDuration / lines.length : totalDuration;
                const segments = lines.map((text, i) => ({
                  text,
                  start: Math.round(i * segDuration * 10) / 10,
                  end: Math.round((i + 1) * segDuration * 10) / 10,
                }));
                generateClipsV2.mutate(
                  {
                    segments,
                    profile_type: profileType,
                    min_clip_score: 50,
                    max_clips: 10,
                  },
                  {
                    onSuccess: (data) => {
                      setV2Clips(data.clips);
                      setV2ProfileUsed(data.profile_type);
                    },
                  }
                );
              }}
              disabled={generateClipsV2.isPending || !hasTranscript}
              title={!hasTranscript ? "Process the video first to generate a transcript" : undefined}
              className="bg-purple-600 hover:bg-purple-700 disabled:opacity-50 disabled:cursor-not-allowed px-4 py-2 rounded-lg text-white text-sm font-medium transition-colors shrink-0"
            >
              {generateClipsV2.isPending ? "Generating…" : "Generate V2 Clips"}
            </button>
          </div>

          {generateClipsV2.isError && (
            <p className="text-xs text-red-400 bg-red-400/10 px-3 py-2 rounded-lg">
              Failed to generate clips. Make sure the AI service is running.
            </p>
          )}

          {v2Clips.length > 0 && (
            <div className="space-y-3">
              <p className="text-xs text-slate-400">
                {v2Clips.length} clip{v2Clips.length !== 1 ? "s" : ""} generated
                {v2ProfileUsed && ` · profile: ${v2ProfileUsed}`}
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {v2Clips.map((clip, i) => (
                  <ClipV2Card key={i} clip={clip} index={i} />
                ))}
              </div>
            </div>
          )}

          {v2Clips.length === 0 && !generateClipsV2.isPending && !generateClipsV2.isError && (
            <p className="text-xs text-slate-500 text-center py-2">
              {hasTranscript
                ? <>Select a content profile and click <em>Generate V2 Clips</em> to run the Dynamic Clip Engine.</>
                : "Process the video with AI first to generate a transcript, then you can run V2 Clip Generation."}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
