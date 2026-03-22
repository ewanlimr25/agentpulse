"use client";

import { useEffect } from "react";
import type { Span, SpanEval } from "@/lib/types";
import { SpanDetailContent } from "./SpanDetailContent";

interface Props {
  span: Span | undefined;
  evals?: SpanEval[];
  runStartTime: string;
  onClose: () => void;
}

export function SpanDetailDrawer({ span, evals, runStartTime, onClose }: Props) {
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

        <SpanDetailContent span={span} evals={evals} runStartTime={runStartTime} />
      </div>
    </>
  );
}
