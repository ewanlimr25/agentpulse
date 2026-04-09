"use client";

import { use } from "react";
import { AlertsSection } from "@/components/alerts/AlertsSection";

export default function AlertsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Alerts</h1>
      <AlertsSection projectId={projectId} />
    </div>
  );
}
