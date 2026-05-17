export default function SchedulePage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Schedule</h1>
        <p className="text-slate-400">Schedule your clips to post at optimal times.</p>
      </div>
      <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
        <p className="text-lg font-medium text-slate-300">No scheduled posts</p>
        <p className="mt-1 text-sm">Connect a social account and schedule your first post.</p>
      </div>
    </div>
  );
}
