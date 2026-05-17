export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-slate-400">Welcome back! Here&apos;s what&apos;s happening with your content.</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {[
          { label: "Videos Uploaded", value: "0", change: "+0 this week" },
          { label: "Clips Generated", value: "0", change: "+0 this week" },
          { label: "Total Views", value: "0", change: "Across all platforms" },
          { label: "Posts Scheduled", value: "0", change: "Upcoming" },
        ].map((stat) => (
          <div key={stat.label} className="bg-slate-800 border border-slate-700 rounded-xl p-5">
            <p className="text-sm text-slate-400">{stat.label}</p>
            <p className="text-3xl font-bold text-white mt-1">{stat.value}</p>
            <p className="text-xs text-slate-500 mt-1">{stat.change}</p>
          </div>
        ))}
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl p-8 text-center">
        <h3 className="text-lg font-semibold text-white mb-2">Upload your first video</h3>
        <p className="text-slate-400 mb-4">Start by uploading a long-form video and let our AI do the work.</p>
        <a
          href="/videos/upload"
          className="inline-block bg-purple-600 hover:bg-purple-700 px-6 py-3 rounded-lg font-medium text-white transition-colors"
        >
          Upload Video
        </a>
      </div>
    </div>
  );
}
