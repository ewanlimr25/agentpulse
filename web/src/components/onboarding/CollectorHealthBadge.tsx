"use client";

import { useProjectHealth } from "@/lib/hooks/useProjectHealth";

interface CollectorHealthBadgeProps {
  projectId: string;
  enabled?: boolean;
}

export function CollectorHealthBadge({ projectId, enabled = true }: CollectorHealthBadgeProps) {
  const { health, isLoading } = useProjectHealth(projectId, enabled);

  if (isLoading && !health) {
    return (
      <span className="inline-flex items-center gap-2 text-sm text-[var(--text-muted)]">
        <span className="w-2 h-2 rounded-full bg-[var(--text-muted)] opacity-50 animate-pulse" />
        Checking…
      </span>
    );
  }

  if (health?.CollectorReachable) {
    return (
      <span className="inline-flex items-center gap-2 text-sm text-green-400">
        <span className="w-2 h-2 rounded-full bg-green-400" />
        Receiving data
      </span>
    );
  }

  if (health && !health.CollectorReachable && health.LastSpanAt !== null) {
    return (
      <span className="inline-flex items-center gap-2 text-sm text-red-400">
        <span className="w-2 h-2 rounded-full bg-red-400" />
        No recent data
      </span>
    );
  }

  // health is undefined or LastSpanAt is null — never received anything
  return (
    <span className="inline-flex items-center gap-2 text-sm text-amber-400">
      <span className="w-2 h-2 rounded-full bg-amber-400 animate-pulse" />
      Waiting for first trace…
    </span>
  );
}
