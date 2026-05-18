"use client";

import { useState } from "react";
import { useAnalyticsSummary, useTrendingTopics } from "@/hooks/useAnalytics";

const PLATFORMS = ["all", "tiktok", "youtube", "instagram"] as const;
type Platform = (typeof PLATFORMS)[number];

const PLATFORM_LABELS: Record<Platform, string> = {
  all: "All Platforms",
  tiktok: "TikTok",
  youtube: "YouTube",
  instagram: "Instagram",
};

export default function AnalyticsPage() {
  const { data, isLoading } = useAnalyticsSummary();
  const [trendPlatform, setTrendPlatform] = useState<Platform>("all");
  const { data: topics, isLoading: topicsLoading } = useTrendingTopics(
    trendPlatform === "all" ? undefined : trendPlatform
  );

  const stats = data
    ? [
        { label: "Total Views", value: data.total_views.toLocaleString() },
        { label: "Total Likes", value: data.total_likes.toLocaleString() },
        { label: "Total Shares", value: data.total_shares.toLocaleString() },
        {
          label: "Avg Engagement Rate",
          value: `${(data.avg_engagement_rate * 100).toFixed(1)}%`,
        },
        { label: "Top Platform", value: data.top_platform || "—" },
        {
          label: "Clips Published",
          value: String(data.clips_published),
        },
      ]
    : [];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Analytics</h1>
        <p className="text-slate-400">
          Track performance across all your published clips.
        </p>
      </div>

      {/* Performance Summary */}
      {isLoading ? (
        <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
          Loading…
        </div>
      ) : !data || data.clips_published === 0 ? (
        <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
          <p className="text-lg font-medium text-slate-300">No analytics yet</p>
          <p className="mt-1 text-sm">
            Publish clips to start seeing performance data.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
          {stats.map((s) => (
            <div
              key={s.label}
              className="bg-slate-800 border border-slate-700 rounded-xl p-5"
            >
              <p className="text-sm text-slate-400">{s.label}</p>
              <p className="text-2xl font-bold text-white mt-1">{s.value}</p>
            </div>
          ))}
        </div>
      )}

      {/* Trending Topics */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold text-white">Trending Topics</h2>
            <p className="text-sm text-slate-400">
              Platform-wide trends to align your next clip.
            </p>
          </div>
          <div className="flex gap-1">
            {PLATFORMS.map((p) => (
              <button
                key={p}
                onClick={() => setTrendPlatform(p)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                  trendPlatform === p
                    ? "bg-purple-600 text-white"
                    : "bg-slate-700 text-slate-300 hover:bg-slate-600"
                }`}
              >
                {PLATFORM_LABELS[p]}
              </button>
            ))}
          </div>
        </div>

        {topicsLoading ? (
          <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
            Loading trends…
          </div>
        ) : !topics || topics.length === 0 ? (
          <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
            <p className="text-lg font-medium text-slate-300">No trends available</p>
            <p className="mt-1 text-sm">Trending data is updated periodically.</p>
          </div>
        ) : (
          <div className="bg-slate-800 border border-slate-700 rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-700 text-slate-400 text-xs uppercase">
                  <th className="px-4 py-3 text-left">Topic</th>
                  <th className="px-4 py-3 text-left">Hashtag</th>
                  <th className="px-4 py-3 text-left">Platform</th>
                  <th className="px-4 py-3 text-right">Trend Score</th>
                  <th className="px-4 py-3 text-right">Growth</th>
                  <th className="px-4 py-3 text-right">Posts</th>
                </tr>
              </thead>
              <tbody>
                {topics.map((t) => (
                  <tr
                    key={t.id}
                    className="border-b border-slate-700/50 last:border-0 hover:bg-slate-700/30 transition-colors"
                  >
                    <td className="px-4 py-3 text-white font-medium">{t.topic}</td>
                    <td className="px-4 py-3 text-purple-400">
                      {t.hashtag ? `#${t.hashtag}` : "—"}
                    </td>
                    <td className="px-4 py-3 text-slate-300 capitalize">{t.platform}</td>
                    <td className="px-4 py-3 text-right">
                      <span className="inline-flex items-center gap-1">
                        <span className="text-white font-semibold">
                          {t.trend_score.toFixed(1)}
                        </span>
                        <span className="text-slate-400">/100</span>
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <span
                        className={
                          t.growth_rate >= 0
                            ? "text-green-400"
                            : "text-red-400"
                        }
                      >
                        {t.growth_rate >= 0 ? "+" : ""}
                        {t.growth_rate.toFixed(1)}%
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right text-slate-300">
                      {t.post_count.toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
