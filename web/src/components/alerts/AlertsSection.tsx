"use client";

import { useState } from "react";
import type { AlertRule } from "@/lib/types";
import { AlertRulesTable } from "./AlertRulesTable";
import { AlertEventsTable } from "./AlertEventsTable";
import { AddAlertRuleModal } from "./AddAlertRuleModal";
import { useAlertWebSocket } from "@/hooks/useAlertWebSocket";

// Extend the hook to also poll signal alert events for toast notifications.
// The hook's isConnected status drives the live indicator.

interface Props {
  projectId: string;
}

export function AlertsSection({ projectId }: Props) {
  const { isConnected } = useAlertWebSocket(projectId);
  const [modalOpen, setModalOpen] = useState(false);
  const [editRule, setEditRule] = useState<AlertRule | null>(null);

  function openAdd() { setEditRule(null); setModalOpen(true); }
  function openEdit(rule: AlertRule) { setEditRule(rule); setModalOpen(true); }
  function closeModal() { setModalOpen(false); setEditRule(null); }

  return (
    <>
      <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] mb-4">
        <span className={`inline-block w-2 h-2 rounded-full ${isConnected ? "bg-green-500" : "bg-zinc-500"}`} />
        {isConnected ? "Live" : "Connecting..."}
      </div>

      <AlertRulesTable projectId={projectId} onAddRule={openAdd} onEditRule={openEdit} />

      <div className="mt-8" />

      <AlertEventsTable projectId={projectId} />

      <AddAlertRuleModal
        key={editRule?.ID ?? (modalOpen ? "new" : "closed")}
        projectId={projectId}
        isOpen={modalOpen}
        editRule={editRule}
        onClose={closeModal}
      />
    </>
  );
}
