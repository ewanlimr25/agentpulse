"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { evalsApi, AuthError } from "@/lib/api";
import type { RunEvalSummary } from "@/lib/types";
import type { Run } from "@/lib/types";
import { toQualitySeries } from "@/lib/chart-utils";
import { EvalHealthCards } from "./EvalHealthCards";
import { EvalTrendChart } from "./EvalTrendChart";
import { EvalConfigTable } from "./EvalConfigTable";
import { AddEvalConfigModal } from "./AddEvalConfigModal";

interface Props {
  projectId: string;
  runs: Run[];
  evalSummaries: RunEvalSummary[];
}

export function EvalsSection({ projectId, runs, evalSummaries }: Props) {
  const [showAddModal, setShowAddModal] = useState(false);

  const { data: configs = [] } = useQuery({
    queryKey: ["evalConfigs", projectId],
    queryFn: () => evalsApi.listConfigs(projectId),
    retry: (_, err) => !(err instanceof AuthError),
  });

  const qualitySeries = toQualitySeries(runs, evalSummaries);

  return (
    <div className="flex flex-col gap-8">
      <EvalHealthCards summaries={evalSummaries} />

      <div>
        <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Eval Trends</h2>
        <div style={{ height: 200 }}>
          <EvalTrendChart data={qualitySeries} defaultShowAll />
        </div>
      </div>

      <EvalConfigTable
        projectId={projectId}
        configs={configs}
        onAddCustom={() => setShowAddModal(true)}
      />

      {showAddModal && (
        <AddEvalConfigModal projectId={projectId} onClose={() => setShowAddModal(false)} />
      )}
    </div>
  );
}
