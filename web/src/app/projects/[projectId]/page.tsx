"use client";

import { use, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { runsApi, projectsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { MetricCard } from "@/components/ui/MetricCard";
import type { Run } from "@/lib/types";
import { RunCharts } from "@/components/charts/RunCharts";
import { TabBar } from "@/components/ui/TabBar";
import { BudgetSection } from "@/components/budget/BudgetSection";

function formatDuration(ms: number) {
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatCost(usd: number) {
  if (usd < 0.001) return `$${(usd * 1000).toFixed(2)}m`;
  return `$${usd.toFixed(4)}`;
}

function RunRow({ run, projectId }: { run: Run; projectId: string }) {
  return (
    <Link
      href={`/projects/${projectId}/runs/${run.RunID}`}
      className="flex items-center gap-4 px-5 py-4 border border-[var(--border)] bg-[var(--surface)] rounded-xl hover:border-indigo-600 transition-colors group"
    >
      <StatusBadge status={run.Status === "ok" ? "ok" : "error"} />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-mono text-[var(--text-muted)] truncate">{run.RunID}</p>
        <p className="text-xs text-[var(--text-muted)]">
          {new Date(run.StartTime).toLocaleString()}
        </p>
      </div>
      <div className="flex gap-6 text-sm tabular-nums">
        <span className="text-[var(--text-muted)]">{formatDuration(run.DurationMS)}</span>
        <span className="text-indigo-400">{formatCost(run.TotalCostUSD)}</span>
        <span className="text-[var(--text-muted)]">{run.TotalTokens.toLocaleString()} tok</span>
        <span className="text-[var(--text-muted)]">{run.SpanCount} spans</span>
      </div>
    </Link>
  );
}

export default function ProjectPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  const [activeTab, setActiveTab] = useState<"overview" | "budget">("overview");

  const { data: project } = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => projectsApi.get(projectId),
  });

  const { data: runsData, isLoading, error } = useQuery({
    queryKey: ["runs", projectId],
    queryFn: () => runsApi.list(projectId),
    refetchInterval: 10_000,
  });

  const runs = runsData?.runs ?? [];

  // Aggregate stats
  const totalCost = runs.reduce((s, r) => s + r.TotalCostUSD, 0);
  const totalTokens = runs.reduce((s, r) => s + r.TotalTokens, 0);
  const errorRate = runs.length
    ? ((runs.filter((r) => r.Status === "error").length / runs.length) * 100).toFixed(1)
    : "0";

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
        <div className="mb-2">
          <Link href="/" className="text-sm text-[var(--text-muted)] hover:text-indigo-400">
            ← Projects
          </Link>
        </div>
        <h1 className="text-2xl font-bold text-[var(--text)] mb-6">
          {project?.Name ?? projectId}
        </h1>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
          <MetricCard label="Total Runs" value={runs.length} />
          <MetricCard label="Total Cost" value={formatCost(totalCost)} accent />
          <MetricCard label="Total Tokens" value={totalTokens.toLocaleString()} />
          <MetricCard label="Error Rate" value={`${errorRate}%`} />
        </div>

        <TabBar
          tabs={[{ key: "overview", label: "Overview" }, { key: "budget", label: "Budget" }]}
          activeTab={activeTab}
          onTabChange={(k) => setActiveTab(k as "overview" | "budget")}
        />

        {activeTab === "overview" && (
          <>
            <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Trends</h2>
            <RunCharts runs={runs} />

            <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Recent Runs</h2>

            {isLoading && <div className="text-[var(--text-muted)]">Loading runs...</div>}
            {error && (
              <div className="text-red-400">Failed to load runs: {(error as Error).message}</div>
            )}

            <div className="flex flex-col gap-3">
              {runs.map((r) => (
                <RunRow key={r.RunID} run={r} projectId={projectId} />
              ))}
              {!isLoading && runs.length === 0 && (
                <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-10 text-center">
                  No runs yet. Send traces with <code className="text-indigo-400">make seed</code>.
                </div>
              )}
            </div>
          </>
        )}

        {activeTab === "budget" && <BudgetSection projectId={projectId} />}
      </main>
    </div>
  );
}
