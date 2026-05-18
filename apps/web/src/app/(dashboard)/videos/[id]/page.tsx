"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useVideo, useDeleteVideo, useProcessVideo } from "@/hooks/useVideos";
import { useClips } from "@/hooks/useClips";
import { formatBytes, formatDuration } from "@/lib/utils";

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

export default function VideoDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data: video, isLoading: videoLoading } = useVideo(id);
  const { data: clipsData, isLoading: clipsLoading } = useClips(id);
  const deleteVideo = useDeleteVideo();
  const processVideo = useProcessVideo();

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

                  <span
                    className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize shrink-0 ${
                      CLIP_STATUS_COLORS[clip.status] ?? "text-slate-400"
                    }`}
                  >
                    {clip.status}
                  </span>

                  {clip.status === "ready" && (
                    <Link
                      href={`/schedule?clip_id=${clip.id}`}
                      className="text-xs bg-purple-600 hover:bg-purple-700 px-3 py-1.5 rounded-lg text-white transition-colors shrink-0"
                    >
                      Schedule
                    </Link>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}
