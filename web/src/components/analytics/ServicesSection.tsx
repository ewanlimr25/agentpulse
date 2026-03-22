"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { analyticsApi } from "@/lib/api";
import type { AnalyticsWindow } from "@/lib/types";
import { ToolAnalyticsTable } from "./ToolAnalyticsTable";
import { AgentCostTable } from "./AgentCostTable";
import { TopToolsCharts } from "./TopToolsCharts";

interface Props {
  projectId: string;
}

const WINDOWS: { label: string; value: AnalyticsWindow }[] = [
  { label: "24h", value: "24h" },
  { label: "7d", value: "7d" },
];

export function ServicesSection({ projectId }: Props) {
  const [window, setWindow] = useState<AnalyticsWindow>("24h");

  // Pre-fetch tool stats here so TopToolsCharts can share the cache without an extra request.
  const { data } = useQuery({
    queryKey: ["toolStats", projectId, window],
    queryFn: () => analyticsApi.toolStats(projectId, window),
  });

  const tools = data?.tools ?? [];

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
      </div>

      <TopToolsCharts tools={tools} />

      <ToolAnalyticsTable projectId={projectId} window={window} />

      <div className="mt-8" />

      <AgentCostTable projectId={projectId} window={window} />
    </>
  );
}
