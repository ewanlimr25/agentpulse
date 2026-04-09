"use client";

import { use } from "react";
import { SettingsSection } from "@/components/settings/SettingsSection";

export default function SettingsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Settings</h1>
      <SettingsSection projectId={projectId} />
    </div>
  );
}
