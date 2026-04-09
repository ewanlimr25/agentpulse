"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { projectsApi, AuthError } from "@/lib/api";
import { saveApiKey, removeApiKey } from "@/lib/api-keys";

export interface ProjectAuthState {
  gated: boolean;
  isValidating: boolean;
  keyError: string;
  submitKey: (key: string) => Promise<void>;
}

export function useProjectAuth(projectId: string): ProjectAuthState {
  const queryClient = useQueryClient();
  const [keyError, setKeyError] = useState("");
  // isValidating prevents the flash when the project query briefly transitions
  // through "pending" state (error → pending → error/success) during key submission.
  const [isValidating, setIsValidating] = useState(false);

  const { error: projectError } = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => projectsApi.get(projectId),
    retry: (_, err) => !(err instanceof AuthError),
  });

  const isAuthError = projectError instanceof AuthError;
  // Show the gate whenever we have an auth error OR we're in the middle of validating.
  const gated = isAuthError || isValidating;

  async function submitKey(key: string): Promise<void> {
    setKeyError("");
    setIsValidating(true);
    saveApiKey(projectId, key);
    try {
      await queryClient.fetchQuery({
        queryKey: ["project", projectId],
        queryFn: () => projectsApi.get(projectId),
        retry: false,
      });
      // Success — clear any cached 401 errors from dependent queries.
      queryClient.removeQueries({ queryKey: ["runs", projectId] });
      queryClient.removeQueries({ queryKey: ["evalSummaries", projectId] });
    } catch {
      removeApiKey(projectId);
      setKeyError("Invalid API key — please check and try again.");
    } finally {
      setIsValidating(false);
    }
  }

  return { gated, isValidating, keyError, submitKey };
}
