"use client";

import { useQuery } from "@tanstack/react-query";
import { usersApi } from "@/lib/api";
import { UserCostTable } from "./UserCostTable";

interface Props {
  projectId: string;
}

export function UsersSection({ projectId }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: ["users", projectId],
    queryFn: () => usersApi.list(projectId),
  });

  const users = data?.users ?? [];

  return (
    <div>
      <h2 className="text-lg font-semibold text-[var(--text)] mb-3">User Cost Attribution</h2>
      {isLoading ? (
        <div className="border border-[var(--border)] rounded-xl px-6 py-8 text-center text-sm text-[var(--text-muted)]">
          Loading…
        </div>
      ) : (
        <UserCostTable users={users} />
      )}
    </div>
  );
}
