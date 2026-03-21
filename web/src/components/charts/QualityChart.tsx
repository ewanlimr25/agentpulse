'use client'

import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine } from "recharts";
import type { ValueType } from "recharts/types/component/DefaultTooltipContent";
import type { QualityPoint } from "@/lib/chart-utils";
import { ChartCard } from "./ChartCard";

interface Props {
  data: QualityPoint[];
}

function formatScoreTooltip(v: ValueType | undefined): [string, string] {
  const score = typeof v === "number" ? v : Number(v ?? 0);
  return [(score * 100).toFixed(1) + "%", "Avg Quality"];
}

export function QualityChart({ data }: Props) {
  const xAxisInterval =
    data.length > 15 ? Math.floor(data.length / 8) : undefined;

  return (
    <ChartCard title="Quality Score (avg)" isEmpty={data.length < 2}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            {...(xAxisInterval !== undefined ? { interval: xAxisInterval } : {})}
          />
          <YAxis
            domain={[0, 1]}
            tickFormatter={(v: number) => `${(v * 100).toFixed(0)}%`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={44}
          />
          <Tooltip formatter={formatScoreTooltip} />
          <ReferenceLine y={0.7} stroke="#6b7280" strokeDasharray="4 2" strokeOpacity={0.5} />
          <Line
            type="monotone"
            dataKey="score"
            stroke="#34d399"
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
