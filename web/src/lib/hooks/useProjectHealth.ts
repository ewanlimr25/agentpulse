"use client";

import { useQuery } from "@tanstack/react-query";
import { healthApi, AuthError } from "@/lib/api";
import type { ProjectHealth } from "@/lib/types";

export function useProjectHealth(
  projectId: string,
  enabled = true
): {
  health: ProjectHealth | undefined;
  isLoading: boolean;
  isReceiving: boolean;
} {
  const { data, isLoading } = useQuery({
    queryKey: ["projectHealth", projectId],
    queryFn: () => healthApi.status(projectId),
    enabled,
    refetchInterval: (query) => {
      // Stop polling once collector is confirmed receiving
      if (query.state.data?.CollectorReachable) return false;
      return 5000;
    },
    retry: (_, err) => !(err instanceof AuthError),
  });

  return {
    health: data,
    isLoading,
    isReceiving: data?.CollectorReachable ?? false,
  };
}
