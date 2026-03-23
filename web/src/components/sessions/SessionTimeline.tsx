"use client";

import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import type { ValueType } from "recharts/types/component/DefaultTooltipContent";
import type { Run } from "@/lib/types";
import { formatCost } from "@/components/runs/RunRow";

interface Props {
  runs: Run[];
}

function formatCostTooltip(v: ValueType | undefined): [string, string] {
  const cost = typeof v === "number" ? v : Number(v ?? 0);
  return [formatCost(cost), "Cost"];
}

export function SessionTimeline({ runs }: Props) {
  const sorted = [...runs].sort(
    (a, b) => new Date(a.StartTime).getTime() - new Date(b.StartTime).getTime()
  );

  const data = sorted.map((run, i) => ({
    label: `Turn ${i + 1}`,
    cost: run.TotalCostUSD,
    runId: run.RunID,
  }));

  const isSingle = data.length <= 1;

  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-4">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
        Cost per Turn
      </h3>
      {isSingle && (
        <p className="text-xs text-[var(--text-muted)] mb-2">
          Single-run session — more turns will show a trend
        </p>
      )}
      <div className="h-48">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
            <XAxis
              dataKey="label"
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={(v: number) => formatCost(v)}
              tick={{ fontSize: 11 }}
              tickLine={false}
              axisLine={false}
              width={70}
            />
            <Tooltip formatter={formatCostTooltip} />
            <Line
              dataKey="cost"
              stroke="#818cf8"
              strokeWidth={2}
              dot={{ r: 4, fill: "#818cf8", strokeWidth: 0 }}
              activeDot={{ r: 6 }}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
