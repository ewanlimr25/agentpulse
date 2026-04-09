"use client";

import { use } from "react";
import { BudgetSection } from "@/components/budget/BudgetSection";

export default function BudgetPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Budget</h1>
      <BudgetSection projectId={projectId} />
    </div>
  );
}
