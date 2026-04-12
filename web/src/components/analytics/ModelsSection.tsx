"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "@/lib/api";
import type { AnalyticsWindow } from "@/lib/types";
import { ModelComparisonChart } from "./ModelComparisonChart";
import { ModelStatsTable } from "./ModelStatsTable";
import { ModelCostProjection } from "./ModelCostProjection";
import { ExportButton } from "@/components/export/ExportButton";

interface Props {
  projectId: string;
}

const WINDOWS: { label: string; value: AnalyticsWindow }[] = [
  { label: "24h", value: "24h" },
  { label: "7d", value: "7d" },
];

export function ModelsSection({ projectId }: Props) {
  const [window, setWindow] = useState<AnalyticsWindow>("24h");

  const { data } = useQuery({
    queryKey: ["modelStats", projectId, window],
    queryFn: () => analyticsApi.modelStats(projectId, window),
  });

  const models = data?.models ?? [];
  const pricing = data?.pricing ?? {};

  return (
    <>
      <div className="flex items-center gap-2 mb-6">
        <span className="text-xs text-[var(--text-muted)] mr-1">Window:</span>
        {WINDOWS.map((w) => (
          <button
            key={w.value}
            onClick={() => setWindow(w.value)}
            className={`px-3 py-1 text-xs rounded-lg border transition-colors ${
              window === w.value
                ? "border-indigo-500 bg-indigo-600/20 text-indigo-300"
                : "border-[var(--border)] text-[var(--text-muted)] hover:border-indigo-500"
            }`}
          >
            {w.label}
          </button>
        ))}
        <ExportButton
          projectId={projectId}
          exportType="analytics"
          params={{ window }}
        />
      </div>

      <ModelComparisonChart models={models} />

      <div className="mt-8" />

      <ModelStatsTable projectId={projectId} window={window} />

      <div className="mt-8" />

      <ModelCostProjection models={models} pricing={pricing} />
    </>
  );
}
