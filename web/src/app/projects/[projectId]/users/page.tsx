"use client";

import { use } from "react";
import { UsersSection } from "@/components/users/UsersSection";

export default function UsersPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = use(params);
  return (
    <div className="px-6 py-8">
      <h1 className="text-2xl font-bold text-[var(--text)] mb-6">Users</h1>
      <UsersSection projectId={projectId} />
    </div>
  );
}
