"use client";

import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import type { ValueType } from "recharts/types/component/DefaultTooltipContent";

import type { Run } from "@/lib/types";
import { toLatencySeries } from "@/lib/chart-utils";
import { ChartCard } from "./ChartCard";

interface Props {
  runs: Run[];
}

export function LatencyChart({ runs }: Props) {
  const data = toLatencySeries(runs);
  const xAxisInterval =
    runs.length > 15 ? Math.floor(runs.length / 8) : undefined;

  return (
    <ChartCard title="Latency per Run" isEmpty={runs.length < 2}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            {...(xAxisInterval !== undefined ? { interval: xAxisInterval } : {})}
          />
          <YAxis
            tickFormatter={(v: number) => `${v}s`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={45}
          />
          <Tooltip
            formatter={(v: ValueType | undefined) => [
              typeof v === "number" ? `${v}s` : v ?? "",
              "Duration",
            ]}
          />
          <Area
            dataKey="durationSec"
            stroke="#a78bfa"
            fill="#a78bfa"
            fillOpacity={0.15}
          />
        </AreaChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
