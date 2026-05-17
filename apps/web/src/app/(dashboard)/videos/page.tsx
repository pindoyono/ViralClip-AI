"use client";

import Link from "next/link";
import { useVideos, useDeleteVideo, useProcessVideo } from "@/hooks/useVideos";
import { formatBytes } from "@/lib/utils";

const STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  processing: "text-blue-400 bg-blue-400/10",
  completed: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
};

export default function VideosPage() {
  const { data, isLoading } = useVideos();
  const deleteVideo = useDeleteVideo();
  const processVideo = useProcessVideo();

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Videos</h1>
          <p className="text-slate-400">
            Manage your uploaded videos and generated clips.
          </p>
        </div>
        <Link
          href="/videos/upload"
          className="bg-purple-600 hover:bg-purple-700 px-4 py-2 rounded-lg font-medium text-white transition-colors text-sm"
        >
          Upload Video
        </Link>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl">
        {isLoading ? (
          <div className="p-6 text-center text-slate-400">Loading…</div>
        ) : !data?.videos?.length ? (
          <div className="p-6 text-center text-slate-400">
            <p className="text-lg font-medium text-slate-300">No videos yet</p>
            <p className="mt-1 text-sm">Upload your first video to get started.</p>
          </div>
        ) : (
          <ul className="divide-y divide-slate-700">
            {data.videos.map((video) => (
              <li
                key={video.id}
                className="flex items-center justify-between px-6 py-4 gap-4"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-white font-medium truncate">{video.title}</p>
                  <p className="text-xs text-slate-500 mt-0.5">
                    {formatBytes(video.file_size)} ·{" "}
                    {new Date(video.created_at).toLocaleDateString()}
                  </p>
                </div>

                <span
                  className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize ${
                    STATUS_COLORS[video.status] ?? "text-slate-400"
                  }`}
                >
                  {video.status}
                </span>

                <div className="flex items-center gap-2 shrink-0">
                  {(video.status === "pending" || video.status === "failed") && (
                    <button
                      onClick={() => processVideo.mutate(video.id)}
                      disabled={processVideo.isPending}
                      className="text-xs bg-purple-600 hover:bg-purple-700 disabled:opacity-50 px-3 py-1.5 rounded-lg text-white transition-colors"
                    >
                      Process
                    </button>
                  )}
                  <button
                    onClick={() => deleteVideo.mutate(video.id)}
                    disabled={deleteVideo.isPending || video.status === "processing"}
                    className="text-xs text-slate-400 hover:text-red-400 disabled:opacity-40 transition-colors"
                  >
                    Delete
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

