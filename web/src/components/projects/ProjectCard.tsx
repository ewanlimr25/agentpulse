"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { runsApi, AuthError } from "@/lib/api";
import { getApiKey } from "@/lib/api-keys";
import { formatCost } from "@/components/runs/RunRow";
import type { Project } from "@/lib/types";

interface Props {
  project: Project;
}

function StatCell({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <p className="text-xs text-[var(--text-muted)] mb-0.5">{label}</p>
      <p className="text-sm font-semibold text-[var(--text)]">{value}</p>
    </div>
  );
}

function SkeletonStat() {
  return (
    <div>
      <div className="h-3 w-12 bg-[var(--surface-2)] rounded animate-pulse mb-1.5" />
      <div className="h-4 w-8 bg-[var(--surface-2)] rounded animate-pulse" />
    </div>
  );
}

export function ProjectCard({ project }: Props) {
  // null = not yet checked (SSR), true/false = checked client-side
  const [hasKey, setHasKey] = useState<boolean | null>(null);

  useEffect(() => {
    setHasKey(getApiKey(project.ID) !== null);
  }, [project.ID]);

  const { data, isLoading } = useQuery({
    queryKey: ["runs-summary", project.ID],
    queryFn: () => runsApi.list(project.ID, 20, 0),
    enabled: hasKey === true,
    retry: (_, err) => !(err instanceof AuthError),
  });

  const totalRuns = data?.total ?? 0;
  const runs = data?.runs ?? [];
  const totalCost = runs.reduce((s, r) => s + r.TotalCostUSD, 0);
  const errorCount = runs.filter((r) => r.Status === "error").length;
  const errorRate = runs.length
    ? `${((errorCount / runs.length) * 100).toFixed(1)}%`
    : "—";

  // isLoading is only true when hasKey is true and fetch is in flight
  const showSkeleton = hasKey === null || (hasKey === true && isLoading);
  const showAuthBadge = hasKey === false;
  const showStats = hasKey === true && !isLoading && data !== undefined;

  return (
    <Link
      href={`/projects/${project.ID}/overview`}
      className="block border border-[var(--border)] bg-[var(--surface)] rounded-xl px-6 py-4 hover:border-indigo-600 transition-colors group"
    >
      {/* Name + date */}
      <div className="flex items-center justify-between mb-1">
        <span className="font-semibold text-[var(--text)] group-hover:text-indigo-300 transition-colors">
          {project.Name}
        </span>
        <span className="text-xs text-[var(--text-muted)]">
          {new Date(project.CreatedAt).toLocaleDateString()}
        </span>
      </div>

      {/* Project ID */}
      <p className="text-xs text-[var(--text-muted)] font-mono mb-3">{project.ID}</p>

      {/* Stats row */}
      {showSkeleton && (
        <div className="flex gap-6">
          <SkeletonStat />
          <SkeletonStat />
          <SkeletonStat />
        </div>
      )}

      {showAuthBadge && (
        <span className="inline-flex items-center text-xs text-[var(--text-muted)] border border-[var(--border)] rounded-full px-2.5 py-0.5">
          Auth required to load metrics
        </span>
      )}

      {showStats && (
        <div className="flex gap-6">
          <StatCell label="Total Runs" value={totalRuns} />
          <StatCell label="Cost (sample)" value={formatCost(totalCost)} />
          <StatCell label="Error Rate" value={errorRate} />
        </div>
      )}
    </Link>
  );
}
