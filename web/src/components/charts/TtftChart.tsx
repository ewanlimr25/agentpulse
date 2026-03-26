"use client";

import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import type { ValueType, NameType } from "recharts/types/component/DefaultTooltipContent";

import type { Run } from "@/lib/types";
import { toTtftSeries } from "@/lib/chart-utils";
import { ChartCard } from "./ChartCard";

interface Props {
  runs: Run[];
}

export function TtftChart({ runs }: Props) {
  const data = toTtftSeries(runs);
  const xAxisInterval =
    data.length > 15 ? Math.floor(data.length / 8) : undefined;

  return (
    <ChartCard
      title="TTFT per Run (streaming spans only)"
      isEmpty={data.length < 2}
      emptyMessage="No streaming span data"
    >
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
            tickFormatter={(v: number) => `${v}ms`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={55}
          />
          <Tooltip
            formatter={(v: ValueType | undefined, name: NameType | undefined) => [
              typeof v === "number" ? `${v.toFixed(0)}ms` : (v ?? ""),
              name === "p50" ? "p50 TTFT" : "p95 TTFT",
            ]}
          />
          <Legend
            formatter={(value) => (value === "p50" ? "p50" : "p95")}
            iconType="plainline"
          />
          <Area
            dataKey="p50"
            stroke="#22d3ee"
            fill="#22d3ee"
            fillOpacity={0.15}
            strokeWidth={1.5}
          />
          <Area
            dataKey="p95"
            stroke="#0ea5e9"
            fill="#0ea5e9"
            fillOpacity={0.08}
            strokeWidth={1.5}
            strokeDasharray="4 2"
          />
        </AreaChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
