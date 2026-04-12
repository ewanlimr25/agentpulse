"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "@/lib/api";
import { formatCost } from "@/components/runs/RunRow";
import type { AnalyticsWindow, ModelStats } from "@/lib/types";

interface Props {
  projectId: string;
  window: AnalyticsWindow;
}

type SortKey = keyof Pick<
  ModelStats,
  | "CallCount"
  | "ErrorRate"
  | "AvgLatencyMS"
  | "P95LatencyMS"
  | "TotalCostUSD"
  | "AvgCostPerCall"
  | "TotalTokens"
  | "CostPerMillionTokens"
>;

function errorRateColor(rate: number): string {
  if (rate >= 20) return "text-red-400";
  if (rate >= 5) return "text-amber-400";
  return "text-green-400";
}

export function ModelStatsTable({ projectId, window }: Props) {
  const [sortKey, setSortKey] = useState<SortKey>("TotalCostUSD");
  const [sortAsc, setSortAsc] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["modelStats", projectId, window],
    queryFn: () => analyticsApi.modelStats(projectId, window),
  });

  const models = data?.models ?? [];

  const sorted = [...models].sort((a, b) => {
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
        {label}{active ? (sortAsc ? " \u2191" : " \u2193") : ""}
      </th>
    );
  }

  return (
    <div>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">Model Performance</h2>

      {isLoading && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          Loading\u2026
        </div>
      )}

      {!isLoading && models.length === 0 && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          No model calls in this window.
        </div>
      )}

      {!isLoading && models.length > 0 && (
        <div className="border border-[var(--border)] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">
                  Model
                </th>
                {headerBtn("Calls", "CallCount")}
                {headerBtn("Error %", "ErrorRate")}
                {headerBtn("Avg Latency", "AvgLatencyMS")}
                {headerBtn("P95 Latency", "P95LatencyMS")}
                {headerBtn("Cost", "TotalCostUSD")}
                {headerBtn("Avg $/Call", "AvgCostPerCall")}
                {headerBtn("Tokens", "TotalTokens")}
                {headerBtn("$/1M Tokens", "CostPerMillionTokens")}
              </tr>
            </thead>
            <tbody>
              {sorted.map((m) => (
                <tr key={m.ModelID} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-2)] transition-colors">
                  <td className="px-4 py-3 text-sm text-[var(--text)]">
                    <span className="font-mono">{m.ModelID}</span>
                    {m.Provider ? (
                      <span className="ml-2 text-xs text-[var(--text-muted)] bg-[var(--surface-2)] px-1.5 py-0.5 rounded">
                        {m.Provider}
                      </span>
                    ) : (
                      <span className="ml-2 text-xs text-amber-400">(unknown pricing)</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text)]">
                    {m.CallCount.toLocaleString()}
                  </td>
                  <td className={`px-4 py-3 text-right tabular-nums font-medium ${errorRateColor(m.ErrorRate)}`}>
                    {m.ErrorRate.toFixed(1)}%
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {m.AvgLatencyMS.toFixed(0)} ms
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {m.P95LatencyMS.toFixed(0)} ms
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {formatCost(m.TotalCostUSD)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {formatCost(m.AvgCostPerCall)}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {m.TotalTokens.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {formatCost(m.CostPerMillionTokens)}
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
