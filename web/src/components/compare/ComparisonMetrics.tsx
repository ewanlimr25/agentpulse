"use client";

import type { Run, SpanEval } from "@/lib/types";
import { DeltaIndicator } from "./DeltaIndicator";
import { formatDuration, formatCost } from "@/components/runs/RunRow";

interface Props {
  runA: Run;
  runB: Run;
  evalsA: SpanEval[];
  evalsB: SpanEval[];
}

function avgScore(evals: SpanEval[], evalName: string): { avg: number; count: number } | null {
  const filtered = evals.filter((e) => e.EvalName === evalName);
  if (filtered.length === 0) return null;
  const avg = filtered.reduce((sum, e) => sum + e.Score, 0) / filtered.length;
  return { avg, count: filtered.length };
}

function getAllEvalNames(evalsA: SpanEval[], evalsB: SpanEval[]): string[] {
  const names = new Set<string>();
  for (const e of evalsA) names.add(e.EvalName);
  for (const e of evalsB) names.add(e.EvalName);
  return Array.from(names).sort();
}

interface MetricRowProps {
  label: string;
  valueA: string;
  valueB: string;
  numA: number;
  numB: number;
  format?: "currency" | "number" | "percentage" | "ms";
  lowerIsBetter?: boolean;
}

function MetricRow({ label, valueA, valueB, numA, numB, format = "number", lowerIsBetter }: MetricRowProps) {
  return (
    <tr className="border-b border-[var(--border)] last:border-0">
      <td className="py-3 px-4 text-sm font-mono text-blue-300 tabular-nums text-right">{valueA}</td>
      <td className="py-3 px-4 text-xs text-[var(--text-muted)] text-center whitespace-nowrap uppercase tracking-wide">
        {label}
      </td>
      <td className="py-3 px-4 text-sm font-mono text-orange-300 tabular-nums">{valueB}</td>
      <td className="py-3 px-4 text-right">
        <DeltaIndicator valueA={numA} valueB={numB} format={format} lowerIsBetter={lowerIsBetter} />
      </td>
    </tr>
  );
}

export function ComparisonMetrics({ runA, runB, evalsA, evalsB }: Props) {
  const evalNames = getAllEvalNames(evalsA, evalsB);

  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] overflow-hidden">
      {/* Header */}
      <div className="grid grid-cols-[1fr_auto_1fr_auto] border-b border-[var(--border)]">
        <div className="px-4 py-3 text-xs font-semibold text-blue-400 uppercase tracking-wide text-right">
          Run A
        </div>
        <div className="px-4 py-3 text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide text-center">
          Metric
        </div>
        <div className="px-4 py-3 text-xs font-semibold text-orange-400 uppercase tracking-wide">
          Run B
        </div>
        <div className="px-4 py-3 text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide text-right">
          Delta (B vs A)
        </div>
      </div>

      <table className="w-full">
        <tbody>
          <MetricRow
            label="Total Cost"
            valueA={formatCost(runA.TotalCostUSD)}
            valueB={formatCost(runB.TotalCostUSD)}
            numA={runA.TotalCostUSD}
            numB={runB.TotalCostUSD}
            format="currency"
            lowerIsBetter
          />
          <MetricRow
            label="Input Tokens"
            valueA={runA.TotalInputTokens.toLocaleString()}
            valueB={runB.TotalInputTokens.toLocaleString()}
            numA={runA.TotalInputTokens}
            numB={runB.TotalInputTokens}
            lowerIsBetter
          />
          <MetricRow
            label="Output Tokens"
            valueA={runA.TotalOutputTokens.toLocaleString()}
            valueB={runB.TotalOutputTokens.toLocaleString()}
            numA={runA.TotalOutputTokens}
            numB={runB.TotalOutputTokens}
            lowerIsBetter
          />
          <MetricRow
            label="Duration"
            valueA={formatDuration(runA.DurationMS)}
            valueB={formatDuration(runB.DurationMS)}
            numA={runA.DurationMS}
            numB={runB.DurationMS}
            format="ms"
            lowerIsBetter
          />
          <MetricRow
            label="Span Count"
            valueA={runA.SpanCount.toLocaleString()}
            valueB={runB.SpanCount.toLocaleString()}
            numA={runA.SpanCount}
            numB={runB.SpanCount}
          />
          <MetricRow
            label="Error Count"
            valueA={runA.ErrorCount.toLocaleString()}
            valueB={runB.ErrorCount.toLocaleString()}
            numA={runA.ErrorCount}
            numB={runB.ErrorCount}
            lowerIsBetter
          />
          <tr className="border-b border-[var(--border)]">
            <td className="py-3 px-4 text-sm text-right">
              <span className={runA.Status === "ok" ? "text-green-400" : "text-red-400"}>
                {runA.Status}
              </span>
            </td>
            <td className="py-3 px-4 text-xs text-[var(--text-muted)] text-center uppercase tracking-wide">
              Status
            </td>
            <td className="py-3 px-4 text-sm">
              <span className={runB.Status === "ok" ? "text-green-400" : "text-red-400"}>
                {runB.Status}
              </span>
            </td>
            <td />
          </tr>
        </tbody>
      </table>

      {/* Eval quality scores */}
      {evalNames.length > 0 && (
        <div className="border-t border-[var(--border)]">
          <div className="px-4 py-2 text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide bg-[var(--bg)]">
            Quality Scores
          </div>
          <table className="w-full">
            <tbody>
              {evalNames.map((name) => {
                const scoreA = avgScore(evalsA, name);
                const scoreB = avgScore(evalsB, name);
                const displayA = scoreA
                  ? `${scoreA.avg.toFixed(2)} / ${scoreA.count} spans`
                  : "—";
                const displayB = scoreB
                  ? `${scoreB.avg.toFixed(2)} / ${scoreB.count} spans`
                  : "—";

                return (
                  <tr key={name} className="border-b border-[var(--border)] last:border-0">
                    <td className="py-3 px-4 text-sm font-mono text-blue-300 text-right">{displayA}</td>
                    <td className="py-3 px-4 text-xs text-[var(--text-muted)] text-center uppercase tracking-wide whitespace-nowrap">
                      {name}
                    </td>
                    <td className="py-3 px-4 text-sm font-mono text-orange-300">{displayB}</td>
                    <td className="py-3 px-4 text-right">
                      {scoreA && scoreB ? (
                        <DeltaIndicator
                          valueA={scoreA.avg}
                          valueB={scoreB.avg}
                          format="number"
                          lowerIsBetter={false}
                        />
                      ) : (
                        <span className="text-[var(--text-muted)] text-xs">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
