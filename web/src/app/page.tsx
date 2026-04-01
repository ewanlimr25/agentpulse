"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { projectsApi } from "@/lib/api";
import { saveApiKey } from "@/lib/api-keys";
import { Navbar } from "@/components/Navbar";

interface CreatedKeys {
  apiKey: string;
  adminKey: string;
}

function CopyKeyField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div>
      <label className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      <div className="flex gap-2 items-stretch">
        <div className="flex-1 bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2.5 overflow-hidden">
          <p className="text-xs font-mono text-green-400 break-all">{value}</p>
        </div>
        <button
          onClick={handleCopy}
          className="shrink-0 border border-[var(--border)] text-[var(--text-muted)] text-xs px-3 rounded-lg hover:border-indigo-500 hover:text-[var(--text)] transition-colors"
        >
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>
    </div>
  );
}

function CreateProjectModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState("");
  const [createdKeys, setCreatedKeys] = useState<CreatedKeys | null>(null);
  const qc = useQueryClient();

  const { mutate, isPending, error } = useMutation({
    mutationFn: (n: string) => projectsApi.create(n),
    onSuccess: (data) => {
      saveApiKey(data.project.ID, data.api_key);
      if (data.admin_key) {
        localStorage.setItem(`adminKey_${data.project.ID}`, data.admin_key);
      }
      qc.invalidateQueries({ queryKey: ["projects"] });
      setCreatedKeys({ apiKey: data.api_key, adminKey: data.admin_key ?? "" });
    },
  });

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 px-4">
      <div className="w-full max-w-md bg-[var(--surface)] border border-[var(--border)] rounded-xl px-8 py-8">
        {!createdKeys ? (
          <>
            <h2 className="text-lg font-semibold text-[var(--text)] mb-6">Create Project</h2>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                const n = name.trim();
                if (n) mutate(n);
              }}
              className="flex flex-col gap-4"
            >
              <div>
                <label className="block text-xs text-[var(--text-muted)] mb-1">Project Name</label>
                <input
                  autoFocus
                  type="text"
                  placeholder="my-agent"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
                />
                {error && (
                  <p className="text-xs text-red-400 mt-1">{(error as Error).message}</p>
                )}
              </div>
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={onClose}
                  className="flex-1 border border-[var(--border)] text-[var(--text-muted)] text-sm py-2 rounded-lg hover:border-indigo-500 transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isPending || !name.trim()}
                  className="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium py-2 rounded-lg transition-colors"
                >
                  {isPending ? "Creating…" : "Create"}
                </button>
              </div>
            </form>
          </>
        ) : (
          <>
            <h2 className="text-lg font-semibold text-[var(--text)] mb-2">Project Created</h2>
            <p className="text-sm text-[var(--text-muted)] mb-4">
              Copy both keys now — they won&apos;t be shown again.
            </p>
            <div className="flex flex-col gap-3 mb-2">
              <CopyKeyField label="API Key (for SDK)" value={createdKeys.apiKey} />
              {createdKeys.adminKey && (
                <>
                  <CopyKeyField label="Admin Key (for settings)" value={createdKeys.adminKey} />
                  <p className="text-xs text-amber-400/80">
                    Store the Admin Key securely — it is used to modify project settings and will not be shown again.
                  </p>
                </>
              )}
            </div>
            <div className="flex gap-3 mt-4">
              <button
                onClick={onClose}
                className="flex-1 bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium py-2 rounded-lg transition-colors"
              >
                Done
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

export default function ProjectsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const { data: projects, isLoading, error } = useQuery({
    queryKey: ["projects"],
    queryFn: projectsApi.list,
  });

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1 max-w-4xl mx-auto w-full px-6 py-10">
        <div className="flex items-center justify-between mb-8">
          <h1 className="text-2xl font-bold text-[var(--text)]">Projects</h1>
          <button
            onClick={() => setShowCreate(true)}
            className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
          >
            + New Project
          </button>
        </div>

        {isLoading && (
          <div className="text-[var(--text-muted)]">Loading projects...</div>
        )}

        {error && (
          <div className="text-red-400 bg-red-950/40 border border-red-800 rounded-xl px-4 py-3">
            {(error as Error).message}
          </div>
        )}

        {projects && projects.length === 0 && (
          <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-10 text-center">
            No projects yet. Click <strong className="text-[var(--text)]">+ New Project</strong> to get started.
          </div>
        )}

        <div className="grid gap-4">
          {projects?.map((p) => (
            <Link
              key={p.ID}
              href={`/projects/${p.ID}`}
              className="block border border-[var(--border)] bg-[var(--surface)] rounded-xl px-6 py-4 hover:border-indigo-600 transition-colors group"
            >
              <div className="flex items-center justify-between">
                <span className="font-semibold text-[var(--text)] group-hover:text-indigo-300 transition-colors">
                  {p.Name}
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  {new Date(p.CreatedAt).toLocaleDateString()}
                </span>
              </div>
              <p className="text-xs text-[var(--text-muted)] mt-1 font-mono">{p.ID}</p>
            </Link>
          ))}
        </div>
      </main>

      {showCreate && <CreateProjectModal onClose={() => setShowCreate(false)} />}
    </div>
  );
}
