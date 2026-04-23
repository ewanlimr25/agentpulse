"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { alertsApi } from "@/lib/api";
import type { AlertRule } from "@/lib/types";
import { SIGNAL_LABELS, SIGNAL_UNITS, COMPARE_LABELS, formatWindow } from "./alertUtils";

interface Props {
  projectId: string;
  onAddRule: () => void;
  onEditRule: (rule: AlertRule) => void;
}

export function AlertRulesTable({ projectId, onAddRule, onEditRule }: Props) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["alertRules", projectId],
    queryFn: () => alertsApi.listRules(projectId),
  });
  const rules = data ?? [];

  const toggleMutation = useMutation({
    mutationFn: (rule: AlertRule) =>
      alertsApi.updateRule(projectId, rule.ID, {
        name: rule.Name,
        signal_type: rule.SignalType,
        threshold: rule.Threshold,
        compare_op: rule.CompareOp,
        window_seconds: rule.WindowSeconds,
        scope_filter: rule.ScopeFilter,
        webhook_url: rule.WebhookURL,
        slack_webhook_url: rule.SlackWebhookURL,
        discord_webhook_url: rule.DiscordWebhookURL,
        enabled: !rule.Enabled,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alertRules", projectId] }),
  });

  const deleteMutation = useMutation({
    mutationFn: (ruleId: string) => alertsApi.deleteRule(projectId, ruleId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alertRules", projectId] }),
  });

  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-[var(--text)]">Alert Rules</h2>
        <button
          onClick={onAddRule}
          className="text-sm bg-indigo-600 hover:bg-indigo-500 text-white px-3 py-1.5 rounded-lg transition-colors"
        >
          + Add Rule
        </button>
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">Loading rules...</p>}

      {!isLoading && rules.length === 0 && (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          No alert rules yet. Add one to get notified when a signal crosses a threshold.
        </div>
      )}

      {rules.length > 0 && (
        <div className="border border-[var(--border)] rounded-xl overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Name</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Signal</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Condition</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Window</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Scope</th>
                <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider">Enabled</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr key={rule.ID} className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-2)] transition-colors">
                  <td className="px-4 py-3 text-[var(--text)] font-medium">
                    <div className="flex items-center gap-1.5">
                      {rule.Name}
                      {rule.LastChannelError && (
                        <span
                          title={rule.LastChannelError}
                          className="inline-flex items-center text-xs bg-red-950/50 text-red-400 border border-red-800/40 px-1.5 py-0.5 rounded-full cursor-help"
                        >
                          Channel error
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-muted)]">
                    <span className="bg-indigo-950/60 text-indigo-300 text-xs px-2 py-0.5 rounded-full">
                      {SIGNAL_LABELS[rule.SignalType]}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-muted)] font-mono text-xs">
                    {COMPARE_LABELS[rule.CompareOp]} {rule.Threshold}{SIGNAL_UNITS[rule.SignalType]}
                  </td>
                  <td className="px-4 py-3 text-[var(--text-muted)]">{formatWindow(rule.WindowSeconds)}</td>
                  <td className="px-4 py-3 text-[var(--text-muted)] font-mono text-xs">
                    {rule.ScopeFilter ?? <span className="opacity-40">—</span>}
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => toggleMutation.mutate(rule)}
                      className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${rule.Enabled ? "bg-indigo-600" : "bg-zinc-600"}`}
                    >
                      <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${rule.Enabled ? "translate-x-4" : "translate-x-1"}`} />
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2 justify-end">
                      <button
                        onClick={() => onEditRule(rule)}
                        className="text-xs text-[var(--text-muted)] hover:text-indigo-400 transition-colors"
                      >
                        Edit
                      </button>
                      {confirmDelete === rule.ID ? (
                        <div className="flex items-center gap-1">
                          <button
                            onClick={() => { deleteMutation.mutate(rule.ID); setConfirmDelete(null); }}
                            className="text-xs text-red-400 hover:text-red-300"
                          >
                            Confirm
                          </button>
                          <button onClick={() => setConfirmDelete(null)} className="text-xs text-[var(--text-muted)]">
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <button
                          onClick={() => setConfirmDelete(rule.ID)}
                          className="text-xs text-[var(--text-muted)] hover:text-red-400 transition-colors"
                        >
                          Delete
                        </button>
                      )}
                    </div>
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
