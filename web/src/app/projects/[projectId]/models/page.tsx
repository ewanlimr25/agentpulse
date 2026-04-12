"use client";

import { use } from "react";
import { ModelsSection } from "@/components/analytics/ModelsSection";

export default function ModelsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Models</h1>
      <ModelsSection projectId={projectId} />
    </div>
  );
}
