"use client";

import type { Run, RunEvalSummary } from "@/lib/types";
import { toQualitySeries } from "@/lib/chart-utils";
import { CostChart } from "./CostChart";
import { LatencyChart } from "./LatencyChart";
import { ErrorRateChart } from "./ErrorRateChart";
import { TokenUsageChart } from "./TokenUsageChart";
import { EvalTrendChart } from "@/components/evals/EvalTrendChart";

interface Props {
  runs: Run[];
  evalSummaries?: RunEvalSummary[];
}

export function RunCharts({ runs, evalSummaries }: Props) {
  const sorted = [...runs].sort(
    (a, b) => new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime()
  );

  if (sorted.length < 2) {
    return null;
  }

  const qualityData = evalSummaries ? toQualitySeries(sorted, evalSummaries) : [];

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
      <CostChart runs={sorted} />
      <LatencyChart runs={sorted} />
      <ErrorRateChart runs={sorted} />
      <TokenUsageChart runs={sorted} />
      {qualityData.length >= 2 && <EvalTrendChart data={qualityData} />}
    </div>
  );
}
