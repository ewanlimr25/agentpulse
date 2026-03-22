import Link from "next/link";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { LoopBadge } from "@/components/loops/LoopBadge";
import type { Run } from "@/lib/types";

export function formatDuration(ms: number) {
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatCost(usd: number) {
  if (usd < 0.001) return `$${(usd * 1000).toFixed(2)}m`;
  return `$${usd.toFixed(4)}`;
}

export function RunRow({ run, projectId }: { run: Run; projectId: string }) {
  return (
    <Link
      href={`/projects/${projectId}/runs/${run.RunID}`}
      className="flex items-center gap-4 px-5 py-4 border border-[var(--border)] bg-[var(--surface)] rounded-xl hover:border-indigo-600 transition-colors group"
    >
      <StatusBadge status={run.Status === "ok" ? "ok" : "error"} />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-mono text-[var(--text-muted)] truncate">{run.RunID}</p>
        <p className="text-xs text-[var(--text-muted)]">
          {new Date(run.StartTime).toLocaleString()}
        </p>
      </div>
      <div className="flex items-center gap-6 text-sm tabular-nums">
        {run.LoopDetected && <LoopBadge />}
        <span className="text-[var(--text-muted)]">{formatDuration(run.DurationMS)}</span>
        <span className="text-indigo-400">{formatCost(run.TotalCostUSD)}</span>
        <span className="text-[var(--text-muted)]">{run.TotalTokens.toLocaleString()} tok</span>
        <span className="text-[var(--text-muted)]">{run.SpanCount} spans</span>
      </div>
    </Link>
  );
}
