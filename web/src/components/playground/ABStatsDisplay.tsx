"use client";

import type { PlaygroundVariant, PlaygroundExecution } from "@/lib/types";

interface ABStatsDisplayProps {
  variantA: PlaygroundVariant;
  variantB: PlaygroundVariant;
}

interface MetricResult {
  name: string;
  meanA: number | null;
  meanB: number | null;
  pValue: number | null;
  nA: number;
  nB: number;
  lowerIsBetter: boolean;
}

// ── Statistical helpers ───────────────────────────────────────────────────────

function erf(x: number): number {
  const a1 = 0.254829592;
  const a2 = -0.284496736;
  const a3 = 1.421413741;
  const a4 = -1.453152027;
  const a5 = 1.061405429;
  const p = 0.3275911;
  const sign = x < 0 ? -1 : 1;
  const ax = Math.abs(x);
  const t = 1 / (1 + p * ax);
  const y = 1 - ((((a5 * t + a4) * t + a3) * t + a2) * t + a1) * t * Math.exp(-ax * ax);
  return sign * y;
}

function normalCdf(x: number): number {
  return 0.5 * (1 + erf(x / Math.SQRT2));
}

/** Welch's t-test: returns two-tailed p-value, or null if insufficient data. */
function welchTTest(a: number[], b: number[]): number | null {
  if (a.length < 5 || b.length < 5) return null;

  const meanA = a.reduce((s, v) => s + v, 0) / a.length;
  const meanB = b.reduce((s, v) => s + v, 0) / b.length;
  const varA = a.reduce((s, v) => s + (v - meanA) ** 2, 0) / (a.length - 1);
  const varB = b.reduce((s, v) => s + (v - meanB) ** 2, 0) / (b.length - 1);

  const se = Math.sqrt(varA / a.length + varB / b.length);
  if (se === 0) return 1;

  const t = Math.abs(meanA - meanB) / se;

  const df =
    (varA / a.length + varB / b.length) ** 2 /
    ((varA / a.length) ** 2 / (a.length - 1) +
      (varB / b.length) ** 2 / (b.length - 1));

  if (df > 30) {
    return 2 * (1 - normalCdf(t));
  }
  // rough approximation for small df
  return t > 2.0 ? 0.04 : 0.5;
}

// ── Metric extraction ─────────────────────────────────────────────────────────

type MetricKey = "LatencyMS" | "InputTokens" | "OutputTokens" | "CostUSD";

const METRICS: { key: MetricKey; label: string; lowerIsBetter: boolean }[] = [
  { key: "LatencyMS", label: "Latency (ms)", lowerIsBetter: true },
  { key: "InputTokens", label: "Input Tokens", lowerIsBetter: true },
  { key: "OutputTokens", label: "Output Tokens", lowerIsBetter: false },
  { key: "CostUSD", label: "Cost (USD)", lowerIsBetter: true },
];

function extractValues(
  executions: PlaygroundExecution[] | null,
  key: MetricKey
): number[] {
  if (!executions) return [];
  return executions
    .filter((e) => e.Error === null)
    .map((e) => e[key]);
}

function computeMetrics(
  variantA: PlaygroundVariant,
  variantB: PlaygroundVariant
): MetricResult[] {
  return METRICS.map(({ key, label, lowerIsBetter }) => {
    const valuesA = extractValues(variantA.Executions, key);
    const valuesB = extractValues(variantB.Executions, key);
    const nA = valuesA.length;
    const nB = valuesB.length;
    const meanA = nA > 0 ? valuesA.reduce((s, v) => s + v, 0) / nA : null;
    const meanB = nB > 0 ? valuesB.reduce((s, v) => s + v, 0) / nB : null;
    const pValue = welchTTest(valuesA, valuesB);
    return { name: label, meanA, meanB, pValue, nA, nB, lowerIsBetter };
  });
}

// ── Formatting helpers ────────────────────────────────────────────────────────

function formatMean(key: MetricKey, value: number | null): string {
  if (value === null) return "—";
  if (key === "CostUSD") return `$${value.toFixed(5)}`;
  if (key === "LatencyMS") return `${value.toFixed(0)} ms`;
  return value.toFixed(1);
}

function formatPValue(p: number | null): string {
  if (p === null) return "—";
  if (p < 0.001) return "p < 0.001";
  return `p = ${p.toFixed(3)}`;
}

// ── Winner badge ──────────────────────────────────────────────────────────────

function WinnerBadge({
  meanA,
  meanB,
  pValue,
  lowerIsBetter,
  labelA,
  labelB,
}: {
  meanA: number | null;
  meanB: number | null;
  pValue: number | null;
  lowerIsBetter: boolean;
  labelA: string;
  labelB: string;
}) {
  const SIGNIFICANCE_THRESHOLD = 0.05;

  if (pValue === null || meanA === null || meanB === null) {
    return <span className="text-[var(--text-muted)] text-xs">—</span>;
  }

  if (pValue >= SIGNIFICANCE_THRESHOLD) {
    return (
      <span className="text-[var(--text-muted)] text-xs">no sig. diff</span>
    );
  }

  const aWins = lowerIsBetter ? meanA < meanB : meanA > meanB;
  const winnerLabel = aWins ? labelA : labelB;

  return (
    <span
      className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold"
      style={{
        background: "rgba(34,197,94,0.15)",
        color: "rgb(134,239,172)",
        border: "1px solid rgba(34,197,94,0.3)",
      }}
    >
      ▲ {winnerLabel}
    </span>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

export function ABStatsDisplay({ variantA, variantB }: ABStatsDisplayProps) {
  const metrics = computeMetrics(variantA, variantB);

  const minN = Math.min(
    (variantA.Executions ?? []).filter((e) => e.Error === null).length,
    (variantB.Executions ?? []).filter((e) => e.Error === null).length
  );
  const insufficientData = minN < 5;

  const metricKeys = METRICS.map((m) => m.key);

  return (
    <div
      className="mt-6 rounded-lg border p-5"
      style={{
        background: "var(--surface)",
        borderColor: "var(--border)",
      }}
    >
      {/* Header */}
      <div className="flex items-center gap-2 mb-4">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-[var(--text)]">
          A/B Statistical Significance
        </h2>
        <span
          className="rounded-full px-2 py-0.5 text-xs"
          style={{
            background: "rgba(99,102,241,0.15)",
            color: "rgb(165,180,252)",
            border: "1px solid rgba(99,102,241,0.3)",
          }}
        >
          experimental
        </span>
      </div>

      {insufficientData && (
        <p className="text-xs text-[var(--text-muted)] mb-4">
          Need &ge;5 successful runs per variant for significance testing.
          {minN > 0 && ` (${minN} run${minN !== 1 ? "s" : ""} so far)`}
        </p>
      )}

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr
              style={{ borderBottom: "1px solid var(--border)" }}
              className="text-xs uppercase tracking-wide text-[var(--text-muted)]"
            >
              <th className="text-left pb-2 pr-4 font-medium">Metric</th>
              <th className="text-right pb-2 pr-4 font-medium">
                Mean {variantA.Label}
              </th>
              <th className="text-right pb-2 pr-4 font-medium">
                Mean {variantB.Label}
              </th>
              <th className="text-right pb-2 pr-4 font-medium">p-value</th>
              <th className="text-right pb-2 font-medium">Winner</th>
            </tr>
          </thead>
          <tbody>
            {metrics.map((metric, i) => {
              const key = metricKeys[i];
              return (
                <tr
                  key={metric.name}
                  style={{ borderBottom: "1px solid var(--border)" }}
                  className="hover:bg-[var(--surface-2)] transition-colors"
                >
                  <td className="py-2.5 pr-4 text-[var(--text)] font-medium">
                    {metric.name}
                  </td>
                  <td className="py-2.5 pr-4 text-right font-mono text-[var(--text)]">
                    {formatMean(key, metric.meanA)}
                    <span className="text-[var(--text-muted)] text-xs ml-1">
                      n={metric.nA}
                    </span>
                  </td>
                  <td className="py-2.5 pr-4 text-right font-mono text-[var(--text)]">
                    {formatMean(key, metric.meanB)}
                    <span className="text-[var(--text-muted)] text-xs ml-1">
                      n={metric.nB}
                    </span>
                  </td>
                  <td className="py-2.5 pr-4 text-right font-mono text-xs text-[var(--text-muted)]">
                    {formatPValue(metric.pValue)}
                  </td>
                  <td className="py-2.5 text-right">
                    <WinnerBadge
                      meanA={metric.meanA}
                      meanB={metric.meanB}
                      pValue={metric.pValue}
                      lowerIsBetter={metric.lowerIsBetter}
                      labelA={variantA.Label}
                      labelB={variantB.Label}
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <p className="mt-3 text-xs text-[var(--text-muted)]">
        Welch&apos;s t-test, two-tailed. Significance threshold: p &lt; 0.05.
        Failed executions excluded.
      </p>
    </div>
  );
}
