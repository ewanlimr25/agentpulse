"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { budgetApi } from "@/lib/api";
import type { BudgetRule } from "@/lib/types";

interface Props {
  projectId: string;
  onAddRule: () => void;
}

const actionStyles: Record<string, string> = {
  notify: "bg-indigo-950/60 text-indigo-300 border border-indigo-800",
  halt: "bg-red-950/60 text-red-300 border border-red-800",
};

const scopeStyles: Record<string, string> = {
  run: "bg-sky-950/60 text-sky-300 border border-sky-800",
  agent: "bg-violet-950/60 text-violet-300 border border-violet-800",
  window: "bg-zinc-800 text-zinc-400 border border-zinc-700",
};

function ActionBadge({ action }: { action: string }) {
  const cls = actionStyles[action] ?? "bg-zinc-800 text-zinc-400 border border-zinc-700";
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>
      {action}
    </span>
  );
}

function ScopeBadge({ scope }: { scope: string }) {
  const cls = scopeStyles[scope] ?? "bg-zinc-800 text-zinc-400 border border-zinc-700";
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>
      {scope}
    </span>
  );
}

function EnabledToggle({
  enabled,
  onToggle,
  disabled,
}: {
  enabled: boolean;
  onToggle: () => void;
  disabled: boolean;
}) {
  return (
    <button
      onClick={onToggle}
      disabled={disabled}
      aria-checked={enabled}
      role="switch"
      className={`relative w-10 h-5 rounded-full transition-colors focus:outline-none disabled:opacity-50 ${
        enabled ? "bg-indigo-600" : "bg-zinc-600"
      }`}
    >
      <span
        className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
          enabled ? "translate-x-5" : "translate-x-0"
        }`}
      />
    </button>
  );
}

export function BudgetRulesTable({ projectId, onAddRule }: Props) {
  const queryClient = useQueryClient();
  const queryKey = ["budget-rules", projectId];

  const { data: rules, isLoading, error } = useQuery({
    queryKey,
    queryFn: () => budgetApi.listRules(projectId),
  });

  const deleteMutation = useMutation({
    mutationFn: (ruleId: string) => budgetApi.deleteRule(projectId, ruleId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: ({ ruleId, enabled }: { ruleId: string; enabled: boolean }) =>
      budgetApi.updateRule(projectId, ruleId, { Enabled: enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });

  return (
    <section>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-[var(--text)]">Budget Rules</h2>
        <button
          onClick={onAddRule}
          className="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-medium transition-colors"
        >
          Add Rule
        </button>
      </div>

      {error && (
        <div className="text-red-400 text-sm mb-4">
          Failed to load rules: {(error as Error).message}
        </div>
      )}

      {isLoading ? (
        <div className="flex flex-col gap-2">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-12 rounded-lg bg-[var(--surface)] border border-[var(--border)] animate-pulse"
            />
          ))}
        </div>
      ) : !rules || rules.length === 0 ? (
        <div className="border border-[var(--border)] rounded-xl px-6 py-10 text-center text-[var(--text-muted)] text-sm">
          No budget rules yet
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)] uppercase tracking-wide">
                <th className="pb-2 pr-4 font-medium">Name</th>
                <th className="pb-2 pr-4 font-medium">Threshold</th>
                <th className="pb-2 pr-4 font-medium">Action</th>
                <th className="pb-2 pr-4 font-medium">Scope</th>
                <th className="pb-2 pr-4 font-medium">Enabled</th>
                <th className="pb-2 font-medium">Delete</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule: BudgetRule) => (
                <tr
                  key={rule.ID}
                  className="border-b border-[var(--border)] last:border-0"
                >
                  <td className="py-3 pr-4 text-[var(--text)] font-medium">{rule.Name}</td>
                  <td className="py-3 pr-4 tabular-nums text-[var(--text-muted)]">
                    ${rule.ThresholdUSD.toFixed(2)}
                  </td>
                  <td className="py-3 pr-4">
                    <ActionBadge action={rule.Action} />
                  </td>
                  <td className="py-3 pr-4">
                    <ScopeBadge scope={rule.Scope} />
                  </td>
                  <td className="py-3 pr-4">
                    <EnabledToggle
                      enabled={rule.Enabled}
                      onToggle={() =>
                        toggleMutation.mutate({ ruleId: rule.ID, enabled: !rule.Enabled })
                      }
                      disabled={toggleMutation.isPending}
                    />
                  </td>
                  <td className="py-3">
                    <button
                      onClick={() => deleteMutation.mutate(rule.ID)}
                      disabled={deleteMutation.isPending}
                      className="text-[var(--text-muted)] hover:text-red-400 transition-colors text-lg leading-none disabled:opacity-50"
                      aria-label={`Delete rule ${rule.Name}`}
                    >
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
