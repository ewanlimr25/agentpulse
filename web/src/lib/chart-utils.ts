import type { Run, RunEvalSummary } from "@/lib/types";

export interface CostPoint {
  label: string;
  cost: number;
  runId: string;
}

export interface LatencyPoint {
  label: string;
  durationSec: number;
  runId: string;
}

export interface ErrorPoint {
  label: string;
  ok: number;
  error: number;
  runId: string;
}

export interface TokenPoint {
  label: string;
  input: number;
  output: number;
  runId: string;
}

/**
 * Format an ISO date string as "MM/DD HH:mm".
 * Uses UTC fields to avoid locale-dependent offsets; callers that want
 * local-time formatting should pre-process the label themselves.
 */
function formatLabel(isoString: string): string {
  const date = new Date(isoString);
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  const hh = String(date.getHours()).padStart(2, "0");
  const min = String(date.getMinutes()).padStart(2, "0");
  return `${mm}/${dd} ${hh}:${min}`;
}

/**
 * Convert a Run array into cost chart data points.
 * Caller is responsible for sorting runs before passing them in.
 */
export function toCostSeries(runs: Run[]): CostPoint[] {
  return runs.map((run) => ({
    label: formatLabel(run.StartTime),
    cost: run.TotalCostUSD,
    runId: run.RunID,
  }));
}

/**
 * Convert a Run array into latency chart data points.
 * DurationMS is converted to seconds, rounded to 2 decimal places.
 */
export function toLatencySeries(runs: Run[]): LatencyPoint[] {
  return runs.map((run) => ({
    label: formatLabel(run.StartTime),
    durationSec: Math.round((run.DurationMS / 1000) * 100) / 100,
    runId: run.RunID,
  }));
}

/**
 * Convert a Run array into error/ok stacked bar chart data points.
 * Each run contributes exactly 1 to either the ok or error bucket.
 */
export function toErrorSeries(runs: Run[]): ErrorPoint[] {
  return runs.map((run) => ({
    label: formatLabel(run.StartTime),
    ok: run.Status === "ok" ? 1 : 0,
    error: run.Status === "error" ? 1 : 0,
    runId: run.RunID,
  }));
}

/**
 * Convert a Run array into token usage chart data points.
 */
export function toTokenSeries(runs: Run[]): TokenPoint[] {
  return runs.map((run) => ({
    label: formatLabel(run.StartTime),
    input: run.TotalInputTokens,
    output: run.TotalOutputTokens,
    runId: run.RunID,
  }));
}

export interface QualityPoint {
  label: string;
  score: number;
  runId: string;
}

/**
 * Join sorted runs with their eval summaries to produce quality chart data.
 * Runs without an eval score are omitted.
 */
export function toQualitySeries(runs: Run[], summaries: RunEvalSummary[]): QualityPoint[] {
  const scoreByRunId = new Map(summaries.map((s) => [s.RunID, s.AvgScore]));
  const points: QualityPoint[] = [];
  for (const run of runs) {
    const score = scoreByRunId.get(run.RunID);
    if (score !== undefined) {
      points.push({ label: formatLabel(run.StartTime), score, runId: run.RunID });
    }
  }
  return points;
}
