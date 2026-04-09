"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import { evalsApi, AuthError } from "@/lib/api";
import { EvalsSection } from "@/components/evals/EvalsSection";
import { useAllFetchedRuns } from "@/lib/hooks/useAllFetchedRuns";

export default function EvalsPage({
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

  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Evals</h1>
      <EvalsSection
        projectId={projectId}
        runs={runs}
        evalSummaries={evalSummaries ?? []}
      />
    </div>
  );
}
