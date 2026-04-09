"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { runsApi, AuthError } from "@/lib/api";
import type { Run } from "@/lib/types";

const PAGE_SIZE = 20;

/**
 * Reads all pages fetched so far from the shared infinite query cache.
 * This uses the same query key as RunList, so React Query deduplicates
 * the request — no extra network call when RunList is also mounted.
 */
export function useAllFetchedRuns(projectId: string): Run[] {
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
