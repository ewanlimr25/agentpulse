import type { RunEvalSummary } from "@/lib/types";

interface Props {
  summaries: RunEvalSummary[];
}

function scoreColor(score: number): string {
  if (score >= 0.7) return "bg-green-950/40 border border-green-700 text-green-400";
  if (score >= 0.4) return "bg-amber-950/40 border border-amber-700 text-amber-400";
  return "bg-red-950/40 border border-red-700 text-red-400";
}

/**
 * Composite eval score badge for the run list.
 * Shows the worst (minimum) score across all active eval types so triage is easy.
 * On hover, a tooltip shows per-type breakdown.
 */
export function EvalScoreBadge({ summaries }: Props) {
  if (summaries.length === 0) return null;

  const worstScore = Math.min(...summaries.map((s) => s.AvgScore));
  const tooltipLines = summaries
    .map((s) => `${s.EvalName}: ${(s.AvgScore * 100).toFixed(0)}%`)
    .join("\n");

  return (
    <span
      title={tooltipLines}
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono tabular-nums shrink-0 cursor-default ${scoreColor(worstScore)}`}
    >
      <span>●</span>
      <span>{(worstScore * 100).toFixed(0)}%</span>
      {summaries.length > 1 && (
        <span className="opacity-60 text-[10px]">+{summaries.length - 1}</span>
      )}
    </span>
  );
}
