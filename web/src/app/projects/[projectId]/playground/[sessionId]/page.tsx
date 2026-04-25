"use client";

import { use, useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import { playgroundApi, modelsApi } from "@/lib/api";
import type {
  PlaygroundSession,
  PlaygroundVariant,
  PlaygroundExecution,
  ModelInfo,
} from "@/lib/types";
import { PlaygroundEditor } from "@/components/playground/PlaygroundEditor";
import { PlaygroundResult } from "@/components/playground/PlaygroundResult";
import { ABStatsDisplay } from "@/components/playground/ABStatsDisplay";

export default function PlaygroundSessionPage({
  params,
}: {
  params: Promise<{ projectId: string; sessionId: string }>;
}) {
  const { projectId, sessionId } = use(params);
  const router = useRouter();

  const [session, setSession] = useState<PlaygroundSession | null>(null);
  const [variants, setVariants] = useState<PlaygroundVariant[]>([]);
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [runningVariants, setRunningVariants] = useState<Set<string>>(
    new Set()
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    try {
      setLoading(true);
      const [sess, modelList] = await Promise.all([
        playgroundApi.getSession(projectId, sessionId),
        modelsApi.list(),
      ]);
      setSession(sess);
      setVariants(sess.Variants ?? []);
      setModels(modelList);
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to load session";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [projectId, sessionId]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  function handleUpdate(variantId: string, changes: Partial<PlaygroundVariant>) {
    setVariants((prev) =>
      prev.map((v) => (v.ID === variantId ? { ...v, ...changes } : v))
    );
  }

  async function handleRun(variantId: string) {
    const variant = variants.find((v) => v.ID === variantId);
    if (!variant) return;

    setRunningVariants((prev) => new Set([...prev, variantId]));
    setError(null);

    try {
      // Persist current local state before running
      await playgroundApi.updateVariant(projectId, sessionId, variantId, {
        label: variant.Label,
        model_id: variant.ModelID,
        system: variant.System,
        messages: variant.Messages,
        temperature: variant.Temperature,
        max_tokens: variant.MaxTokens,
      });

      // Execute the variant
      const execution = await playgroundApi.runVariant(
        projectId,
        sessionId,
        variantId
      );

      // Append execution to local state
      setVariants((prev) =>
        prev.map((v) => {
          if (v.ID !== variantId) return v;
          const executions = v.Executions ?? [];
          return { ...v, Executions: [...executions, execution] };
        })
      );
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Failed to run variant";
      setError(message);
    } finally {
      setRunningVariants((prev) => {
        const next = new Set(prev);
        next.delete(variantId);
        return next;
      });
    }
  }

  function handleDuplicate() {
    if (variants.length < 2) return;
    const sourceVariant = variants[0];
    const targetVariant = variants[1];

    const duplicated: PlaygroundVariant = {
      ...targetVariant,
      ModelID: sourceVariant.ModelID,
      System: sourceVariant.System,
      Messages: sourceVariant.Messages.map((m) => ({ ...m })),
      Temperature: sourceVariant.Temperature,
      MaxTokens: sourceVariant.MaxTokens,
    };

    setVariants((prev) =>
      prev.map((v) => (v.ID === targetVariant.ID ? duplicated : v))
    );
  }

  function getLatestExecution(
    variant: PlaygroundVariant
  ): PlaygroundExecution | null {
    const executions = variant.Executions ?? [];
    if (executions.length === 0) return null;
    return executions[executions.length - 1];
  }

  if (loading) {
    return (
      <div className="px-6 py-8">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-48 rounded bg-[var(--surface-2)]" />
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="h-96 rounded-lg bg-[var(--surface-2)]" />
            <div className="h-96 rounded-lg bg-[var(--surface-2)]" />
          </div>
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="px-6 py-8">
        <p className="text-sm text-red-400">
          {error ?? "Session not found."}
        </p>
      </div>
    );
  }

  return (
    <div className="px-6 py-8">
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <button
          type="button"
          onClick={() => router.push(`/projects/${projectId}/playground`)}
          className="text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
        >
          &larr; Sessions
        </button>
        <h1 className="text-2xl font-bold text-[var(--text)]">
          {session.Name}
        </h1>
        {session.SourceRunID && (
          <span className="inline-flex items-center rounded-full bg-indigo-500/20 text-indigo-300 text-xs px-2 py-0.5">
            Seeded from run
          </span>
        )}
      </div>

      {error && (
        <div className="rounded-lg bg-red-950/40 border border-red-700 p-3 mb-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {/* Duplicate button */}
      {variants.length === 2 && (
        <div className="flex justify-center mb-4">
          <button
            type="button"
            onClick={handleDuplicate}
            className="text-xs text-blue-400 hover:text-blue-300 border border-[var(--border)] rounded-lg px-3 py-1.5 transition-colors"
          >
            Duplicate A &rarr; B
          </button>
        </div>
      )}

      {/* Two-column variant grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {variants.map((variant) => {
          const isRunning = runningVariants.has(variant.ID);
          return (
            <div
              key={variant.ID}
              className="rounded-lg border border-[var(--border)] bg-[var(--surface)] p-4 space-y-4"
            >
              <PlaygroundEditor
                variant={variant}
                models={models}
                onUpdate={(changes) => handleUpdate(variant.ID, changes)}
                onRun={() => handleRun(variant.ID)}
                isRunning={isRunning}
              />
              <div className="border-t border-[var(--border)] pt-4">
                <h3 className="text-xs text-[var(--text-muted)] mb-2 font-medium uppercase tracking-wide">
                  Result
                </h3>
                <PlaygroundResult
                  execution={getLatestExecution(variant)}
                  isRunning={isRunning}
                />
              </div>
            </div>
          );
        })}
      </div>

      {/* A/B statistical significance — only shown for two-variant sessions */}
      {variants.length === 2 && (
        <ABStatsDisplay variantA={variants[0]} variantB={variants[1]} />
      )}
    </div>
  );
}
