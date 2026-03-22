"use client";

import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { alertsApi } from "@/lib/api";
import type { AlertRule, SignalType, CompareOp } from "@/lib/types";
import { SIGNAL_LABELS, SIGNAL_UNITS } from "./alertUtils";

interface Props {
  projectId: string;
  isOpen: boolean;
  editRule?: AlertRule | null;
  onClose: () => void;
}

const SIGNAL_TYPES: SignalType[] = ["error_rate", "latency_p95", "quality_score", "tool_failure"];
const WINDOW_PRESETS = [
  { label: "5 min", value: 300 },
  { label: "15 min", value: 900 },
  { label: "1 hour", value: 3600 },
  { label: "24 hours", value: 86400 },
];

export function AddAlertRuleModal({ projectId, isOpen, editRule, onClose }: Props) {
  const qc = useQueryClient();

  const [name, setName] = useState("");
  const [signalType, setSignalType] = useState<SignalType>("error_rate");
  const [threshold, setThreshold] = useState("");
  const [compareOp, setCompareOp] = useState<CompareOp>("gt");
  const [windowSeconds, setWindowSeconds] = useState(900);
  const [scopeFilter, setScopeFilter] = useState("");
  const [webhookURL, setWebhookURL] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (editRule) {
      setName(editRule.Name);
      setSignalType(editRule.SignalType);
      setThreshold(String(editRule.Threshold));
      setCompareOp(editRule.CompareOp);
      setWindowSeconds(editRule.WindowSeconds);
      setScopeFilter(editRule.ScopeFilter ?? "");
      setWebhookURL(editRule.WebhookURL ?? "");
      setEnabled(editRule.Enabled);
    } else {
      setName(""); setSignalType("error_rate"); setThreshold("");
      setCompareOp("gt"); setWindowSeconds(900); setScopeFilter("");
      setWebhookURL(""); setEnabled(true);
    }
    setError("");
  }, [editRule, isOpen]);

  const mutation = useMutation({
    mutationFn: (body: Parameters<typeof alertsApi.createRule>[1]) =>
      editRule
        ? alertsApi.updateRule(projectId, editRule.ID, body)
        : alertsApi.createRule(projectId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["alertRules", projectId] });
      onClose();
    },
    onError: (e) => setError((e as Error).message),
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const t = parseFloat(threshold);
    if (!name.trim()) { setError("Name is required"); return; }
    if (isNaN(t) || t < 0) { setError("Threshold must be a non-negative number"); return; }
    if (signalType === "tool_failure" && !scopeFilter.trim()) {
      setError("Tool name is required for Tool Failure Rate alerts");
      return;
    }
    mutation.mutate({
      name: name.trim(),
      signal_type: signalType,
      threshold: t,
      compare_op: compareOp,
      window_seconds: windowSeconds,
      scope_filter: scopeFilter.trim() || undefined,
      webhook_url: webhookURL.trim() || undefined,
      enabled,
    });
  }

  if (!isOpen) return null;

  const unit = SIGNAL_UNITS[signalType];
  const thresholdHint = signalType === "quality_score"
    ? "0.0 – 1.0"
    : signalType === "latency_p95"
    ? "milliseconds"
    : "percentage (0–100)";

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 px-4">
      <div className="w-full max-w-lg bg-[var(--surface)] border border-[var(--border)] rounded-xl px-8 py-8">
        <h2 className="text-lg font-semibold text-[var(--text)] mb-6">
          {editRule ? "Edit Alert Rule" : "Add Alert Rule"}
        </h2>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-xs text-[var(--text-muted)] mb-1">Rule Name</label>
            <input
              autoFocus
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="High error rate"
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">Signal Type</label>
              <select
                value={signalType}
                onChange={(e) => { setSignalType(e.target.value as SignalType); setCompareOp(e.target.value === "quality_score" ? "lt" : "gt"); }}
                className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
              >
                {SIGNAL_TYPES.map((s) => (
                  <option key={s} value={s}>{SIGNAL_LABELS[s]}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">Direction</label>
              <select
                value={compareOp}
                onChange={(e) => setCompareOp(e.target.value as CompareOp)}
                className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
              >
                <option value="gt">Above threshold (↑)</option>
                <option value="lt">Below threshold (↓)</option>
              </select>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">
                Threshold{unit ? ` (${thresholdHint})` : ` (${thresholdHint})`}
              </label>
              <div className="relative">
                <input
                  type="number"
                  step="any"
                  min="0"
                  value={threshold}
                  onChange={(e) => setThreshold(e.target.value)}
                  placeholder={signalType === "quality_score" ? "0.6" : "10"}
                  className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500 pr-8"
                />
                {unit && (
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-[var(--text-muted)]">{unit}</span>
                )}
              </div>
            </div>

            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">Window</label>
              <select
                value={windowSeconds}
                onChange={(e) => setWindowSeconds(Number(e.target.value))}
                className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
              >
                {WINDOW_PRESETS.map((p) => (
                  <option key={p.value} value={p.value}>{p.label}</option>
                ))}
              </select>
            </div>
          </div>

          {signalType === "tool_failure" && (
            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">Tool Name (span_name)</label>
              <input
                type="text"
                value={scopeFilter}
                onChange={(e) => setScopeFilter(e.target.value)}
                placeholder="web_search"
                className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] font-mono focus:outline-none focus:border-indigo-500"
              />
            </div>
          )}

          <div>
            <label className="block text-xs text-[var(--text-muted)] mb-1">Webhook URL (optional)</label>
            <input
              type="url"
              value={webhookURL}
              onChange={(e) => setWebhookURL(e.target.value)}
              placeholder="https://hooks.example.com/..."
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
            />
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setEnabled(!enabled)}
              className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${enabled ? "bg-indigo-600" : "bg-zinc-600"}`}
            >
              <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${enabled ? "translate-x-4" : "translate-x-1"}`} />
            </button>
            <span className="text-sm text-[var(--text-muted)]">Enabled</span>
          </div>

          {error && <p className="text-xs text-red-400">{error}</p>}

          <div className="flex gap-3 mt-2">
            <button
              type="button"
              onClick={onClose}
              className="flex-1 border border-[var(--border)] text-[var(--text-muted)] text-sm py-2 rounded-lg hover:border-indigo-500 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm font-medium py-2 rounded-lg transition-colors"
            >
              {mutation.isPending ? "Saving…" : editRule ? "Save Changes" : "Create Rule"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
