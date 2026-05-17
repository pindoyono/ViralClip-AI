"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { apiClient } from "@/lib/api";
import { useAuthStore } from "@/lib/store";

export default function RegisterPage() {
  const router = useRouter();
  const setAuth = useAuthStore((s) => s.setAuth);
  const [form, setForm] = useState({ name: "", email: "", password: "" });
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm((prev) => ({ ...prev, [e.target.name]: e.target.value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const { data } = await apiClient.post("/auth/register", form);
      setAuth(data.data.user, data.data.access_token);
      router.push("/dashboard");
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Registration failed";
      setError(message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <h1 className="text-2xl font-bold text-white mb-6">Create your account</h1>
      {error && (
        <div className="mb-4 p-3 bg-red-900/40 border border-red-700 rounded-lg text-red-300 text-sm">
          {error}
        </div>
      )}
      <form onSubmit={handleSubmit} className="space-y-4">
        {[
          { label: "Full Name", name: "name", type: "text", placeholder: "Jane Smith" },
          { label: "Email", name: "email", type: "email", placeholder: "you@example.com" },
          { label: "Password", name: "password", type: "password", placeholder: "Min. 8 characters" },
        ].map((f) => (
          <div key={f.name}>
            <label className="block text-sm font-medium text-slate-300 mb-1">{f.label}</label>
            <input
              type={f.type}
              name={f.name}
              value={form[f.name as keyof typeof form]}
              onChange={handleChange}
              required
              minLength={f.name === "password" ? 8 : undefined}
              className="w-full px-4 py-2.5 bg-slate-700 border border-slate-600 rounded-lg text-white placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-purple-500"
              placeholder={f.placeholder}
            />
          </div>
        ))}
        <button
          type="submit"
          disabled={loading}
          className="w-full bg-purple-600 hover:bg-purple-700 disabled:opacity-60 py-3 rounded-lg font-semibold text-white transition-colors"
        >
          {loading ? "Creating account…" : "Create account"}
        </button>
      </form>
      <p className="mt-4 text-center text-slate-400 text-sm">
        Already have an account?{" "}
        <Link href="/login" className="text-purple-400 hover:text-purple-300">
          Sign in
        </Link>
      </p>
    </>
  );
}
