"use client";

import { useState } from "react";
import { useAlertWebSocket } from "@/hooks/useAlertWebSocket";
import { BudgetRulesTable } from "@/components/budget/BudgetRulesTable";
import { AlertHistoryTable } from "@/components/budget/AlertHistoryTable";
import { AddRuleModal } from "@/components/budget/AddRuleModal";

interface Props {
  projectId: string;
}

export function BudgetSection({ projectId }: Props) {
  const { isConnected } = useAlertWebSocket(projectId);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);

  return (
    <>
      <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] mb-4">
        <span
          className={`inline-block w-2 h-2 rounded-full ${
            isConnected ? "bg-green-500" : "bg-zinc-500"
          }`}
        />
        {isConnected ? "Live" : "Connecting..."}
      </div>

      <BudgetRulesTable
        projectId={projectId}
        onAddRule={() => setIsAddModalOpen(true)}
      />

      <div className="mt-8" />

      <AlertHistoryTable projectId={projectId} />

      <AddRuleModal
        projectId={projectId}
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
      />
    </>
  );
}
