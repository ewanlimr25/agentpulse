"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getApiKey } from "@/lib/api-keys";
import { useToast } from "@/components/toast/ToastContext";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

type ExportFormat = "csv" | "jsonl";

interface ExportButtonProps {
  projectId: string;
  exportType: "spans" | "runs" | "analytics";
  /** Additional query params to append (e.g., window=7d, agent_name=foo) */
  params?: Record<string, string>;
}

export function ExportButton({ projectId, exportType, params }: ExportButtonProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const { addToast } = useToast();

  // Close dropdown on click outside
  useEffect(() => {
    if (!open) return;

    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  const handleExport = useCallback(
    async (format: ExportFormat) => {
      setOpen(false);

      const apiKey = getApiKey(projectId);
      const qp = new URLSearchParams({ format, ...params });
      const url = `${BASE_URL}/api/v1/projects/${projectId}/export/${exportType}?${qp}`;

      addToast({
        title: "Export started",
        message: `Preparing ${format.toUpperCase()} download...`,
        variant: "info",
      });

      try {
        const res = await fetch(url, {
          headers: apiKey ? { Authorization: `Bearer ${apiKey}` } : {},
        });

        if (!res.ok) {
          const body = await res.json().catch(() => null);
          const message = body?.error ?? `Export failed (HTTP ${res.status})`;
          addToast({ title: "Export failed", message, variant: "halt" });
          return;
        }

        const blob = await res.blob();
        const ext = format === "csv" ? "csv" : "jsonl";
        const filename = `${exportType}_${new Date().toISOString().slice(0, 10)}.${ext}`;
        const a = document.createElement("a");
        a.href = URL.createObjectURL(blob);
        a.download = filename;
        a.click();
        URL.revokeObjectURL(a.href);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : "Unexpected error";
        addToast({ title: "Export failed", message, variant: "halt" });
      }
    },
    [projectId, exportType, params, addToast],
  );

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-[var(--border)] text-xs font-medium text-[var(--text-muted)] hover:border-indigo-600 hover:text-[var(--text)] transition-colors"
      >
        Export
        <svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          className={`transition-transform ${open ? "rotate-180" : ""}`}
        >
          <path
            d="M3 4.5L6 7.5L9 4.5"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[120px] rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] shadow-lg overflow-hidden">
          <button
            type="button"
            onClick={() => handleExport("csv")}
            className="w-full text-left px-3 py-2 text-xs text-[var(--text-muted)] hover:bg-[var(--border)] hover:text-[var(--text)] transition-colors"
          >
            CSV
          </button>
          <button
            type="button"
            onClick={() => handleExport("jsonl")}
            className="w-full text-left px-3 py-2 text-xs text-[var(--text-muted)] hover:bg-[var(--border)] hover:text-[var(--text)] transition-colors"
          >
            JSON Lines
          </button>
        </div>
      )}
    </div>
  );
}
