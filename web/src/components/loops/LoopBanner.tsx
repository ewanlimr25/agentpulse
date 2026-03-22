"use client";

import type { RunLoop } from "@/lib/types";

interface Props {
  loops: RunLoop[];
}

function loopMessage(loop: RunLoop): string {
  if (loop.DetectionType === "topology_cycle") {
    return "Cycle detected in agent execution graph — the agent topology contains a back-edge.";
  }
  if (loop.Confidence === "high") {
    return `Agent called "${loop.SpanName}" ${loop.OccurrenceCount}× with identical input and output — no new information was retrieved.`;
  }
  return `Agent called "${loop.SpanName}" ${loop.OccurrenceCount}× with the same input — possible retry loop.`;
}

export function LoopBanner({ loops }: Props) {
  if (loops.length === 0) return null;

  // Show the highest-confidence loop first (high > low, topology_cycle = high).
  const top = loops[0];

  return (
    <div className="flex items-start gap-3 px-4 py-3 mb-6 rounded-xl border border-amber-700/60 bg-amber-950/30 text-amber-300 text-sm">
      <span className="mt-0.5 shrink-0 text-amber-400">⚠</span>
      <div className="flex-1 min-w-0">
        <p className="font-medium">{loopMessage(top)}</p>
        {loops.length > 1 && (
          <p className="mt-1 text-amber-400/70 text-xs">
            {loops.length - 1} more loop pattern{loops.length > 2 ? "s" : ""} detected in this run.
          </p>
        )}
      </div>
      <span className={`shrink-0 text-xs px-2 py-0.5 rounded-full border ${
        top.Confidence === "high"
          ? "bg-amber-900/50 border-amber-600 text-amber-300"
          : "bg-zinc-900/50 border-zinc-600 text-zinc-400"
      }`}>
        {top.Confidence === "high" ? "high confidence" : "low confidence"}
      </span>
    </div>
  );
}
