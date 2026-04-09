"use client";

import { use } from "react";
import { SessionsSection } from "@/components/sessions/SessionsSection";

export default function SessionsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Sessions</h1>
      <SessionsSection projectId={projectId} />
    </div>
  );
}
