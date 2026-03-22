"use client";

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts";
import type { ToolStats } from "@/lib/types";

interface Props {
  tools: ToolStats[];
}

const BAR_COLOR = "#6366f1"; // indigo-500

function truncate(s: string, n = 16): string {
  return s.length > n ? s.slice(0, n) + "…" : s;
}

export function TopToolsCharts({ tools }: Props) {
  if (tools.length === 0) return null;

  const slowest = [...tools]
    .sort((a, b) => b.P95LatencyMS - a.P95LatencyMS)
    .slice(0, 5)
    .map((t) => ({ name: truncate(t.ToolName), value: Math.round(t.P95LatencyMS) }));

  const mostCalled = [...tools]
    .sort((a, b) => b.CallCount - a.CallCount)
    .slice(0, 5)
    .map((t) => ({ name: truncate(t.ToolName), value: t.CallCount }));

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 mb-8">
      <ChartCard title="Slowest Tools (p95 ms)" data={slowest} unit="ms" />
      <ChartCard title="Most Called Tools" data={mostCalled} unit="" />
    </div>
  );
}

function ChartCard({
  title,
  data,
  unit,
}: {
  title: string;
  data: { name: string; value: number }[];
  unit: string;
}) {
  return (
    <div className="border border-[var(--border)] rounded-xl p-4">
      <p className="text-sm font-medium text-[var(--text)] mb-4">{title}</p>
      <ResponsiveContainer width="100%" height={160}>
        <BarChart
          data={data}
          layout="vertical"
          margin={{ top: 0, right: 40, bottom: 0, left: 0 }}
        >
          <XAxis
            type="number"
            tick={{ fill: "var(--text-muted)", fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(v) => (unit ? `${v}${unit}` : String(v))}
          />
          <YAxis
            type="category"
            dataKey="name"
            width={90}
            tick={{ fill: "var(--text-muted)", fontSize: 11 }}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip
            cursor={{ fill: "rgba(99,102,241,0.08)" }}
            contentStyle={{
              background: "var(--surface)",
              border: "1px solid var(--border)",
              borderRadius: 8,
              fontSize: 12,
              color: "var(--text)",
            }}
            formatter={(v) => [unit ? `${v} ${unit}` : v, ""]}
          />
          <Bar dataKey="value" radius={[0, 4, 4, 0]}>
            {data.map((_, i) => (
              <Cell key={i} fill={BAR_COLOR} fillOpacity={1 - i * 0.1} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
