"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { runsApi, AuthError } from "@/lib/api";
import { ComparisonTopology } from "@/components/compare/ComparisonTopology";
import { ReplaySpanDiff } from "@/components/runs/ReplaySpanDiff";

interface PageProps {
  params: Promise<{ projectId: string; runId: string; replayRunId: string }>;
}

export default function ReplayDiffPage({ params }: PageProps) {
  const { projectId, runId, replayRunId } = use(params);

  const {
    data: comparison,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["compare", projectId, runId, replayRunId],
    queryFn: () => runsApi.compare(projectId, runId, replayRunId),
    retry: false,
  });

  const { data: spansA } = useQuery({
    queryKey: ["spans", runId],
    queryFn: () => runsApi.spans(runId, projectId),
  });

  const { data: spansB } = useQuery({
    queryKey: ["spans", replayRunId],
    queryFn: () => runsApi.spans(replayRunId, projectId),
  });

  function renderError(err: unknown): string {
    if (err instanceof AuthError) {
      return err.status === 403
        ? "One or both runs do not belong to this project."
        : "Authentication required.";
    }
    if (err instanceof Error) return err.message;
    return "An unexpected error occurred.";
  }

  return (
    <div className="flex-1">
      <main className="max-w-7xl mx-auto px-4 py-8 space-y-8">
        <div>
          <nav className="text-xs text-[var(--text-muted)] mb-3 flex items-center gap-1.5">
            <Link href="/" className="hover:text-[var(--text)] transition-colors">
              Projects
            </Link>
            <span>/</span>
            <Link
              href={`/projects/${projectId}`}
              className="hover:text-[var(--text)] transition-colors"
            >
              {projectId.slice(0, 8)}…
            </Link>
            <span>/</span>
            <Link
              href={`/projects/${projectId}/runs/${runId}`}
              className="hover:text-[var(--text)] transition-colors font-mono"
            >
              {runId.slice(0, 8)}…
            </Link>
            <span>/</span>
            <span>Replay diff</span>
          </nav>
          <h1 className="text-2xl font-bold text-[var(--text)]">Replay Diff</h1>
          <div className="flex items-center gap-3 mt-2 flex-wrap text-sm">
            <span className="text-xs text-[var(--text-muted)] uppercase tracking-wide">Original:</span>
            <Link
              href={`/projects/${projectId}/runs/${runId}`}
              className="font-mono text-blue-400 hover:text-blue-300"
            >
              {runId}
            </Link>
            <span className="text-[var(--text-muted)]">vs</span>
            <span className="text-xs text-[var(--text-muted)] uppercase tracking-wide">Replay:</span>
            <Link
              href={`/projects/${projectId}/runs/${replayRunId}`}
              className="font-mono text-orange-400 hover:text-orange-300"
            >
              {replayRunId}
            </Link>
          </div>
        </div>

        {isLoading && (
          <div className="space-y-4">
            <div className="h-96 rounded-xl border border-[var(--border)] bg-[var(--surface)] animate-pulse" />
          </div>
        )}

        {error && !isLoading && (
          <div className="rounded-xl border border-red-800 bg-red-950/30 px-6 py-5">
            <p className="text-red-400 font-semibold mb-1">Failed to load replay diff</p>
            <p className="text-sm text-red-300/80">{renderError(error)}</p>
          </div>
        )}

        {comparison && (
          <section>
            <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
              Topology
            </h2>
            <ComparisonTopology
              topologyA={comparison.TopologyA}
              topologyB={comparison.TopologyB}
            />
          </section>
        )}

        <section>
          <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
            Span diff
          </h2>
          <ReplaySpanDiff spansA={spansA ?? []} spansB={spansB ?? []} />
        </section>
      </main>
    </div>
  );
}
