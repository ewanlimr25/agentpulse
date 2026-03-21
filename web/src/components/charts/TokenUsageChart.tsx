"use client";

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";

import type { Run } from "@/lib/types";
import { toTokenSeries } from "@/lib/chart-utils";
import { ChartCard } from "./ChartCard";

interface Props {
  runs: Run[];
}

export function TokenUsageChart({ runs }: Props) {
  const data = toTokenSeries(runs);
  const xAxisInterval =
    runs.length > 15 ? Math.floor(runs.length / 8) : undefined;

  return (
    <ChartCard title="Token Usage" isEmpty={runs.length < 2}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            {...(xAxisInterval !== undefined ? { interval: xAxisInterval } : {})}
          />
          <YAxis
            tickFormatter={(v: number) =>
              v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v)
            }
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={45}
          />
          <Tooltip
            formatter={(v, name) => [typeof v === "number" ? v.toLocaleString() : String(v ?? ""), String(name ?? "")]}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Bar
            dataKey="input"
            stackId="tokens"
            fill="#818cf8"
            name="Input"
          />
          <Bar
            dataKey="output"
            stackId="tokens"
            fill="#a78bfa"
            name="Output"
          />
        </BarChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
