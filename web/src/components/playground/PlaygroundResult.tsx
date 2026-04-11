"use client";

import type { PlaygroundExecution } from "@/lib/types";

interface PlaygroundResultProps {
  execution: PlaygroundExecution | null;
  isRunning: boolean;
}

function formatCost(usd: number): string {
  return `$${usd.toFixed(5)}`;
}

export function PlaygroundResult({ execution, isRunning }: PlaygroundResultProps) {
  if (isRunning) {
    return (
      <div className="rounded-lg bg-[var(--surface-2)] p-4">
        <div className="animate-pulse space-y-3">
          <div className="h-4 w-3/4 rounded bg-[var(--border)]" />
          <div className="h-4 w-1/2 rounded bg-[var(--border)]" />
          <div className="h-4 w-2/3 rounded bg-[var(--border)]" />
        </div>
      </div>
    );
  }

  if (!execution) {
    return (
      <div className="rounded-lg bg-[var(--surface-2)] p-4">
        <p className="text-sm text-[var(--text-muted)]">Run to see results</p>
      </div>
    );
  }

  if (execution.Error) {
    return (
      <div className="space-y-3">
        <div className="rounded-lg bg-red-950/40 border border-red-700 p-4">
          <p className="text-sm font-medium text-red-400 mb-1">Error</p>
          <pre className="text-xs text-red-300 whitespace-pre-wrap break-words font-mono">
            {execution.Error}
          </pre>
        </div>
        <MetricsRow execution={execution} />
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="rounded-lg bg-[var(--surface-2)] p-4">
        <pre className="text-sm text-[var(--text)] whitespace-pre-wrap break-words font-mono">
          {execution.Output ?? ""}
        </pre>
      </div>
      <MetricsRow execution={execution} />
    </div>
  );
}

function MetricsRow({ execution }: { execution: PlaygroundExecution }) {
  return (
    <div className="grid grid-cols-4 gap-2">
      <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
        <p className="text-[var(--text-muted)] text-xs mb-0.5">Input tokens</p>
        <p className="font-mono text-sm text-[var(--text)]">{execution.InputTokens.toLocaleString()}</p>
      </div>
      <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
        <p className="text-[var(--text-muted)] text-xs mb-0.5">Output tokens</p>
        <p className="font-mono text-sm text-[var(--text)]">{execution.OutputTokens.toLocaleString()}</p>
      </div>
      <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
        <p className="text-[var(--text-muted)] text-xs mb-0.5">Cost</p>
        <p className="font-mono text-sm text-[var(--text)]">{formatCost(execution.CostUSD)}</p>
      </div>
      <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
        <p className="text-[var(--text-muted)] text-xs mb-0.5">Latency</p>
        <p className="font-mono text-sm text-[var(--text)]">{execution.LatencyMS.toLocaleString()} ms</p>
      </div>
    </div>
  );
}
