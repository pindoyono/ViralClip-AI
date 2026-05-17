import Link from "next/link";

export default function HomePage() {
  return (
    <main className="min-h-screen bg-gradient-to-br from-slate-900 via-purple-900 to-slate-900 text-white">
      <nav className="flex items-center justify-between px-8 py-6 max-w-7xl mx-auto">
        <div className="flex items-center gap-2">
          <span className="text-2xl font-bold bg-gradient-to-r from-purple-400 to-pink-400 bg-clip-text text-transparent">
            ViralClip AI
          </span>
        </div>
        <div className="flex items-center gap-4">
          <Link href="/login" className="text-slate-300 hover:text-white transition-colors">
            Sign in
          </Link>
          <Link
            href="/register"
            className="bg-purple-600 hover:bg-purple-700 px-4 py-2 rounded-lg font-medium transition-colors"
          >
            Get started free
          </Link>
        </div>
      </nav>

      <section className="max-w-7xl mx-auto px-8 py-24 text-center">
        <h1 className="text-6xl font-extrabold mb-6 bg-gradient-to-r from-white via-purple-200 to-pink-200 bg-clip-text text-transparent">
          Turn Long Videos Into<br />Viral Short Clips
        </h1>
        <p className="text-xl text-slate-300 mb-10 max-w-2xl mx-auto">
          Upload any video. Our AI identifies the most engaging moments, generates clips with captions,
          and schedules them across TikTok, Reels, and YouTube Shorts — automatically.
        </p>
        <div className="flex justify-center gap-4">
          <Link
            href="/register"
            className="bg-purple-600 hover:bg-purple-700 px-8 py-4 rounded-xl text-lg font-semibold transition-colors"
          >
            Start for free
          </Link>
          <Link
            href="#features"
            className="border border-slate-600 hover:border-slate-400 px-8 py-4 rounded-xl text-lg font-medium transition-colors"
          >
            See how it works
          </Link>
        </div>
      </section>

      <section id="features" className="max-w-7xl mx-auto px-8 py-20">
        <h2 className="text-4xl font-bold text-center mb-16">Everything you need to go viral</h2>
        <div className="grid md:grid-cols-3 gap-8">
          {[
            { title: "AI Clip Detection", desc: "GPT-4 analyzes your transcript and identifies viral moments with precision." },
            { title: "Auto Captions", desc: "Whisper AI transcribes and burns animated captions into every clip." },
            { title: "Smart Scheduling", desc: "Post to TikTok, Instagram Reels, and YouTube Shorts at optimal times." },
          ].map((f) => (
            <div key={f.title} className="bg-white/5 backdrop-blur border border-white/10 rounded-2xl p-8">
              <h3 className="text-xl font-bold mb-3">{f.title}</h3>
              <p className="text-slate-400">{f.desc}</p>
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
