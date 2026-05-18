"use client";

import { useSearchParams } from "next/navigation";
import { useState } from "react";
import { useScheduledPosts, useCreateScheduledPost, useCancelScheduledPost } from "@/hooks/useSchedule";
import { useSocialAccounts } from "@/hooks/useSocialAccounts";
import { useClip } from "@/hooks/useClips";
import { format } from "date-fns";

const STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  published: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
  cancelled: "text-slate-400 bg-slate-400/10",
};

function ScheduleForm({ preselectedClipId }: { preselectedClipId?: string }) {
  const [clipId, setClipId] = useState(preselectedClipId ?? "");
  const [accountId, setAccountId] = useState("");
  const [scheduledAt, setScheduledAt] = useState("");
  const [caption, setCaption] = useState("");

  const { data: accounts } = useSocialAccounts();
  const { data: preselectedClip } = useClip(preselectedClipId ?? "");
  const createPost = useCreateScheduledPost();
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await createPost.mutateAsync({
      clip_id: clipId,
      social_account_id: accountId,
      scheduled_at: new Date(scheduledAt).toISOString(),
      caption: caption || undefined,
    });
    setClipId(preselectedClipId ?? "");
    setAccountId("");
    setScheduledAt("");
    setCaption("");
    setSuccess(true);
    setTimeout(() => setSuccess(false), 3000);
  };

  const activeAccounts = accounts?.filter((a) => a.is_active) ?? [];

  return (
    <form onSubmit={handleSubmit} className="bg-slate-800 border border-slate-700 rounded-xl p-6 space-y-4">
      <h2 className="text-white font-semibold text-lg">Schedule a Post</h2>

      {preselectedClip && (
        <p className="text-sm text-purple-300">
          Scheduling clip: <span className="font-medium">{preselectedClip.title}</span>
        </p>
      )}

      {!preselectedClipId && (
        <div>
          <label className="block text-xs text-slate-400 mb-1">Clip ID</label>
          <input
            type="text"
            value={clipId}
            onChange={(e) => setClipId(e.target.value)}
            required
            placeholder="Paste a clip ID"
            className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-400 text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
          />
        </div>
      )}

      <div>
        <label className="block text-xs text-slate-400 mb-1">Social Account</label>
        {activeAccounts.length === 0 ? (
          <p className="text-xs text-slate-500">No connected accounts. Go to Settings to connect one.</p>
        ) : (
          <select
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            required
            className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
          >
            <option value="">Select account…</option>
            {activeAccounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.platform} · @{a.username}
              </option>
            ))}
          </select>
        )}
      </div>

      <div>
        <label className="block text-xs text-slate-400 mb-1">Schedule at</label>
        <input
          type="datetime-local"
          value={scheduledAt}
          onChange={(e) => setScheduledAt(e.target.value)}
          required
          min={new Date().toISOString().slice(0, 16)}
          className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
        />
      </div>

      <div>
        <label className="block text-xs text-slate-400 mb-1">Caption (optional)</label>
        <textarea
          value={caption}
          onChange={(e) => setCaption(e.target.value)}
          rows={2}
          placeholder="Add a caption for the post…"
          className="w-full px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-400 text-sm focus:outline-none focus:ring-2 focus:ring-purple-500 resize-none"
        />
      </div>

      <button
        type="submit"
        disabled={createPost.isPending || !clipId || !accountId || !scheduledAt || activeAccounts.length === 0}
        className="bg-purple-600 hover:bg-purple-700 disabled:opacity-50 px-5 py-2 rounded-lg text-white text-sm font-medium transition-colors"
      >
        {createPost.isPending ? "Scheduling…" : "Schedule Post"}
      </button>

      {success && <p className="text-green-400 text-sm">Post scheduled successfully!</p>}
      {createPost.isError && (
        <p className="text-red-400 text-sm">
          {(createPost.error as Error)?.message ?? "Failed to schedule post"}
        </p>
      )}
    </form>
  );
}

export default function SchedulePage() {
  const searchParams = useSearchParams();
  const preselectedClipId = searchParams.get("clip_id") ?? undefined;

  const { data, isLoading } = useScheduledPosts();
  const cancelPost = useCancelScheduledPost();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Schedule</h1>
        <p className="text-slate-400">
          Schedule your clips to post at optimal times.
        </p>
      </div>

      <ScheduleForm preselectedClipId={preselectedClipId} />

      <div className="bg-slate-800 border border-slate-700 rounded-xl">
        <div className="px-6 py-4 border-b border-slate-700">
          <h2 className="text-white font-semibold">Upcoming Posts</h2>
        </div>
        {isLoading ? (
          <div className="p-6 text-center text-slate-400">Loading…</div>
        ) : !data?.length ? (
          <div className="p-6 text-center text-slate-400">
            <p className="text-slate-300 font-medium">No scheduled posts</p>
            <p className="mt-1 text-sm">Use the form above to schedule your first post.</p>
          </div>
        ) : (
          <ul className="divide-y divide-slate-700">
            {data.map((post) => (
              <li
                key={post.id}
                className="flex items-center gap-4 px-6 py-4"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-white font-medium capitalize">
                    {post.platform}
                  </p>
                  {post.caption && (
                    <p className="text-sm text-slate-400 truncate mt-0.5">
                      {post.caption}
                    </p>
                  )}
                  <p className="text-xs text-slate-500 mt-0.5">
                    {format(new Date(post.scheduled_at), "MMM d, yyyy · h:mm a")}
                  </p>
                </div>

                <span
                  className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize shrink-0 ${
                    STATUS_COLORS[post.status] ?? "text-slate-400"
                  }`}
                >
                  {post.status}
                </span>

                {post.status === "pending" && (
                  <button
                    onClick={() => cancelPost.mutate(post.id)}
                    disabled={cancelPost.isPending}
                    className="text-xs text-slate-400 hover:text-red-400 disabled:opacity-40 transition-colors shrink-0"
                  >
                    Cancel
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}


const STATUS_COLORS: Record<string, string> = {
  pending: "text-yellow-400 bg-yellow-400/10",
  published: "text-green-400 bg-green-400/10",
  failed: "text-red-400 bg-red-400/10",
  cancelled: "text-slate-400 bg-slate-400/10",
};

export default function SchedulePage() {
  const { data, isLoading } = useScheduledPosts();
  const cancelPost = useCancelScheduledPost();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Schedule</h1>
        <p className="text-slate-400">
          Schedule your clips to post at optimal times.
        </p>
      </div>

      <div className="bg-slate-800 border border-slate-700 rounded-xl">
        {isLoading ? (
          <div className="p-6 text-center text-slate-400">Loading…</div>
        ) : !data?.length ? (
          <div className="p-6 text-center text-slate-400">
            <p className="text-lg font-medium text-slate-300">
              No scheduled posts
            </p>
            <p className="mt-1 text-sm">
              Connect a social account and schedule your first post.
            </p>
          </div>
        ) : (
          <ul className="divide-y divide-slate-700">
            {data.map((post) => (
              <li
                key={post.id}
                className="flex items-center gap-4 px-6 py-4"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-white font-medium capitalize">
                    {post.platform}
                  </p>
                  {post.caption && (
                    <p className="text-sm text-slate-400 truncate mt-0.5">
                      {post.caption}
                    </p>
                  )}
                  <p className="text-xs text-slate-500 mt-0.5">
                    {format(new Date(post.scheduled_at), "MMM d, yyyy · h:mm a")}
                  </p>
                </div>

                <span
                  className={`text-xs font-medium px-2 py-0.5 rounded-full capitalize shrink-0 ${
                    STATUS_COLORS[post.status] ?? "text-slate-400"
                  }`}
                >
                  {post.status}
                </span>

                {post.status === "pending" && (
                  <button
                    onClick={() => cancelPost.mutate(post.id)}
                    disabled={cancelPost.isPending}
                    className="text-xs text-slate-400 hover:text-red-400 disabled:opacity-40 transition-colors shrink-0"
                  >
                    Cancel
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
