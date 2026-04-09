"use client";

import { use } from "react";
import { RunList } from "@/components/runs/RunList";

export default function RunsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);

  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Runs</h1>
      <RunList projectId={projectId} />
    </div>
  );
}
