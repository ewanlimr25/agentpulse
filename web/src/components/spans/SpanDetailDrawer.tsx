"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import type { Span, SpanEval, SpanEvalGroup, SpanFeedback } from "@/lib/types";
import { runsApi } from "@/lib/api";
import { SpanDetailContent } from "./SpanDetailContent";

interface Props {
  span: Span | undefined;
  evals?: SpanEval[];
  evalGroups?: SpanEvalGroup[];
  runStartTime: string;
  onClose: () => void;
  projectId: string;
  runId: string;
  feedback?: SpanFeedback | null;
  onFeedbackChange?: (feedback: SpanFeedback | null) => void;
}

export function SpanDetailDrawer({ span, evals, evalGroups, runStartTime, onClose, projectId, runId, feedback, onFeedbackChange }: Props) {
  const { data: resolvedSpan } = useQuery({
    queryKey: ["span", runId, span?.SpanID],
    queryFn: () => runsApi.fetchSpan(runId, span!.SpanID, projectId),
    enabled: !!span && !!runId && !!projectId,
    staleTime: Infinity,
  });

  const payloadKeys = ["gen_ai.prompt", "gen_ai.completion", "tool.input", "tool.output"] as const;
  const displaySpan: Span | undefined = span
    ? {
        ...span,
        Attributes: {
          ...(span.Attributes ?? {}),
          ...Object.fromEntries(
            payloadKeys
              .filter((k) => resolvedSpan?.Attributes?.[k] !== undefined)
              .map((k) => [k, resolvedSpan!.Attributes![k]])
          ),
        },
      }
    : undefined;

  // Close on ESC
  useEffect(() => {
    if (!span) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [span, onClose]);

  if (!span) return null;

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Drawer panel */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Span details"
        className="fixed top-0 right-0 h-full w-[480px] max-w-full z-50 bg-[var(--surface)] border-l border-[var(--border)] overflow-y-auto flex flex-col drawer-enter"
      >
        {/* Sticky header bar */}
        <div className="sticky top-0 flex items-center justify-between px-5 py-3 border-b border-[var(--border)] bg-[var(--surface)] z-10">
          <p className="text-sm font-semibold text-[var(--text)]">Span Detail</p>
          <button
            type="button"
            aria-label="Close drawer"
            onClick={onClose}
            className="text-[var(--text-muted)] hover:text-[var(--text)] text-xl leading-none transition-colors"
          >
            ×
          </button>
        </div>

        <SpanDetailContent span={displaySpan!} evals={evals} evalGroups={evalGroups} runStartTime={runStartTime} projectId={projectId} runId={runId} feedback={feedback} onFeedbackChange={onFeedbackChange} isResolvingPayload={!resolvedSpan && !!span} />
      </div>
    </>
  );
}
