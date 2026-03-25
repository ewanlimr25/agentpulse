"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { evalsApi } from "@/lib/api";
import type { EvalConfig } from "@/lib/types";
import { BUILTIN_EVAL_NAMES } from "@/lib/types";

interface Props {
  projectId: string;
  configs: EvalConfig[];
  onAddCustom: () => void;
}

function evalLabel(name: string): string {
  return name.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

export function EvalConfigTable({ projectId, configs, onAddCustom }: Props) {
  const queryClient = useQueryClient();
  const [busyName, setBusyName] = useState<string | null>(null);

  const upsertMutation = useMutation({
    mutationFn: (cfg: { eval_name: string; enabled: boolean; span_kind: string }) =>
      evalsApi.upsertConfig(projectId, cfg),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evalConfigs", projectId] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (evalName: string) => evalsApi.deleteConfig(projectId, evalName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evalConfigs", projectId] });
    },
  });

  // Build display rows: built-ins first (using configs or defaults), then custom.
  const configByName = new Map(configs.map((c) => [c.EvalName, c]));

  const builtinRows: EvalConfig[] = BUILTIN_EVAL_NAMES.map((name) => {
    return configByName.get(name) ?? {
      ID: "",
      ProjectID: projectId,
      EvalName: name,
      Enabled: name === "relevance",
      SpanKind: name === "tool_correctness" ? "tool.call" : "llm.call",
      PromptVersion: 1,
      CreatedAt: "",
      UpdatedAt: "",
    } as EvalConfig;
  });

  const customRows = configs.filter((c) => c.PromptTemplate !== undefined && c.PromptTemplate !== null);

  const allRows = [...builtinRows, ...customRows];

  async function handleToggle(cfg: EvalConfig) {
    setBusyName(cfg.EvalName);
    try {
      await upsertMutation.mutateAsync({
        eval_name: cfg.EvalName,
        enabled: !cfg.Enabled,
        span_kind: cfg.SpanKind,
        ...(cfg.PromptTemplate ? { prompt_template: cfg.PromptTemplate } : {}),
      });
    } finally {
      setBusyName(null);
    }
  }

  async function handleDelete(evalName: string) {
    if (!confirm(`Delete custom eval "${evalName}"?`)) return;
    setBusyName(evalName);
    try {
      await deleteMutation.mutateAsync(evalName);
    } finally {
      setBusyName(null);
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Eval Configuration</p>
        <button
          onClick={onAddCustom}
          className="text-xs px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium transition-colors"
        >
          + Add Custom Eval
        </button>
      </div>
      <div className="border border-[var(--border)] rounded-xl overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
              <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Type</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Span Kind</th>
              <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Status</th>
              <th className="px-4 py-2.5" />
            </tr>
          </thead>
          <tbody>
            {allRows.map((cfg) => (
              <tr key={cfg.EvalName} className="border-t border-[var(--border)] first:border-0">
                <td className="px-4 py-3">
                  <span className="text-[var(--text)] font-medium">{evalLabel(cfg.EvalName)}</span>
                  {cfg.PromptTemplate && (
                    <>
                      <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded bg-violet-950/40 border border-violet-700 text-violet-400">custom</span>
                      <span className="ml-1 text-[10px] px-1.5 py-0.5 rounded bg-[var(--surface-2)] border border-[var(--border)] text-[var(--text-muted)] font-mono">v{cfg.PromptVersion}</span>
                    </>
                  )}
                  {cfg.ScopeFilter?.agent_name?.length ? (
                    <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded bg-amber-950/30 border border-amber-700/50 text-amber-400" title={cfg.ScopeFilter.agent_name.join(", ")}>
                      {cfg.ScopeFilter.agent_name.length === 1 ? cfg.ScopeFilter.agent_name[0] : `${cfg.ScopeFilter.agent_name.length} agents`}
                    </span>
                  ) : null}
                </td>
                <td className="px-4 py-3 text-xs text-[var(--text-muted)] font-mono">{cfg.SpanKind}</td>
                <td className="px-4 py-3">
                  <span className={`text-xs font-medium ${cfg.Enabled ? "text-green-400" : "text-[var(--text-muted)]"}`}>
                    {cfg.Enabled ? "Active" : "Inactive"}
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <button
                      onClick={() => handleToggle(cfg)}
                      disabled={busyName === cfg.EvalName}
                      className="text-xs px-2.5 py-1 rounded bg-[var(--surface-2)] hover:bg-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-40"
                    >
                      {cfg.Enabled ? "Disable" : "Enable"}
                    </button>
                    {cfg.PromptTemplate && (
                      <button
                        onClick={() => handleDelete(cfg.EvalName)}
                        disabled={busyName === cfg.EvalName}
                        className="text-xs px-2.5 py-1 rounded bg-red-950/30 hover:bg-red-950/50 text-red-400 transition-colors disabled:opacity-40"
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
    </div>
  );
}
