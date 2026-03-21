"use client";

import { useEffect, useState } from "react";
import type { Span } from "@/lib/types";
import { SpanDetailContent } from "./SpanDetailContent";

interface Props {
  span: Span | undefined;
  runStartTime: string;
  onClose: () => void;
}

export function SpanDetailDrawer({ span, runStartTime, onClose }: Props) {
  const [isExiting, setIsExiting] = useState(false);

  // Reset exit state when a new span is selected
  useEffect(() => {
    if (span) setIsExiting(false);
  }, [span?.SpanID]);

  // Close on ESC
  useEffect(() => {
    if (!span) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") handleClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [span]);

  if (!span) return null;

  function handleClose() {
    setIsExiting(true);
  }

  function handleAnimationEnd() {
    if (isExiting) onClose();
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={handleClose}
        aria-hidden="true"
      />

      {/* Drawer panel */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Span details"
        className={`fixed top-0 right-0 h-full w-[480px] max-w-full z-50 bg-[var(--surface)] border-l border-[var(--border)] overflow-y-auto flex flex-col ${isExiting ? "drawer-exit" : "drawer-enter"}`}
        onAnimationEnd={handleAnimationEnd}
      >
        {/* Sticky header bar */}
        <div className="sticky top-0 flex items-center justify-between px-5 py-3 border-b border-[var(--border)] bg-[var(--surface)] z-10">
          <p className="text-sm font-semibold text-[var(--text)]">Span Detail</p>
          <button
            type="button"
            aria-label="Close drawer"
            onClick={handleClose}
            className="text-[var(--text-muted)] hover:text-[var(--text)] text-xl leading-none transition-colors"
          >
            ×
          </button>
        </div>

        <SpanDetailContent span={span} runStartTime={runStartTime} />
      </div>
    </>
  );
}
