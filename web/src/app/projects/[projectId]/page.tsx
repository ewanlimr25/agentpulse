"use client";

import { use, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { InfiniteData } from "@tanstack/react-query";
import Link from "next/link";
import { projectsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { MetricCard } from "@/components/ui/MetricCard";
import type { Run, RunsListResponse } from "@/lib/types";
import { RunCharts } from "@/components/charts/RunCharts";
import { TabBar } from "@/components/ui/TabBar";
import { BudgetSection } from "@/components/budget/BudgetSection";
import { RunList } from "@/components/runs/RunList";
import { formatCost } from "@/components/runs/RunRow";

function useAllFetchedRuns(projectId: string): Run[] {
  const qc = useQueryClient();
  const cached = qc.getQueryData<InfiniteData<RunsListResponse>>(["runs", projectId]);
  if (!cached) return [];
  const seen = new Set<string>();
  const runs: Run[] = [];
  for (const page of cached.pages) {
    for (const run of page.runs) {
      if (!seen.has(run.RunID)) {
        seen.add(run.RunID);
        runs.push(run);
      }
    }
  }
  return runs;
}

export default function ProjectPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  const searchParams = useSearchParams();
  const [activeTab, setActiveTab] = useState<"overview" | "budget">(
    searchParams.get("tab") === "budget" ? "budget" : "overview"
  );

  const { data: project } = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => projectsApi.get(projectId),
  });

  // Reads from the same cache key that RunList populates via useInfiniteQuery
  const runs = useAllFetchedRuns(projectId);

  // Aggregate stats across all fetched runs
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
            <RunList projectId={projectId} />
          </>
        )}

        {activeTab === "budget" && <BudgetSection projectId={projectId} />}
      </main>
    </div>
  );
}
