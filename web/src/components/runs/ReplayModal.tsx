"use client";

import { useState } from "react";
import { runsApi } from "@/lib/api";

interface Props {
  runId: string;
  projectId: string;
  open: boolean;
  onClose: () => void;
}

export function ReplayModal({ runId, projectId, open, onClose }: Props) {
  const [copied, setCopied] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!open) return null;

  const cliCommand = `agentpulse replay ${runId}`;

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(cliCommand);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setError("Could not copy to clipboard");
    }
  }

  async function handleDownload() {
    setDownloading(true);
    setError(null);
    try {
      const bundle = await runsApi.replayBundle(runId, projectId);
      const blob = new Blob([JSON.stringify(bundle, null, 2)], {
        type: "application/json",
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `replay-${runId}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Download failed");
    } finally {
      setDownloading(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg mx-4 rounded-xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between mb-4">
          <div>
            <h2 className="text-lg font-semibold text-[var(--text)]">Replay this run</h2>
            <p className="text-xs text-[var(--text-muted)] mt-1">
              Reproduce this run locally with mocked LLM and tool responses.
            </p>
          </div>
          <button
            onClick={onClose}
            className="text-[var(--text-muted)] hover:text-[var(--text)] text-xl leading-none"
            aria-label="Close"
          >
            ×
          </button>
        </div>

        <div className="space-y-4">
          <div>
            <p className="text-xs uppercase tracking-wide text-[var(--text-muted)] mb-2">
              Run from CLI
            </p>
            <div className="flex items-stretch gap-2">
              <code className="flex-1 px-3 py-2 rounded-lg bg-black/40 border border-[var(--border)] text-sm font-mono text-indigo-300 overflow-x-auto">
                {cliCommand}
              </code>
              <button
                onClick={handleCopy}
                className="px-3 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-medium transition-colors shrink-0"
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>

          <div>
            <p className="text-xs uppercase tracking-wide text-[var(--text-muted)] mb-2">
              Or download the bundle
            </p>
            <button
              onClick={handleDownload}
              disabled={downloading}
              className="w-full px-4 py-2 rounded-lg border border-[var(--border)] bg-[var(--bg)] hover:border-indigo-600/60 text-sm text-[var(--text)] transition-colors disabled:opacity-50"
            >
              {downloading ? "Downloading…" : "Download bundle JSON"}
            </button>
          </div>

          {error && (
            <p className="text-xs text-red-400">{error}</p>
          )}
        </div>
      </div>
    </div>
  );
}
