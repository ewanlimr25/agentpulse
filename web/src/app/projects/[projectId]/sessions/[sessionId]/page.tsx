"use client";

import { use } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { projectsApi, sessionsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { MetricCard } from "@/components/ui/MetricCard";
import { RunRow, formatCost } from "@/components/runs/RunRow";
import { SessionTimeline } from "@/components/sessions/SessionTimeline";

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const mins = Math.floor(ms / 60_000);
  const secs = Math.floor((ms % 60_000) / 1000);
  return `${mins}m ${secs}s`;
}

function sessionDuration(firstRunAt: string, lastRunAt: string): string {
  const diffMs = new Date(lastRunAt).getTime() - new Date(firstRunAt).getTime();
  return formatDuration(diffMs);
}

export default function SessionDetailPage({
  params,
}: {
  params: Promise<{ projectId: string; sessionId: string }>;
}) {
  const { projectId, sessionId } = use(params);
  const decodedSessionId = decodeURIComponent(sessionId);

  const { data: project } = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => projectsApi.get(projectId),
  });

  const { data: session, isLoading: sessionLoading } = useQuery({
    queryKey: ["session", projectId, decodedSessionId],
    queryFn: () => sessionsApi.get(projectId, decodedSessionId),
  });

  const { data: runs, isLoading: runsLoading } = useQuery({
    queryKey: ["sessionRuns", projectId, decodedSessionId],
    queryFn: () => sessionsApi.listRuns(projectId, decodedSessionId),
  });

  const isLoading = sessionLoading || runsLoading;

  const shortSessionId =
    decodedSessionId.length > 16
      ? decodedSessionId.slice(0, 16) + "…"
      : decodedSessionId;

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
        {/* Breadcrumb */}
        <nav className="flex items-center gap-2 text-sm text-[var(--text-muted)] mb-6">
          <Link href="/" className="hover:text-indigo-400 transition-colors">
            Projects
          </Link>
          <span>/</span>
          <Link
            href={`/projects/${projectId}`}
            className="hover:text-indigo-400 transition-colors"
          >
            {project?.Name ?? projectId}
          </Link>
          <span>/</span>
          <Link
            href={`/projects/${projectId}?tab=sessions`}
            className="hover:text-indigo-400 transition-colors"
          >
            Sessions
          </Link>
          <span>/</span>
          <span
            className="font-mono text-[var(--text)] truncate max-w-xs"
            title={decodedSessionId}
          >
            {shortSessionId}
          </span>
        </nav>

        <h1 className="text-2xl font-bold text-[var(--text)] mb-2 font-mono truncate" title={decodedSessionId}>
          Session: {shortSessionId}
        </h1>

        {isLoading ? (
          <div className="text-sm text-[var(--text-muted)] py-16 text-center">
            Loading session…
          </div>
        ) : (
          <>
            {/* Metric cards */}
            {session && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8 mt-6">
                <MetricCard label="Runs" value={session.RunCount} />
                <MetricCard
                  label="Total Cost"
                  value={formatCost(session.TotalCostUSD)}
                  accent
                />
                <MetricCard
                  label="Total Tokens"
                  value={session.TotalTokens.toLocaleString()}
                />
                <MetricCard
                  label="Duration"
                  value={sessionDuration(session.FirstRunAt, session.LastRunAt)}
                  sub={`${new Date(session.FirstRunAt).toLocaleDateString()} → ${new Date(session.LastRunAt).toLocaleDateString()}`}
                />
              </div>
            )}

            {/* Timeline chart */}
            {runs && runs.length > 0 && (
              <div className="mb-8">
                <SessionTimeline runs={runs} />
              </div>
            )}

            {/* Run list */}
            <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Turns</h2>
            {runs && runs.length > 0 ? (
              <div className="flex flex-col gap-3">
                {[...runs]
                  .sort(
                    (a, b) =>
                      new Date(a.StartTime).getTime() -
                      new Date(b.StartTime).getTime()
                  )
                  .map((run) => (
                    <RunRow key={run.RunID} run={run} projectId={projectId} />
                  ))}
              </div>
            ) : (
              <p className="text-sm text-[var(--text-muted)]">No runs found for this session.</p>
            )}
          </>
        )}
      </main>
    </div>
  );
}
