import type { RunEvalSummary } from "@/lib/types";

interface Props {
  summaries: RunEvalSummary[];
}

function scoreColor(score: number): string {
  if (score >= 0.7) return "text-green-400";
  if (score >= 0.4) return "text-amber-400";
  return "text-red-400";
}

function scoreBg(score: number): string {
  if (score >= 0.7) return "border-green-800/50 bg-green-950/20";
  if (score >= 0.4) return "border-amber-800/50 bg-amber-950/20";
  return "border-red-800/50 bg-red-950/20";
}

/**
 * A row of metric cards — one per active eval type — showing the recent average score.
 */
export function EvalHealthCards({ summaries }: Props) {
  if (summaries.length === 0) return null;

  // Group summaries by eval_name, compute overall avg per type.
  const byType = new Map<string, number[]>();
  for (const s of summaries) {
    const arr = byType.get(s.EvalName) ?? [];
    arr.push(s.AvgScore);
    byType.set(s.EvalName, arr);
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 mb-6">
      {Array.from(byType.entries()).map(([evalName, scores]) => {
        const avg = scores.reduce((a, b) => a + b, 0) / scores.length;
        return (
          <div
            key={evalName}
            className={`rounded-xl border px-4 py-3 ${scoreBg(avg)}`}
          >
            <p className="text-xs text-[var(--text-muted)] capitalize mb-1">{evalName.replace(/_/g, " ")}</p>
            <p className={`text-xl font-bold font-mono tabular-nums ${scoreColor(avg)}`}>
              {(avg * 100).toFixed(0)}%
            </p>
          </div>
        );
      })}
    </div>
  );
}
