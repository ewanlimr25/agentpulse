"use client";

import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "@/lib/api";
import { formatCost } from "@/components/runs/RunRow";
import type { AnalyticsWindow } from "@/lib/types";

interface Props {
  projectId: string;
  window: AnalyticsWindow;
}

export function AgentCostTable({ projectId, window }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: ["agentCostStats", projectId, window],
    queryFn: () => analyticsApi.agentCostStats(projectId, window),
  });

  const agents = data?.agents ?? [];

  return (
    <div>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">Agent Cost Breakdown</h2>

      {isLoading && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          Loading…
        </div>
      )}

      {!isLoading && agents.length === 0 && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          No agent spans in this window.
        </div>
      )}

      {!isLoading && agents.length > 0 && (
        <div className="border border-[var(--border)] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Agent</th>
                <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Total Cost</th>
                <th className="px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider min-w-[140px]">% of Total</th>
                <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Calls</th>
                <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Avg Cost/Call</th>
              </tr>
            </thead>
            <tbody>
              {agents.map((a) => (
                <tr key={a.AgentName} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-2)] transition-colors">
                  <td className="px-4 py-3 font-mono text-sm text-[var(--text)]">{a.AgentName}</td>
                  <td className="px-4 py-3 text-right tabular-nums text-indigo-400 font-medium">
                    {formatCost(a.TotalCostUSD)}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <div className="flex-1 bg-[var(--surface-2)] rounded-full h-1.5 overflow-hidden">
                        <div
                          className="h-full bg-indigo-500 rounded-full"
                          style={{ width: `${Math.min(a.CostPercent, 100)}%` }}
                        />
                      </div>
                      <span className="text-xs tabular-nums text-[var(--text-muted)] w-10 text-right">
                        {a.CostPercent.toFixed(1)}%
                      </span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {a.CallCount.toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                    {formatCost(a.AvgCostPerCall)}
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
