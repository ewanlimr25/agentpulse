"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "@/lib/api";
import { formatCost } from "@/components/runs/RunRow";
import type { AnalyticsWindow, ToolStats } from "@/lib/types";

interface Props {
  projectId: string;
  window: AnalyticsWindow;
}

type SortKey = keyof Pick<ToolStats, "CallCount" | "ErrorRate" | "AvgLatencyMS" | "P95LatencyMS" | "TotalCostUSD">;

function errorRateColor(rate: number): string {
  if (rate >= 20) return "text-red-400";
  if (rate >= 5) return "text-amber-400";
  return "text-green-400";
}

export function ToolAnalyticsTable({ projectId, window }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>("CallCount");
  const [sortAsc, setSortAsc] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["toolStats", projectId, window],
    queryFn: () => analyticsApi.toolStats(projectId, window),
  });

  const tools = data?.tools ?? [];

  const sorted = [...tools].sort((a, b) => {
    const diff = a[sortKey] - b[sortKey];
    return sortAsc ? diff : -diff;
  });

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      setSortAsc((v) => !v);
    } else {
      setSortKey(key);
      setSortAsc(false);
    }
  }

  function headerBtn(label: string, key: SortKey) {
    const active = sortKey === key;
    return (
      <th
        key={key}
        onClick={() => toggleSort(key)}
        className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider cursor-pointer select-none hover:text-[var(--text)] transition-colors"
      >
        {label}{active ? (sortAsc ? " ↑" : " ↓") : ""}
      </th>
    );
  }

  return (
    <div>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">Tool Performance</h2>

      {isLoading && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          Loading…
        </div>
      )}

      {!isLoading && tools.length === 0 && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          No tool calls in this window.
        </div>
      )}

      {!isLoading && tools.length > 0 && (
        <div className="border border-[var(--border)] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">
                  Tool
                </th>
                {headerBtn("Calls", "CallCount")}
                {headerBtn("Error %", "ErrorRate")}
                {headerBtn("Avg Latency", "AvgLatencyMS")}
                {headerBtn("P95 Latency", "P95LatencyMS")}
                {headerBtn("Cost", "TotalCostUSD")}
              </tr>
            </thead>
            <tbody>
              {sorted.map((t) => (
                <tr key={t.ToolName} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-2)] transition-colors">
                  <td className="px-4 py-3 font-mono text-sm text-[var(--text)]">{t.ToolName}</td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text)]">{t.CallCount.toLocaleString()}</td>
                  <td className={`px-4 py-3 text-right tabular-nums font-medium ${errorRateColor(t.ErrorRate)}`}>
                    {t.ErrorRate.toFixed(1)}%
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {t.AvgLatencyMS.toFixed(0)} ms
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {t.P95LatencyMS.toFixed(0)} ms
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {formatCost(t.TotalCostUSD)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
