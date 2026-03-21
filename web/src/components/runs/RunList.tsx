"use client";

import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { runsApi } from "@/lib/api";
import type { Run } from "@/lib/types";
import { RunRow } from "./RunRow";
import { RunFilterBar, type StatusFilter, type SortBy } from "./RunFilterBar";

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
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [sortBy, setSortBy] = useState<SortBy>("newest");

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
        {!isLoading && total > 0 && (
          <p className="text-sm text-[var(--text-muted)]">
            Showing <span className="text-[var(--text)]">{filtered.length}</span>
            {statusFilter !== "all" && allRuns.length !== filtered.length && (
              <> (filtered from {allRuns.length})</>
            )}{" "}
            of <span className="text-[var(--text)]">{total}</span> runs
          </p>
        )}
      </div>

      {/* Run rows */}
      {isLoading && <div className="text-[var(--text-muted)]">Loading runs...</div>}
      {error && (
        <div className="text-red-400">Failed to load runs: {(error as Error).message}</div>
      )}

      <div className="flex flex-col gap-3">
        {filtered.map((r) => (
          <RunRow key={r.RunID} run={r} projectId={projectId} />
        ))}

        {!isLoading && allRuns.length > 0 && filtered.length === 0 && (
          <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-8 text-center">
            No runs match the current filter.
          </div>
        )}

        {!isLoading && total === 0 && (
          <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-10 text-center">
            No runs yet. Send traces with <code className="text-indigo-400">make seed</code>.
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
    </div>
  );
}
