export default function ClipsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Clips</h1>
        <p className="text-slate-400">All AI-generated clips from your videos.</p>
      </div>
      <div className="bg-slate-800 border border-slate-700 rounded-xl p-6 text-center text-slate-400">
        <p className="text-lg font-medium text-slate-300">No clips yet</p>
        <p className="mt-1 text-sm">Upload a video to generate your first viral clips.</p>
      </div>
    </div>
  );
}
