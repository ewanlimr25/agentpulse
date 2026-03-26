"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { runsApi } from "@/lib/api";
import { AuthError } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { ComparisonMetrics } from "@/components/compare/ComparisonMetrics";
import { ComparisonTopology } from "@/components/compare/ComparisonTopology";

interface PageProps {
  params: Promise<{ projectId: string; runIdA: string; runIdB: string }>;
}

export default function CompareRunsPage({ params }: PageProps) {
  const { projectId, runIdA, runIdB } = use(params);

  const {
    data,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["compare", projectId, runIdA, runIdB],
    queryFn: () => runsApi.compare(projectId, runIdA, runIdB),
    retry: false,
  });

  function renderError(err: unknown) {
    if (err instanceof AuthError) {
      if (err.status === 403) {
        return "One or both runs do not belong to this project.";
      }
      return "Authentication required. Please check your API key.";
    }
    if (err instanceof Error) {
      if (err.message.toLowerCase().includes("not found")) {
        return "One or both runs could not be found.";
      }
      return err.message;
    }
    return "An unexpected error occurred.";
  }

  return (
    <div className="min-h-screen bg-[var(--bg)]">
      <Navbar />
      <main className="max-w-7xl mx-auto px-4 py-8 space-y-8">
        {/* Breadcrumb + title */}
        <div>
          <nav className="text-xs text-[var(--text-muted)] mb-3 flex items-center gap-1.5">
            <Link href="/projects" className="hover:text-[var(--text)] transition-colors">
              Projects
            </Link>
            <span>/</span>
            <Link
              href={`/projects/${projectId}`}
              className="hover:text-[var(--text)] transition-colors"
            >
              {projectId}
            </Link>
            <span>/</span>
            <span>Compare Runs</span>
          </nav>
          <h1 className="text-2xl font-bold text-[var(--text)]">Run Comparison</h1>

          {/* Run ID links */}
          <div className="flex items-center gap-3 mt-2 flex-wrap">
            <span className="text-xs text-[var(--text-muted)] uppercase tracking-wide">Run A:</span>
            <Link
              href={`/projects/${projectId}/runs/${runIdA}`}
              className="text-sm font-mono text-blue-400 hover:text-blue-300 transition-colors"
            >
              {runIdA}
            </Link>
            <span className="text-[var(--text-muted)]">vs</span>
            <span className="text-xs text-[var(--text-muted)] uppercase tracking-wide">Run B:</span>
            <Link
              href={`/projects/${projectId}/runs/${runIdB}`}
              className="text-sm font-mono text-orange-400 hover:text-orange-300 transition-colors"
            >
              {runIdB}
            </Link>
          </div>
        </div>

        {/* Loading */}
        {isLoading && (
          <div className="space-y-4">
            <div className="h-64 rounded-xl border border-[var(--border)] bg-[var(--surface)] animate-pulse" />
            <div className="h-96 rounded-xl border border-[var(--border)] bg-[var(--surface)] animate-pulse" />
          </div>
        )}

        {/* Error */}
        {error && !isLoading && (
          <div className="rounded-xl border border-red-800 bg-red-950/30 px-6 py-5">
            <p className="text-red-400 font-semibold mb-1">Failed to load comparison</p>
            <p className="text-sm text-red-300/80">{renderError(error)}</p>
          </div>
        )}

        {/* Content */}
        {data && (
          <>
            {/* Metrics comparison table */}
            <section>
              <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
                Metrics
              </h2>
              <ComparisonMetrics
                runA={data.RunA}
                runB={data.RunB}
                evalsA={data.EvalsA ?? []}
                evalsB={data.EvalsB ?? []}
              />
            </section>

            {/* Topology comparison */}
            <section>
              <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
                Topology
              </h2>
              <ComparisonTopology
                topologyA={data.TopologyA}
                topologyB={data.TopologyB}
              />
            </section>
          </>
        )}
      </main>
    </div>
  );
}
