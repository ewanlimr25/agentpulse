"use client";

import type { ModelInfo } from "@/lib/types";

interface ModelPickerProps {
  models: ModelInfo[];
  value: string;
  onChange: (modelId: string) => void;
}

function groupByProvider(models: ModelInfo[]): Record<string, ModelInfo[]> {
  const groups: Record<string, ModelInfo[]> = {};
  for (const m of models) {
    if (!groups[m.provider]) {
      groups[m.provider] = [];
    }
    groups[m.provider].push(m);
  }
  return groups;
}

export function ModelPicker({ models, value, onChange }: ModelPickerProps) {
  const grouped = groupByProvider(models);
  const providers = Object.keys(grouped).sort();

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-lg bg-[var(--surface-2)] text-[var(--text)] border border-[var(--border)] px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
    >
      {providers.map((provider) => (
        <optgroup key={provider} label={provider}>
          {grouped[provider].map((m) => (
            <option key={m.model_id} value={m.model_id} disabled={!m.available}>
              {m.model_id}{!m.available ? " (unavailable)" : ""}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  );
}
