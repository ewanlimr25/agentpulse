"use client";

import { formatCost } from "@/components/runs/RunRow";
import type { UserStats } from "@/lib/types";

interface Props {
  users: UserStats[];
}

export function UserCostTable({ users }: Props) {
  if (users.length === 0) {
    return (
      <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
        No user activity recorded yet. Set a user ID in your SDK with <code className="font-mono bg-[var(--surface-2)] px-1 rounded">set_user_id()</code>.
      </div>
    );
  }

  return (
    <div className="border border-[var(--border)] rounded-xl overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
            <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">User ID</th>
            <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Total Cost</th>
            <th className="px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider min-w-[140px]">% of Total</th>
            <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Runs</th>
            <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Tokens</th>
            <th className="text-right px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Errors</th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.UserID} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-2)] transition-colors">
              <td className="px-4 py-3 font-mono text-sm text-[var(--text)]">{u.UserID}</td>
              <td className="px-4 py-3 text-right tabular-nums text-indigo-400 font-medium">
                {formatCost(u.TotalCostUSD)}
              </td>
              <td className="px-4 py-3">
                <div className="flex items-center gap-2">
                  <div className="flex-1 bg-[var(--surface-2)] rounded-full h-1.5 overflow-hidden">
                    <div
                      className="h-full bg-indigo-500 rounded-full"
                      style={{ width: `${Math.min(u.CostPercent, 100)}%` }}
                    />
                  </div>
                  <span className="text-xs tabular-nums text-[var(--text-muted)] w-10 text-right">
                    {u.CostPercent.toFixed(1)}%
                  </span>
                </div>
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                {u.RunCount.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                {u.TotalTokens.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right tabular-nums text-[var(--text-muted)]">
                {u.ErrorCount > 0 ? (
                  <span className="text-red-400">{u.ErrorCount.toLocaleString()}</span>
                ) : (
                  <span>{u.ErrorCount}</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
