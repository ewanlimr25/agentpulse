"use client";

import { use, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useQuery, useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { projectsApi, evalsApi, AuthError } from "@/lib/api";
import { saveApiKey, removeApiKey } from "@/lib/api-keys";
import { Navbar } from "@/components/Navbar";
import { ApiKeyPrompt } from "@/components/ApiKeyPrompt";
import { MetricCard } from "@/components/ui/MetricCard";
import { runsApi } from "@/lib/api";
import type { Run } from "@/lib/types";
import { RunCharts } from "@/components/charts/RunCharts";
import { TabBar } from "@/components/ui/TabBar";
import { BudgetSection } from "@/components/budget/BudgetSection";
import { AlertsSection } from "@/components/alerts/AlertsSection";
import { ServicesSection } from "@/components/analytics/ServicesSection";
import { EvalsSection } from "@/components/evals/EvalsSection";
import { SessionsSection } from "@/components/sessions/SessionsSection";
import { RunList } from "@/components/runs/RunList";
import { formatCost } from "@/components/runs/RunRow";

const PAGE_SIZE = 20;

function useAllFetchedRuns(projectId: string): Run[] {
  // Same query key as RunList — React Query deduplicates requests and shares
  // the cache, so this subscribes reactively without a second network call.
  const { data } = useInfiniteQuery({
    queryKey: ["runs", projectId],
    queryFn: ({ pageParam }) => runsApi.list(projectId, PAGE_SIZE, pageParam as number),
    initialPageParam: 0,
    getNextPageParam: (lastPage) => {
      const next = lastPage.offset + lastPage.limit;
      return next < lastPage.total ? next : undefined;
    },
    retry: (_, err) => !(err instanceof AuthError),
  });

  if (!data) return [];
  const seen = new Set<string>();
  const runs: Run[] = [];
  for (const page of data.pages) {
    for (const run of page.runs ?? []) {
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
  const [activeTab, setActiveTab] = useState<"overview" | "budget" | "alerts" | "services" | "evals" | "sessions">(
    searchParams.get("tab") === "budget" ? "budget" :
    searchParams.get("tab") === "alerts" ? "alerts" :
    searchParams.get("tab") === "services" ? "services" :
    searchParams.get("tab") === "evals" ? "evals" :
    searchParams.get("tab") === "sessions" ? "sessions" : "overview"
  );

  const queryClient = useQueryClient();
  const [keyError, setKeyError] = useState("");
  const [isValidatingKey, setIsValidatingKey] = useState(false);

  const { data: project, error: projectError } = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => projectsApi.get(projectId),
    retry: (_, err) => !(err instanceof AuthError),
  });

  const { data: evalSummaries } = useQuery({
    queryKey: ["evalSummaries", projectId],
    queryFn: () => evalsApi.summaryByProject(projectId),
    retry: (_, err) => !(err instanceof AuthError),
  });

  // Reads from the same cache key that RunList populates via useInfiniteQuery
  const runs = useAllFetchedRuns(projectId);

  // Aggregate stats across all fetched runs
  const totalCost = runs.reduce((s, r) => s + r.TotalCostUSD, 0);
  const totalTokens = runs.reduce((s, r) => s + r.TotalTokens, 0);
  const errorRate = runs.length
    ? ((runs.filter((r) => r.Status === "error").length / runs.length) * 100).toFixed(1)
    : "0";

  // Keep showing the prompt while we're validating — prevents a flash of the
  // main page when fetchQuery briefly transitions the project query through
  // "pending" state (error → pending → error/success).
  if (projectError instanceof AuthError || isValidatingKey) {
    return (
      <div className="flex flex-col min-h-screen">
        <Navbar />
        <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
          <div className="mb-2">
            <Link href="/" className="text-sm text-[var(--text-muted)] hover:text-indigo-400">
              ← Projects
            </Link>
          </div>
          <ApiKeyPrompt
            projectId={projectId}
            keyError={keyError}
            onKeySubmit={async (key) => {
              setKeyError("");
              setIsValidatingKey(true);
              saveApiKey(projectId, key);
              try {
                await queryClient.fetchQuery({
                  queryKey: ["project", projectId],
                  queryFn: () => projectsApi.get(projectId),
                  retry: false,
                });
                // Success — clear cached 401 errors from the other queries.
                queryClient.removeQueries({ queryKey: ["runs", projectId] });
                queryClient.removeQueries({ queryKey: ["evalSummaries", projectId] });
              } catch {
                removeApiKey(projectId);
                setKeyError("Invalid API key — please check and try again.");
              } finally {
                setIsValidatingKey(false);
              }
            }}
          />
        </main>
      </div>
    );
  }

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
          tabs={[
            { key: "overview", label: "Overview" },
            { key: "services", label: "Services" },
            { key: "budget", label: "Budget" },
            { key: "alerts", label: "Alerts" },
            { key: "evals", label: "Evals" },
            { key: "sessions", label: "Sessions" },
          ]}
          activeTab={activeTab}
          onTabChange={(k) => setActiveTab(k as "overview" | "budget" | "alerts" | "services" | "evals" | "sessions")}
        />

        {activeTab === "overview" && (
          <>
            <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Trends</h2>
            <RunCharts runs={runs} evalSummaries={evalSummaries} />

            <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Recent Runs</h2>
            <RunList projectId={projectId} />
          </>
        )}

        {activeTab === "services" && <ServicesSection projectId={projectId} />}

        {activeTab === "budget" && <BudgetSection projectId={projectId} />}

        {activeTab === "alerts" && <AlertsSection projectId={projectId} />}

        {activeTab === "evals" && (
          <EvalsSection projectId={projectId} runs={runs} evalSummaries={evalSummaries ?? []} />
        )}

        {activeTab === "sessions" && <SessionsSection projectId={projectId} />}
      </main>
    </div>
  );
}
