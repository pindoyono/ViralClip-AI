"use client";

import Link from "next/link";
import { useVideos } from "@/hooks/useVideos";
import { useClips } from "@/hooks/useClips";
import { useAnalyticsSummary } from "@/hooks/useAnalytics";
import { useScheduledPosts } from "@/hooks/useSchedule";

export default function DashboardPage() {
  const { data: videosData } = useVideos(1, 1);
  const { data: clipsData } = useClips(undefined, 1, 1);
  const { data: analytics } = useAnalyticsSummary();
  const { data: postsData } = useScheduledPosts(1, 1);

  const stats = [
    {
      label: "Videos Uploaded",
      value: String(videosData?.total ?? 0),
      change: "+0 this week",
    },
    {
      label: "Clips Generated",
      value: String(clipsData?.total ?? 0),
      change: "+0 this week",
    },
    {
      label: "Total Views",
      value: analytics?.total_views?.toLocaleString() ?? "0",
      change: "Across all platforms",
    },
    {
      label: "Posts Scheduled",
      value: String(postsData?.total ?? 0),
      change: "Upcoming",
    },
  ];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-slate-400">
          Welcome back! Here&apos;s what&apos;s happening with your content.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((stat) => (
          <div
            key={stat.label}
            className="bg-slate-800 border border-slate-700 rounded-xl p-5"
          >
            <p className="text-sm text-slate-400">{stat.label}</p>
            <p className="text-3xl font-bold text-white mt-1">{stat.value}</p>
            <p className="text-xs text-slate-500 mt-1">{stat.change}</p>
          </div>
        ))}
      </div>

      {(videosData?.total ?? 0) === 0 ? (
        <div className="bg-slate-800 border border-slate-700 rounded-xl p-8 text-center">
          <h3 className="text-lg font-semibold text-white mb-2">
            Upload your first video
          </h3>
          <p className="text-slate-400 mb-4">
            Start by uploading a long-form video and let our AI do the work.
          </p>
          <Link
            href="/videos/upload"
            className="inline-block bg-purple-600 hover:bg-purple-700 px-6 py-3 rounded-lg font-medium text-white transition-colors"
          >
            Upload Video
          </Link>
        </div>
      ) : (
        <div className="bg-slate-800 border border-slate-700 rounded-xl p-6">
          <h3 className="text-base font-semibold text-white mb-3">
            Recent Activity
          </h3>
          <div className="flex gap-4">
            <Link
              href="/videos"
              className="text-sm text-purple-400 hover:text-purple-300 transition-colors"
            >
              View all videos →
            </Link>
            <Link
              href="/clips"
              className="text-sm text-purple-400 hover:text-purple-300 transition-colors"
            >
              View all clips →
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}
