export default function AnalyticsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Analytics</h1>
        <p className="text-slate-400">Track performance across all your published clips.</p>
      </div>
      <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
        <p className="text-lg font-medium text-slate-300">No analytics yet</p>
        <p className="mt-1 text-sm">Publish clips to start seeing performance data.</p>
      </div>
    </div>
  );
}
