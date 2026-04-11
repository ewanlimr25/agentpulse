"use client";

import { use, useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { playgroundApi } from "@/lib/api";
import type { PlaygroundSession } from "@/lib/types";

export default function PlaygroundListPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  const router = useRouter();

  const [sessions, setSessions] = useState<PlaygroundSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadSessions = useCallback(async () => {
    try {
      setLoading(true);
      const res = await playgroundApi.listSessions(projectId);
      setSessions(res.sessions ?? []);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to load sessions";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  async function handleNewSession() {
    try {
      setCreating(true);
      const session = await playgroundApi.createSession(projectId, {
        name: "Untitled session",
        variants: [
          {
            label: "Variant A",
            model_id: "claude-sonnet-4-6",
            system: "",
            messages: [{ role: "user", content: "" }],
            temperature: 1,
          },
          {
            label: "Variant B",
            model_id: "claude-sonnet-4-6",
            system: "",
            messages: [{ role: "user", content: "" }],
            temperature: 1,
          },
        ],
      });
      router.push(`/projects/${projectId}/playground/${session.ID}`);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to create session";
      setError(message);
      setCreating(false);
    }
  }

  function formatDate(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  return (
    <div className="px-6 py-8">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-[var(--text)]">
          Prompt Playground
        </h1>
        <button
          type="button"
          onClick={handleNewSession}
          disabled={creating}
          className="rounded-lg bg-blue-600 hover:bg-blue-500 disabled:bg-blue-600/50 disabled:cursor-not-allowed text-white text-sm font-medium px-4 py-2 transition-colors"
        >
          {creating ? "Creating..." : "New Session"}
        </button>
      </div>

      {error && (
        <div className="rounded-lg bg-red-950/40 border border-red-700 p-3 mb-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="animate-pulse h-16 rounded-lg bg-[var(--surface-2)]"
            />
          ))}
        </div>
      ) : sessions.length === 0 ? (
        <div className="rounded-lg bg-[var(--surface-2)] border border-[var(--border)] p-8 text-center">
          <p className="text-[var(--text-muted)] text-sm">
            No playground sessions yet. Click &quot;New Session&quot; to get started.
          </p>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {sessions.map((session) => (
            <li key={session.ID}>
              <button
                type="button"
                onClick={() =>
                  router.push(
                    `/projects/${projectId}/playground/${session.ID}`
                  )
                }
                className="w-full flex items-center justify-between border border-[var(--border)] bg-[var(--surface)] rounded-lg px-4 py-3 text-left hover:border-indigo-600 transition-colors"
              >
                <div className="flex flex-col gap-0.5">
                  <span className="text-sm font-medium text-[var(--text)]">
                    {session.Name}
                  </span>
                  <span className="text-xs text-[var(--text-muted)]">
                    {formatDate(session.CreatedAt)}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  {session.SourceRunID && (
                    <span className="inline-flex items-center rounded-full bg-indigo-500/20 text-indigo-300 text-xs px-2 py-0.5">
                      Seeded from run
                    </span>
                  )}
                  <span className="text-xs text-[var(--text-muted)]">
                    {(session.Variants ?? []).length} variant
                    {(session.Variants ?? []).length !== 1 ? "s" : ""}
                  </span>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
