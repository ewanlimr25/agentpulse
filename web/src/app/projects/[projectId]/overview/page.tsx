"use client";

import { use } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { evalsApi, AuthError } from "@/lib/api";
import { MetricCard } from "@/components/ui/MetricCard";
import { RunCharts } from "@/components/charts/RunCharts";
import { useAllFetchedRuns } from "@/lib/hooks/useAllFetchedRuns";
import { formatCost } from "@/components/runs/RunRow";

export default function OverviewPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  const runs = useAllFetchedRuns(projectId);

  const { data: evalSummaries } = useQuery({
    queryKey: ["evalSummaries", projectId],
    queryFn: () => evalsApi.summaryByProject(projectId),
    retry: (_, err) => !(err instanceof AuthError),
  });

  const totalCost = runs.reduce((s, r) => s + r.TotalCostUSD, 0);
  const totalTokens = runs.reduce((s, r) => s + r.TotalTokens, 0);
  const errorRate = runs.length
    ? ((runs.filter((r) => r.Status === "error").length / runs.length) * 100).toFixed(1)
    : "0";

  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Overview</h1>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
        <MetricCard label="Total Runs" value={runs.length} />
        <MetricCard label="Total Cost" value={formatCost(totalCost)} accent />
        <MetricCard label="Total Tokens" value={totalTokens.toLocaleString()} />
        <MetricCard label="Error Rate" value={`${errorRate}%`} />
      </div>

      <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Trends</h2>
      <RunCharts runs={runs} evalSummaries={evalSummaries} />

      <div className="flex items-center justify-between mt-8 mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">Recent Runs</h2>
        <Link
          href={`/projects/${projectId}/runs`}
          className="text-sm text-indigo-400 hover:text-indigo-300 transition-colors"
        >
          View all →
        </Link>
      </div>
      {runs.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)]">No runs yet.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {runs.slice(0, 5).map((run) => (
            <li key={run.RunID}>
              <Link
                href={`/projects/${projectId}/runs/${run.RunID}`}
                className="flex items-center justify-between border border-[var(--border)] bg-[var(--surface)] rounded-lg px-4 py-3 text-sm hover:border-indigo-600 transition-colors"
              >
                <span className="font-mono text-xs text-[var(--text-muted)] truncate">{run.RunID}</span>
                <span className={`ml-4 text-xs font-medium ${run.Status === "error" ? "text-red-400" : "text-green-400"}`}>
                  {run.Status}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
