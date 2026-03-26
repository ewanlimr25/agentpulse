import Link from "next/link";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { LoopBadge } from "@/components/loops/LoopBadge";
import { SessionBadge } from "@/components/sessions/SessionBadge";
import type { Run } from "@/lib/types";

export function formatDuration(ms: number) {
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatCost(usd: number) {
  if (usd < 0.001) return `$${(usd * 1000).toFixed(2)}m`;
  return `$${usd.toFixed(4)}`;
}

interface RunRowProps {
  run: Run;
  projectId: string;
  selectable?: boolean;
  selected?: boolean;
  onToggle?: () => void;
}

export function RunRow({ run, projectId, selectable, selected, onToggle }: RunRowProps) {
  return (
    <Link
      href={`/projects/${projectId}/runs/${run.RunID}`}
      className={`flex items-center gap-4 px-5 py-4 border bg-[var(--surface)] rounded-xl hover:border-indigo-600 transition-colors group ${
        selected
          ? "border-indigo-500 ring-1 ring-indigo-500"
          : "border-[var(--border)]"
      }`}
    >
      {selectable && (
        <input
          type="checkbox"
          checked={selected ?? false}
          onChange={() => {/* handled by onToggle */}}
          onClick={(e) => {
            e.stopPropagation();
            e.preventDefault();
            onToggle?.();
          }}
          className="w-4 h-4 accent-indigo-500 cursor-pointer flex-shrink-0"
          aria-label={`Select run ${run.RunID}`}
        />
      )}
      <StatusBadge status={run.Status === "ok" ? "ok" : "error"} />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-mono text-[var(--text-muted)] truncate">{run.RunID}</p>
        <p className="text-xs text-[var(--text-muted)]">
          {new Date(run.StartTime).toLocaleString()}
        </p>
      </div>
      <div className="flex items-center gap-6 text-sm tabular-nums">
        {run.LoopDetected && <LoopBadge />}
        {run.SessionID && <SessionBadge projectId={projectId} sessionId={run.SessionID} />}
        <span className="text-[var(--text-muted)]">{formatDuration(run.DurationMS)}</span>
        <span className="text-indigo-400">{formatCost(run.TotalCostUSD)}</span>
        <span className="text-[var(--text-muted)]">{run.TotalTokens.toLocaleString()} tok</span>
        <span className="text-[var(--text-muted)]">{run.SpanCount} spans</span>
      </div>
    </Link>
  );
}
