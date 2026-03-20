"use client";

import { use } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { runsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { TopologyGraph } from "@/components/topology/TopologyGraph";

export default function TopologyPage({
  params,
}: {
  params: Promise<{ projectId: string; runId: string }>;
}) {
  const { projectId, runId } = use(params);

  const { data: topology, isLoading, error } = useQuery({
    queryKey: ["topology", runId],
    queryFn: () => runsApi.topology(runId),
  });

  return (
    <div className="flex flex-col h-screen">
      <Navbar />
      <div className="border-b border-[var(--border)] bg-[var(--surface)] px-6 py-2 flex items-center gap-4 text-sm">
        <Link href={`/projects/${projectId}/runs/${runId}`} className="text-[var(--text-muted)] hover:text-indigo-400">
          ← Run Detail
        </Link>
        <span className="text-[var(--text-muted)]">/</span>
        <span className="text-[var(--text)]">Topology</span>
        <span className="ml-4 font-mono text-xs text-[var(--text-muted)]">{runId}</span>
        {topology && (
          <span className="ml-auto text-xs text-[var(--text-muted)]">
            {topology.Nodes?.length ?? 0} nodes · {topology.Edges?.length ?? 0} edges
          </span>
        )}
      </div>

      <div className="flex-1 relative">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center text-[var(--text-muted)]">
            Loading topology...
          </div>
        )}
        {error && (
          <div className="absolute inset-0 flex items-center justify-center text-red-400">
            {(error as Error).message}
          </div>
        )}
        {topology && <TopologyGraph topology={topology} />}
      </div>
    </div>
  );
}
