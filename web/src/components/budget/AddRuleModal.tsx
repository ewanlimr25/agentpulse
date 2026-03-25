"use client";

import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { budgetApi } from "@/lib/api";
import type { BudgetRule } from "@/lib/types";

interface Props {
  projectId: string;
  isOpen: boolean;
  onClose: () => void;
}

type FormData = Omit<BudgetRule, "ID" | "ProjectID" | "CreatedAt" | "UpdatedAt">;

const inputClass =
  "w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] px-3 py-2 text-sm focus:outline-none focus:border-indigo-500";
const labelClass = "block text-xs text-[var(--text-muted)] mb-1";

export function AddRuleModal({ projectId, isOpen, onClose }: Props) {
  const queryClient = useQueryClient();

  const [name, setName] = useState("");
  const [thresholdUSD, setThresholdUSD] = useState("");
  const [action, setAction] = useState<"notify" | "halt">("notify");
  const [scope, setScope] = useState<"run" | "agent" | "user">("run");
  const [webhookURL, setWebhookURL] = useState("");

  function resetForm() {
    setName("");
    setThresholdUSD("");
    setAction("notify");
    setScope("run");
    setWebhookURL("");
  }

  function handleClose() {
    resetForm();
    onClose();
  }

  const mutation = useMutation({
    mutationFn: (data: FormData) => budgetApi.createRule(projectId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["budget-rules", projectId] });
      resetForm();
      onClose();
    },
  });

  useEffect(() => {
    if (!isOpen) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") handleClose();
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
    // handleClose is defined in component scope; isOpen guards registration
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  if (!isOpen) return null;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const parsed = parseFloat(thresholdUSD);
    if (!name.trim() || isNaN(parsed) || parsed <= 0) return;
    const data: FormData = {
      Name: name.trim(),
      ThresholdUSD: parsed,
      Action: action,
      Scope: scope,
      Enabled: true,
      ...(webhookURL.trim() ? { WebhookURL: webhookURL.trim() } : {}),
    };
    mutation.mutate(data);
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/60 z-40"
        onClick={handleClose}
        aria-hidden="true"
      />

      {/* Modal */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-rule-title"
        className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[var(--surface)] border border-[var(--border)] rounded-xl p-6"
      >
        <h2
          id="add-rule-title"
          className="text-base font-semibold text-[var(--text)] mb-5"
        >
          Add Budget Rule
        </h2>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label htmlFor="rule-name" className={labelClass}>
              Name
            </label>
            <input
              id="rule-name"
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              className={inputClass}
              placeholder="e.g. Per-run cap"
            />
          </div>

          <div>
            <label htmlFor="rule-threshold" className={labelClass}>
              Threshold USD
            </label>
            <input
              id="rule-threshold"
              type="number"
              required
              min={0.001}
              step={0.001}
              value={thresholdUSD}
              onChange={(e) => setThresholdUSD(e.target.value)}
              className={inputClass}
              placeholder="0.50"
            />
          </div>

          <div>
            <label htmlFor="rule-action" className={labelClass}>
              Action
            </label>
            <select
              id="rule-action"
              value={action}
              onChange={(e) => setAction(e.target.value as "notify" | "halt")}
              className={inputClass}
            >
              <option value="notify">notify</option>
              <option value="halt">halt</option>
            </select>
          </div>

          <div>
            <label htmlFor="rule-scope" className={labelClass}>
              Scope
            </label>
            <select
              id="rule-scope"
              value={scope}
              onChange={(e) => setScope(e.target.value as "run" | "agent" | "user")}
              className={inputClass}
            >
              <option value="run">run</option>
              <option value="agent">agent</option>
              <option value="user">user</option>
            </select>
          </div>

          <div>
            <label htmlFor="rule-webhook" className={labelClass}>
              Webhook URL (optional)
            </label>
            <input
              id="rule-webhook"
              type="text"
              value={webhookURL}
              onChange={(e) => setWebhookURL(e.target.value)}
              className={inputClass}
              placeholder="https://..."
            />
          </div>

          {mutation.error && (
            <p className="text-red-400 text-xs">
              {(mutation.error as Error).message}
            </p>
          )}

          <div className="flex gap-3 justify-end pt-1">
            <button
              type="button"
              onClick={handleClose}
              className="px-4 py-2 rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] text-sm transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white text-sm font-medium transition-colors"
            >
              {mutation.isPending ? "Saving..." : "Add Rule"}
            </button>
          </div>
        </form>
      </div>
    </>
  );
}
