"use client";

import { useCallback, useState } from "react";
import { useDropzone } from "react-dropzone";
import { useRouter } from "next/navigation";
import { apiClient } from "@/lib/api";

export default function UploadPage() {
  const router = useRouter();
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState("");

  const onDrop = useCallback(
    async (files: File[]) => {
      const file = files[0];
      if (!file) return;

      setUploading(true);
      setError("");
      setProgress(0);

      const formData = new FormData();
      formData.append("file", file);
      formData.append("title", file.name.replace(/\.[^/.]+$/, ""));

      try {
        const { data } = await apiClient.post("/videos/upload", formData, {
          headers: { "Content-Type": "multipart/form-data" },
          onUploadProgress: (evt) => {
            if (evt.total) {
              setProgress(Math.round((evt.loaded * 100) / evt.total));
            }
          },
        });
        router.push(`/videos/${data.data.id}`);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : "Upload failed";
        setError(msg);
      } finally {
        setUploading(false);
      }
    },
    [router]
  );

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: { "video/*": [".mp4", ".mov", ".avi", ".mkv", ".webm"] },
    maxSize: 5 * 1024 * 1024 * 1024, // 5GB
    multiple: false,
    disabled: uploading,
  });

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Upload Video</h1>
        <p className="text-slate-400">Upload a long-form video and our AI will generate viral clips.</p>
      </div>

      {error && (
        <div className="p-4 bg-red-900/40 border border-red-700 rounded-lg text-red-300">{error}</div>
      )}

      <div
        {...getRootProps()}
        className={`border-2 border-dashed rounded-2xl p-16 text-center cursor-pointer transition-colors ${
          isDragActive ? "border-purple-500 bg-purple-500/10" : "border-slate-600 hover:border-slate-400"
        } ${uploading ? "opacity-50 cursor-not-allowed" : ""}`}
      >
        <input {...getInputProps()} />
        {uploading ? (
          <div className="space-y-3">
            <p className="text-slate-300 font-medium">Uploading… {progress}%</p>
            <div className="w-full bg-slate-700 rounded-full h-2">
              <div
                className="bg-purple-500 h-2 rounded-full transition-all"
                style={{ width: `${progress}%` }}
              />
            </div>
          </div>
        ) : isDragActive ? (
          <p className="text-purple-400 text-lg">Drop your video here…</p>
        ) : (
          <div className="space-y-2">
            <p className="text-slate-300 text-lg">Drag & drop a video file here, or click to select</p>
            <p className="text-slate-500 text-sm">MP4, MOV, AVI, MKV, WebM — up to 5 GB</p>
          </div>
        )}
      </div>
    </div>
  );
}
