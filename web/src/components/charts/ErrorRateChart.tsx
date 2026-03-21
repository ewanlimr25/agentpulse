"use client";

import {
  Bar,
  BarChart,
  Legend,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { ValueType, NameType } from "recharts/types/component/DefaultTooltipContent";

import { toErrorSeries } from "@/lib/chart-utils";
import type { Run } from "@/lib/types";

import { ChartCard } from "./ChartCard";

interface Props {
  runs: Run[];
}

function formatStatus(value: ValueType | undefined, name: NameType | undefined): [string, NameType] {
  return [value === 1 ? "Yes" : "No", name ?? ""];
}

export function ErrorRateChart({ runs }: Props) {
  const data = toErrorSeries(runs);
  const xAxisInterval =
    runs.length > 15 ? Math.floor(runs.length / 8) : undefined;

  return (
    <ChartCard title="Run Status" isEmpty={runs.length < 2}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            {...(xAxisInterval !== undefined ? { interval: xAxisInterval } : {})}
          />
          <YAxis hide={true} />
          <Tooltip formatter={formatStatus} />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Bar dataKey="ok" stackId="status" fill="#22c55e" name="OK" />
          <Bar dataKey="error" stackId="status" fill="#ef4444" name="Error" />
        </BarChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
