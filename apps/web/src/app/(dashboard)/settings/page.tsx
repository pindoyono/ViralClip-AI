"use client";

import { useState } from "react";
import { useProfile, useUpdateProfile } from "@/hooks/useProfile";
import { useSocialAccounts, useDisconnectSocialAccount } from "@/hooks/useSocialAccounts";
import { useAuth } from "@/hooks/useAuth";

const TIER_LABELS: Record<string, string> = {
  free: "Free",
  starter: "Starter",
  pro: "Pro",
  enterprise: "Enterprise",
};

const PLATFORM_ICONS: Record<string, string> = {
  tiktok: "TT",
  instagram: "IG",
  youtube: "YT",
  twitter: "TW",
};

export default function SettingsPage() {
  const { user } = useAuth();
  const { data: profile, isLoading: profileLoading } = useProfile();
  const updateProfile = useUpdateProfile();
  const { data: accounts, isLoading: accountsLoading } = useSocialAccounts();
  const disconnectAccount = useDisconnectSocialAccount();

  const [name, setName] = useState("");
  const [profileSaved, setProfileSaved] = useState(false);

  const displayUser = profile ?? user;

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    await updateProfile.mutateAsync({ name: name.trim() });
    setName("");
    setProfileSaved(true);
    setTimeout(() => setProfileSaved(false), 3000);
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Settings</h1>
        <p className="text-slate-400">Manage your account, billing, and social connections.</p>
      </div>

      {/* Profile section */}
      <section className="bg-slate-800 border border-slate-700 rounded-xl p-6 space-y-4">
        <h2 className="text-white font-semibold text-lg">Profile</h2>
        {profileLoading ? (
          <p className="text-slate-400 text-sm">Loading…</p>
        ) : (
          <div className="flex items-center gap-4 mb-4">
            <div className="w-14 h-14 rounded-full bg-purple-600 flex items-center justify-center text-white text-xl font-bold">
              {displayUser?.name?.[0]?.toUpperCase() ?? "U"}
            </div>
            <div>
              <p className="text-white font-medium">{displayUser?.name}</p>
              <p className="text-slate-400 text-sm">{displayUser?.email}</p>
            </div>
          </div>
        )}
        <form onSubmit={handleSaveProfile} className="flex gap-3 max-w-sm">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="New display name"
            className="flex-1 px-3 py-2 bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-400 text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
          />
          <button
            type="submit"
            disabled={updateProfile.isPending || !name.trim()}
            className="bg-purple-600 hover:bg-purple-700 disabled:opacity-50 px-4 py-2 rounded-lg text-white text-sm font-medium transition-colors"
          >
            {updateProfile.isPending ? "Saving…" : "Save"}
          </button>
        </form>
        {profileSaved && (
          <p className="text-green-400 text-sm">Profile updated successfully.</p>
        )}
        {updateProfile.isError && (
          <p className="text-red-400 text-sm">
            {(updateProfile.error as Error)?.message ?? "Update failed"}
          </p>
        )}
      </section>

      {/* Billing section */}
      <section className="bg-slate-800 border border-slate-700 rounded-xl p-6 space-y-3">
        <h2 className="text-white font-semibold text-lg">Billing</h2>
        <div className="flex items-center gap-3">
          <span className="text-slate-300 text-sm">Current plan:</span>
          <span className="bg-purple-600/30 text-purple-300 text-xs font-semibold px-2.5 py-1 rounded-full">
            {TIER_LABELS[displayUser?.subscription_tier ?? "free"] ?? "Free"}
          </span>
        </div>
        {(displayUser?.subscription_tier ?? "free") === "free" && (
          <p className="text-slate-400 text-sm">
            Upgrade to{" "}
            <span className="text-purple-400 font-medium">Starter</span> or{" "}
            <span className="text-purple-400 font-medium">Pro</span> for unlimited clips and scheduled posts.
          </p>
        )}
      </section>

      {/* Social Accounts section */}
      <section className="bg-slate-800 border border-slate-700 rounded-xl p-6 space-y-4">
        <h2 className="text-white font-semibold text-lg">Social Accounts</h2>
        {accountsLoading ? (
          <p className="text-slate-400 text-sm">Loading…</p>
        ) : !accounts?.length ? (
          <p className="text-slate-400 text-sm">
            No social accounts connected yet. Connect an account to start scheduling posts.
          </p>
        ) : (
          <ul className="space-y-3">
            {accounts.map((account) => (
              <li
                key={account.id}
                className="flex items-center justify-between gap-4 p-3 bg-slate-700/50 rounded-lg"
              >
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-full bg-slate-600 flex items-center justify-center text-white text-xs font-bold">
                    {PLATFORM_ICONS[account.platform] ?? account.platform.slice(0, 2).toUpperCase()}
                  </div>
                  <div>
                    <p className="text-white text-sm font-medium capitalize">{account.platform}</p>
                    <p className="text-slate-400 text-xs">@{account.username}</p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <span
                    className={`text-xs font-medium ${
                      account.is_active ? "text-green-400" : "text-slate-400"
                    }`}
                  >
                    {account.is_active ? "Connected" : "Inactive"}
                  </span>
                  <button
                    onClick={() => disconnectAccount.mutate(account.id)}
                    disabled={disconnectAccount.isPending}
                    className="text-xs text-slate-400 hover:text-red-400 disabled:opacity-40 transition-colors"
                  >
                    Disconnect
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* Notifications section */}
      <section className="bg-slate-800 border border-slate-700 rounded-xl p-6">
        <h2 className="text-white font-semibold text-lg mb-3">Notifications</h2>
        <p className="text-slate-400 text-sm">Email notification preferences coming soon.</p>
      </section>

      {/* API Keys section */}
      <section className="bg-slate-800 border border-slate-700 rounded-xl p-6">
        <h2 className="text-white font-semibold text-lg mb-3">API Keys</h2>
        <p className="text-slate-400 text-sm">API key management coming soon.</p>
      </section>
    </div>
  );
}

