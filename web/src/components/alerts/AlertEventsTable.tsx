"use client";

import { useQuery } from "@tanstack/react-query";
import { alertsApi } from "@/lib/api";
import { SIGNAL_LABELS, COMPARE_LABELS, formatSignalValue } from "./alertUtils";
import type { SignalType, CompareOp } from "@/lib/types";

interface Props {
  projectId: string;
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export function AlertEventsTable({ projectId }: Props) {
  const { data } = useQuery({
    queryKey: ["alertEvents", projectId],
    queryFn: () => alertsApi.listEvents(projectId, 50),
    refetchInterval: 10_000,
  });
  const events = data ?? [];

  return (
    <div>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">Alert History</h2>

      {events.length === 0 && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          No alert events yet.
        </div>
      )}

      {events.length > 0 && (
        <div className="border border-[var(--border)] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Time</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Signal</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Value</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Threshold</th>
              </tr>
            </thead>
            <tbody>
              {events.map((evt) => (
                <tr key={evt.ID} className="border-b border-[var(--border)] last:border-0">
                  <td className="px-4 py-3 text-[var(--text-muted)] text-xs tabular-nums">
                    {relativeTime(evt.TriggeredAt)}
                  </td>
                  <td className="px-4 py-3">
                    <span className="bg-amber-950/60 text-amber-300 text-xs px-2 py-0.5 rounded-full">
                      {SIGNAL_LABELS[evt.SignalType]}
                    </span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-red-400">
                    {formatSignalValue(evt.SignalType, evt.CurrentValue)}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-muted)] font-mono text-xs">
                    {COMPARE_LABELS[evt.CompareOp as CompareOp]} {formatSignalValue(evt.SignalType as SignalType, evt.Threshold)}
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
