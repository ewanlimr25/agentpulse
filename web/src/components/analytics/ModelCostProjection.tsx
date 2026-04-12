"use client";

import { useState, useMemo } from "react";
import { formatCost } from "@/components/runs/RunRow";
import type { ModelStats, ModelPricing } from "@/lib/types";

interface Props {
  models: ModelStats[];
  pricing: ModelPricing;
}

export function ModelCostProjection({ models, pricing }: Props) {
  const [sourceId, setSourceId] = useState<string>("");
  const [targetId, setTargetId] = useState<string>("");

  const pricingKeys = useMemo(() => Object.keys(pricing).sort(), [pricing]);

  const source = models.find((m) => m.ModelID === sourceId);
  const targetPricing = pricing[targetId];

  const projectedCost = useMemo(() => {
    if (!source || !targetPricing) return null;
    return (
      (source.InputTokens * targetPricing.input_per_million) / 1_000_000 +
      (source.OutputTokens * targetPricing.output_per_million) / 1_000_000
    );
  }, [source, targetPricing]);

  const savings = source && projectedCost !== null ? source.TotalCostUSD - projectedCost : null;
  const savingsPercent =
    savings !== null && source && source.TotalCostUSD > 0
      ? (savings / source.TotalCostUSD) * 100
      : null;

  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-6">
      <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Cost Projection</h2>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
        <div>
          <label className="block text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider mb-1.5">
            Source model
          </label>
          <select
            value={sourceId}
            onChange={(e) => setSourceId(e.target.value)}
            className="w-full rounded-lg border border-[var(--border)] bg-[var(--surface-2)] text-[var(--text)] text-sm px-3 py-2 focus:outline-none focus:border-indigo-500"
          >
            <option value="">Select a model...</option>
            {models.map((m) => (
              <option key={m.ModelID} value={m.ModelID}>
                {m.ModelID}{m.Provider ? ` (${m.Provider})` : ""}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-xs font-medium text-[var(--text-muted)] uppercase tracking-wider mb-1.5">
            Target model
          </label>
          <select
            value={targetId}
            onChange={(e) => setTargetId(e.target.value)}
            className="w-full rounded-lg border border-[var(--border)] bg-[var(--surface-2)] text-[var(--text)] text-sm px-3 py-2 focus:outline-none focus:border-indigo-500"
          >
            <option value="">Select a model...</option>
            {pricingKeys.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
        </div>
      </div>

      {source && projectedCost !== null && savings !== null && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-4">
          <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3">
            <p className="text-xs text-[var(--text-muted)] uppercase tracking-wider mb-1">Current Cost</p>
            <p className="text-lg font-semibold tabular-nums text-[var(--text)]">
              {formatCost(source.TotalCostUSD)}
            </p>
          </div>

          <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3">
            <p className="text-xs text-[var(--text-muted)] uppercase tracking-wider mb-1">Projected Cost</p>
            <p className="text-lg font-semibold tabular-nums text-[var(--text)]">
              {formatCost(projectedCost)}
            </p>
          </div>

          <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3">
            <p className="text-xs text-[var(--text-muted)] uppercase tracking-wider mb-1">Savings</p>
            <p className={`text-lg font-semibold tabular-nums ${savings >= 0 ? "text-green-400" : "text-red-400"}`}>
              {savings >= 0 ? "-" : "+"}{formatCost(Math.abs(savings))}
            </p>
          </div>

          <div className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] p-3">
            <p className="text-xs text-[var(--text-muted)] uppercase tracking-wider mb-1">Savings %</p>
            <p className={`text-lg font-semibold tabular-nums ${savingsPercent !== null && savingsPercent >= 0 ? "text-green-400" : "text-red-400"}`}>
              {savingsPercent !== null ? `${savingsPercent >= 0 ? "" : "+"}${Math.abs(savingsPercent).toFixed(1)}%` : "\u2014"}
            </p>
          </div>
        </div>
      )}

      <p className="text-xs text-[var(--text-muted)] italic">
        Projection assumes equivalent token usage. Actual costs may vary.
      </p>
    </div>
  );
}
