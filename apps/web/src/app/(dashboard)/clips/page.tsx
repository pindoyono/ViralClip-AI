"use client";

import { useClips, useDeleteClip } from "@/hooks/useClips";
import { formatDuration } from "@/lib/utils";

const STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  processing: "text-blue-400 bg-blue-400/10",
  ready: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
};

export default function ClipsPage() {
  const { data, isLoading } = useClips();
  const deleteClip = useDeleteClip();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Clips</h1>
        <p className="text-slate-400">All AI-generated clips from your videos.</p>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl">
        {isLoading ? (
          <div className="p-6 text-center text-slate-400">Loading…</div>
        ) : !data?.data?.length ? (
          <div className="p-6 text-center text-slate-400">
            <p className="text-lg font-medium text-slate-300">No clips yet</p>
            <p className="mt-1 text-sm">
              Upload a video to generate your first viral clips.
            </p>
          </div>
        ) : (
          <ul className="divide-y divide-slate-700">
            {data.data.map((clip) => (
              <li
                key={clip.id}
                className="flex items-center gap-4 px-6 py-4"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-white font-medium truncate">{clip.title}</p>
                  <p className="text-xs text-slate-500 mt-0.5">
                    {formatDuration(clip.duration)} ·{" "}
                    Viral score: {Math.round(clip.viral_score * 100)}%
                  </p>
                  {clip.hashtags?.length > 0 && (
                    <p className="text-xs text-purple-400 mt-1 truncate">
                      {clip.hashtags.slice(0, 5).join(" ")}
                    </p>
                  )}
                </div>

                <span
                  className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize shrink-0 ${
                    STATUS_COLORS[clip.status] ?? "text-slate-400"
                  }`}
                >
                  {clip.status}
                </span>

                <button
                  onClick={() => deleteClip.mutate(clip.id)}
                  disabled={deleteClip.isPending}
                  className="text-xs text-slate-400 hover:text-red-400 disabled:opacity-40 transition-colors shrink-0"
                >
                  Delete
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

