"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { loopConfigApi } from "@/lib/api";
import { getApiKey } from "@/lib/api-keys";
import { useToast } from "@/components/toast/ToastContext";
import type { LoopConfig } from "@/lib/types";

interface Props {
  projectId: string;
}

const DEFAULT_CONFIG: LoopConfig = {
  tier1_min_count: 2,
  tier2_min_count: 4,
  tier2_max_interval_ms: 3000,
};

const inputClass =
  "w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] px-3 py-2 text-sm focus:outline-none focus:border-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed";

export function LoopDetectionSection({ projectId }: Props) {
  const queryClient = useQueryClient();
  const { addToast } = useToast();

  const adminKey =
    typeof window !== "undefined"
      ? localStorage.getItem(`adminKey_${projectId}`)
      : null;
  const apiKey = getApiKey(projectId) ?? "";
  const hasAdminKey = Boolean(adminKey);

  const { data: remoteConfig, isLoading } = useQuery({
    queryKey: ["loopConfig", projectId],
    queryFn: () => loopConfigApi.get(projectId),
  });

  const [draft, setDraft] = useState<LoopConfig>(DEFAULT_CONFIG);
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    if (remoteConfig) {
      setDraft(remoteConfig);
      setIsDirty(false);
    }
  }, [remoteConfig]);

  function handleChange(field: keyof LoopConfig, rawValue: string) {
    const value = parseInt(rawValue, 10);
    if (isNaN(value)) return;
    const updated = { ...draft, [field]: value };
    setDraft(updated);
    setIsDirty(
      remoteConfig
        ? updated.tier1_min_count !== remoteConfig.tier1_min_count ||
            updated.tier2_min_count !== remoteConfig.tier2_min_count ||
            updated.tier2_max_interval_ms !== remoteConfig.tier2_max_interval_ms
        : true
    );
  }

  const mutation = useMutation({
    mutationFn: () => {
      const key = localStorage.getItem(`adminKey_${projectId}`) ?? "";
      return loopConfigApi.put(projectId, draft, apiKey, key);
    },
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: ["loopConfig", projectId] });
      setDraft(saved);
      setIsDirty(false);
      addToast({ title: "Saved", message: "Loop detection thresholds updated.", variant: "info" });
    },
    onError: (err: Error) => {
      addToast({ title: "Save failed", message: err.message, variant: "halt" });
    },
  });

  if (isLoading) {
    return <p className="text-sm text-[var(--text-muted)] py-6">Loading loop detection config...</p>;
  }

  return (
    <div className="flex flex-col gap-8">
      {/* No admin key notice */}
      {!hasAdminKey && (
        <div className="border border-amber-700/50 bg-amber-950/20 rounded-xl px-4 py-3 text-sm text-amber-300/90">
          Saving changes requires your Admin Key. Your Admin Key was shown once when this project was created.
        </div>
      )}

      <div>
        <p className="text-sm font-semibold text-[var(--text)] mb-1">Loop Detection Thresholds</p>
        <p className="text-xs text-[var(--text-muted)] mb-5">
          Configure how aggressively the loop detector flags repeated tool and LLM calls.
          Changes apply to the next detection cycle (runs approximately every 60 seconds).
        </p>

        <div className="flex flex-col gap-5">
          {/* Tier 1 */}
          <div className="border border-[var(--border)] rounded-xl p-4 bg-[var(--surface)] flex flex-col gap-2">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-xs font-semibold text-green-400 uppercase tracking-wider">Tier 1</span>
              <span className="text-xs text-[var(--text-muted)]">High confidence — same input and output</span>
            </div>
            <label className="block text-xs text-[var(--text-muted)] mb-1">
              Minimum repetitions
            </label>
            <input
              type="number"
              min={1}
              value={draft.tier1_min_count}
              placeholder={String(DEFAULT_CONFIG.tier1_min_count)}
              disabled={!hasAdminKey || mutation.isPending}
              onChange={(e) => handleChange("tier1_min_count", e.target.value)}
              className={inputClass}
            />
            <p className="text-xs text-[var(--text-muted)]">
              A span is flagged when the same (name, input, output) tuple appears at least this many times in a run.
              Default: <span className="font-mono">2</span>.
            </p>
          </div>

          {/* Tier 2 */}
          <div className="border border-[var(--border)] rounded-xl p-4 bg-[var(--surface)] flex flex-col gap-2">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-xs font-semibold text-amber-400 uppercase tracking-wider">Tier 2</span>
              <span className="text-xs text-[var(--text-muted)]">Low confidence — same input, fast interval</span>
            </div>
            <div className="flex flex-col gap-3">
              <div>
                <label className="block text-xs text-[var(--text-muted)] mb-1">Minimum repetitions</label>
                <input
                  type="number"
                  min={1}
                  value={draft.tier2_min_count}
                  placeholder={String(DEFAULT_CONFIG.tier2_min_count)}
                  disabled={!hasAdminKey || mutation.isPending}
                  onChange={(e) => handleChange("tier2_min_count", e.target.value)}
                  className={inputClass}
                />
                <p className="text-xs text-[var(--text-muted)] mt-1">
                  Minimum occurrences of the same (name, input) pair to trigger Tier 2 detection.
                  Default: <span className="font-mono">4</span>.
                </p>
              </div>
              <div>
                <label className="block text-xs text-[var(--text-muted)] mb-1">Max average interval (ms)</label>
                <input
                  type="number"
                  min={1}
                  value={draft.tier2_max_interval_ms}
                  placeholder={String(DEFAULT_CONFIG.tier2_max_interval_ms)}
                  disabled={!hasAdminKey || mutation.isPending}
                  onChange={(e) => handleChange("tier2_max_interval_ms", e.target.value)}
                  className={inputClass}
                />
                <p className="text-xs text-[var(--text-muted)] mt-1">
                  A loop is only flagged if the average time between repeated calls is below this threshold.
                  Default: <span className="font-mono">3000</span> ms.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <button
          onClick={() => mutation.mutate()}
          disabled={!hasAdminKey || !isDirty || mutation.isPending}
          className="px-4 py-2 text-sm font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {mutation.isPending ? "Saving..." : "Save Changes"}
        </button>
      </div>
    </div>
  );
}
