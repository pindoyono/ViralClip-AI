export default function VideosPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Videos</h1>
          <p className="text-slate-400">Manage your uploaded videos and generated clips.</p>
        </div>
        <a
          href="/videos/upload"
          className="bg-purple-600 hover:bg-purple-700 px-4 py-2 rounded-lg font-medium text-white transition-colors text-sm"
        >
          Upload Video
        </a>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl">
        <div className="p-6 text-center text-slate-400">
          <p className="text-lg font-medium text-slate-300">No videos yet</p>
          <p className="mt-1 text-sm">Upload your first video to get started.</p>
        </div>
      </div>
    </div>
  );
}
