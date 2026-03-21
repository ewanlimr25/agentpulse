"use client";

import { useQuery } from "@tanstack/react-query";
import { budgetApi } from "@/lib/api";
import type { BudgetAlert } from "@/lib/types";

interface Props {
  projectId: string;
}

const actionStyles: Record<string, string> = {
  notify: "bg-indigo-950/60 text-indigo-300 border border-indigo-800",
  halt: "bg-red-950/60 text-red-300 border border-red-800",
};

export function AlertHistoryTable({ projectId }: Props) {
  const { data: alerts, isLoading, error } = useQuery({
    queryKey: ["budget-alerts", projectId],
    queryFn: () => budgetApi.listAlerts(projectId),
    refetchInterval: 30_000,
  });

  return (
    <section>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Alert History</h2>

      {error && (
        <div className="text-red-400 text-sm mb-4">
          Failed to load alerts: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <div className="text-[var(--text-muted)] text-sm">Loading...</div>
      ) : !alerts || alerts.length === 0 ? (
        <div className="border border-[var(--border)] rounded-xl px-6 py-10 text-center text-[var(--text-muted)] text-sm">
          No alerts triggered yet
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)] uppercase tracking-wide">
                <th className="pb-2 pr-4 font-medium">Triggered At</th>
                <th className="pb-2 pr-4 font-medium">Run ID</th>
                <th className="pb-2 pr-4 font-medium">Cost</th>
                <th className="pb-2 pr-4 font-medium">Threshold</th>
                <th className="pb-2 font-medium">Action</th>
              </tr>
            </thead>
            <tbody>
              {alerts.map((alert: BudgetAlert) => {
                const actionCls =
                  actionStyles[alert.ActionTaken] ??
                  "bg-zinc-800 text-zinc-400 border border-zinc-700";
                return (
                  <tr
                    key={alert.ID}
                    className="border-b border-[var(--border)] last:border-0"
                  >
                    <td className="py-3 pr-4 text-[var(--text-muted)] tabular-nums">
                      {new Date(alert.TriggeredAt).toLocaleString()}
                    </td>
                    <td className="py-3 pr-4">
                      <span className="font-mono text-[var(--text-muted)] text-xs">
                        {alert.RunID ? alert.RunID.slice(0, 12) : "—"}
                      </span>
                    </td>
                    <td className="py-3 pr-4 tabular-nums text-[var(--text)]">
                      ${alert.CurrentCost.toFixed(6)}
                    </td>
                    <td className="py-3 pr-4 tabular-nums text-[var(--text-muted)]">
                      ${alert.ThresholdUSD.toFixed(2)}
                    </td>
                    <td className="py-3">
                      <span
                        className={`rounded-full px-2 py-0.5 text-xs font-medium ${actionCls}`}
                      >
                        {alert.ActionTaken}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
