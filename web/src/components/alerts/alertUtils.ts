import type { SignalType, CompareOp } from "@/lib/types";

export const SIGNAL_LABELS: Record<SignalType, string> = {
  error_rate:    "Error Rate",
  latency_p95:   "Latency P95",
  quality_score: "Quality Score",
  tool_failure:  "Tool Failure Rate",
};

export const SIGNAL_UNITS: Record<SignalType, string> = {
  error_rate:    "%",
  latency_p95:   "ms",
  quality_score: "",   // 0–1 score, no unit
  tool_failure:  "%",
};

export const COMPARE_LABELS: Record<CompareOp, string> = {
  gt: "above",
  lt: "below",
};

export function formatSignalValue(signalType: SignalType, value: number): string {
  const unit = SIGNAL_UNITS[signalType];
  if (signalType === "latency_p95") return `${value.toFixed(0)}ms`;
  if (signalType === "quality_score") return value.toFixed(3);
  return `${value.toFixed(1)}%`;
}

export function formatWindow(secs: number): string {
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${secs / 60}m`;
  return `${secs / 3600}h`;
}
