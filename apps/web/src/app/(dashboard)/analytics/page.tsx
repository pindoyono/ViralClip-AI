"use client";

import { useAnalyticsSummary } from "@/hooks/useAnalytics";

export default function AnalyticsPage() {
  const { data, isLoading } = useAnalyticsSummary();

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
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Analytics</h1>
        <p className="text-slate-400">
          Track performance across all your published clips.
        </p>
      </div>

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
    </div>
  );
}

