"use client";

import type { Run } from "@/lib/types";
import { CostChart } from "./CostChart";
import { LatencyChart } from "./LatencyChart";
import { ErrorRateChart } from "./ErrorRateChart";
import { TokenUsageChart } from "./TokenUsageChart";

interface Props {
  runs: Run[];
}

export function RunCharts({ runs }: Props) {
  const sorted = [...runs].sort(
    (a, b) => new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime()
  );

  if (sorted.length < 2) {
    return null;
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
      <CostChart runs={sorted} />
      <LatencyChart runs={sorted} />
      <ErrorRateChart runs={sorted} />
      <TokenUsageChart runs={sorted} />
    </div>
  );
}
