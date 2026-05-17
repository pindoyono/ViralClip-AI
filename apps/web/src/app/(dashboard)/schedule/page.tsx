"use client";

import { useScheduledPosts, useCancelScheduledPost } from "@/hooks/useSchedule";
import { format } from "date-fns";

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
        ) : !data?.posts?.length ? (
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
            {data.posts.map((post) => (
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
