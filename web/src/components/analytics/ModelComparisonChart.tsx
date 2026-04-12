"use client";

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import type { ValueType } from "recharts/types/component/DefaultTooltipContent";
import { ChartCard } from "@/components/charts/ChartCard";
import type { ModelStats } from "@/lib/types";

interface Props {
  models: ModelStats[];
}

function formatTooltip(v: ValueType | undefined): [string, string] {
  const cost = typeof v === "number" ? v : Number(v ?? 0);
  return [`$${cost.toFixed(2)}`, "$/1M Tokens"];
}

export function ModelComparisonChart({ models }: Props) {
  const data = [...models]
    .sort((a, b) => b.CostPerMillionTokens - a.CostPerMillionTokens)
    .map((m) => ({
      name: m.ModelID,
      costPerMillion: m.CostPerMillionTokens,
    }));

  return (
    <ChartCard title="Cost per 1M Tokens by Model" isEmpty={models.length === 0}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data} layout="vertical" margin={{ left: 20, right: 20 }}>
          <XAxis
            type="number"
            tickFormatter={(v: number) => `$${v.toFixed(1)}`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            type="category"
            dataKey="name"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={160}
          />
          <Tooltip formatter={formatTooltip} />
          <Bar dataKey="costPerMillion" fill="#818cf8" radius={[0, 4, 4, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
