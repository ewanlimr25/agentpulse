"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useInfiniteQuery } from "@tanstack/react-query";
import { runsApi } from "@/lib/api";
import type { Run } from "@/lib/types";
import { RunRow } from "./RunRow";
import { RunFilterBar, type StatusFilter, type SortBy } from "./RunFilterBar";
import { ExportButton } from "@/components/export/ExportButton";

const PAGE_SIZE = 20;

function applyFilterAndSort(runs: Run[], status: StatusFilter, sort: SortBy): Run[] {
  let result = status === "all" ? runs : runs.filter((r) => r.Status === status);

  result = [...result].sort((a, b) => {
    switch (sort) {
      case "oldest":
        return new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime();
      case "most-expensive":
        return b.TotalCostUSD - a.TotalCostUSD;
      case "longest":
        return b.DurationMS - a.DurationMS;
      default: // newest
        return new Date(b.StartTime).getTime() - new Date(a.StartTime).getTime();
    }
  });

  return result;
}

interface Props {
  projectId: string;
}

export function RunList({ projectId }: Props) {
  const router = useRouter();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [sortBy, setSortBy] = useState<SortBy>("newest");
  const [compareMode, setCompareMode] = useState(false);
  const [selectedRunIds, setSelectedRunIds] = useState<string[]>([]);

  const {
    data,
    isLoading,
    error,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["runs", projectId],
    queryFn: ({ pageParam }) => runsApi.list(projectId, PAGE_SIZE, pageParam as number),
    initialPageParam: 0,
    getNextPageParam: (lastPage) => {
      const next = lastPage.offset + lastPage.limit;
      return next < lastPage.total ? next : undefined;
    },
    refetchInterval: 10_000,
  });

  // Flatten pages, deduplicate by RunID
  const allRuns: Run[] = [];
  const seen = new Set<string>();
  for (const page of data?.pages ?? []) {
    for (const run of page.runs) {
      if (!seen.has(run.RunID)) {
        seen.add(run.RunID);
        allRuns.push(run);
      }
    }
  }

  const total = data?.pages[0]?.total ?? 0;
  const filtered = applyFilterAndSort(allRuns, statusFilter, sortBy);

  function toggleRunSelection(runId: string) {
    setSelectedRunIds((prev) => {
      if (prev.includes(runId)) {
        return prev.filter((id) => id !== runId);
      }
      if (prev.length >= 2) {
        // At capacity — swap out the oldest selection
        return [prev[1], runId];
      }
      return [...prev, runId];
    });
  }

  function handleCompareToggle() {
    setCompareMode((prev) => {
      if (prev) setSelectedRunIds([]);
      return !prev;
    });
  }

  function navigateToCompare() {
    if (selectedRunIds.length === 2) {
      router.push(
        `/projects/${projectId}/runs/compare/${selectedRunIds[0]}/${selectedRunIds[1]}`
      );
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Filter bar + count */}
      <div className="flex items-center justify-between flex-wrap gap-3">
        <RunFilterBar
          statusFilter={statusFilter}
          onStatusChange={setStatusFilter}
          sortBy={sortBy}
          onSortChange={setSortBy}
        />
        <div className="flex items-center gap-3">
          {!isLoading && total > 0 && (
            <p className="text-sm text-[var(--text-muted)]">
              Showing <span className="text-[var(--text)]">{filtered.length}</span>
              {statusFilter !== "all" && allRuns.length !== filtered.length && (
                <> (filtered from {allRuns.length})</>
              )}{" "}
              of <span className="text-[var(--text)]">{total}</span> runs
            </p>
          )}
          <ExportButton projectId={projectId} exportType="runs" />
          <button
            type="button"
            onClick={handleCompareToggle}
            className={`px-3 py-1.5 rounded-lg border text-xs font-medium transition-colors ${
              compareMode
                ? "border-indigo-500 bg-indigo-950/40 text-indigo-300 hover:bg-indigo-950/60"
                : "border-[var(--border)] text-[var(--text-muted)] hover:border-indigo-600 hover:text-[var(--text)]"
            }`}
          >
            {compareMode ? "Cancel Compare" : "Compare"}
          </button>
        </div>
      </div>

      {/* Compare mode hint */}
      {compareMode && (
        <p className="text-xs text-[var(--text-muted)]">
          {selectedRunIds.length === 0 && "Select two runs to compare."}
          {selectedRunIds.length === 1 && "Select one more run to compare."}
          {selectedRunIds.length === 2 && (
            <span className="text-indigo-300">2 runs selected — click Compare Runs to proceed.</span>
          )}
        </p>
      )}

      {/* Run rows */}
      {isLoading && <div className="text-[var(--text-muted)]">Loading runs...</div>}
      {error && (
        <div className="text-red-400">Failed to load runs: {(error as Error).message}</div>
      )}

      <div className="flex flex-col gap-3">
        {filtered.map((r) => (
          <RunRow
            key={r.RunID}
            run={r}
            projectId={projectId}
            selectable={compareMode}
            selected={selectedRunIds.includes(r.RunID)}
            onToggle={() => toggleRunSelection(r.RunID)}
          />
        ))}

        {!isLoading && allRuns.length > 0 && filtered.length === 0 && (
          <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-8 text-center">
            No runs match the current filter.
          </div>
        )}

        {!isLoading && total === 0 && (
          <div className="border border-[var(--border)] rounded-xl px-6 py-10 text-center flex flex-col items-center gap-3">
            <p className="text-sm text-[var(--text-muted)]">No runs yet.</p>
            <p className="text-xs text-[var(--text-muted)]">
              Visit the{" "}
              <a
                href={`/projects/${projectId}/overview`}
                className="text-indigo-400 hover:text-indigo-300 underline-offset-2 hover:underline transition-colors"
              >
                Overview page
              </a>{" "}
              to set up the SDK and send your first trace.
            </p>
          </div>
        )}
      </div>

      {/* Load more */}
      {hasNextPage && (
        <button
          type="button"
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="self-center px-5 py-2 rounded-lg border border-[var(--border)] text-sm text-[var(--text-muted)] hover:border-indigo-600 hover:text-[var(--text)] transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {isFetchingNextPage ? "Loading…" : "Load more"}
        </button>
      )}

      {/* Floating compare button */}
      {compareMode && selectedRunIds.length === 2 && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50">
          <button
            type="button"
            onClick={navigateToCompare}
            className="flex items-center gap-2 px-6 py-3 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-semibold text-sm shadow-2xl transition-colors"
          >
            Compare Runs →
          </button>
        </div>
      )}
    </div>
  );
}
