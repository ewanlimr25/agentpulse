'use client'

import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from "recharts";
import type { ValueType } from "recharts/types/component/DefaultTooltipContent";
import type { Run } from "@/lib/types";
import { toCostSeries } from "@/lib/chart-utils";
import { ChartCard } from "./ChartCard";

interface Props {
  runs: Run[];
}

function formatCostTooltip(v: ValueType | undefined): [string, string] {
  const cost = typeof v === "number" ? v : Number(v ?? 0);
  return [`$${cost.toFixed(6)}`, "Cost"];
}

export function CostChart({ runs }: Props) {
  const data = toCostSeries(runs);
  const xAxisInterval =
    runs.length > 15 ? Math.floor(runs.length / 8) : undefined;

  return (
    <ChartCard title="Cost per Run" isEmpty={runs.length < 2}>
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
            tickFormatter={(v: number) => `$${v.toFixed(4)}`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={70}
          />
          <Tooltip formatter={formatCostTooltip} />
          <Area
            dataKey="cost"
            stroke="#818cf8"
            fill="#818cf8"
            fillOpacity={0.15}
          />
        </AreaChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
