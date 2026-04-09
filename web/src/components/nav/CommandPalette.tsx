"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Command } from "cmdk";
import { Search, X } from "lucide-react";
import { searchApi } from "@/lib/api";
import { SpanKindBadge } from "@/components/spans/SpanKindBadge";
import type { AgentSpanKind } from "@/lib/types";

interface Props {
  open: boolean;
  onClose: () => void;
  projectId: string;
}

const FIELD_LABELS: Record<string, string> = {
  "gen_ai.prompt": "prompt",
  "gen_ai.completion": "completion",
  "tool.input": "tool input",
  "tool.output": "tool output",
};

export function CommandPalette({ open, onClose, projectId }: Props) {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");

  // Focus input when opened
  useEffect(() => {
    if (open) {
      setTimeout(() => inputRef.current?.focus(), 0);
    } else {
      setQuery("");
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    function handler(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const { data, isFetching } = useQuery({
    queryKey: ["cmdpalette", projectId, query],
    queryFn: () => searchApi.search(projectId, { q: query, limit: 15 }),
    enabled: query.length >= 3,
    staleTime: 10_000,
  });

  const results = data?.results ?? [];
  const showEmpty = query.length >= 3 && !isFetching && results.length === 0;

  function navigate(runId: string) {
    router.push(`/projects/${projectId}/runs/${runId}`);
    onClose();
  }

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh] px-4"
      onClick={onClose}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/60" />

      {/* Palette */}
      <div
        className="relative w-full max-w-xl bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-2xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <Command shouldFilter={false} loop>
          {/* Input row */}
          <div className="flex items-center gap-2.5 px-4 py-3 border-b border-[var(--border)]">
            <Search className="w-4 h-4 shrink-0 text-[var(--text-muted)]" />
            <Command.Input
              ref={inputRef}
              value={query}
              onValueChange={setQuery}
              placeholder="Search spans, runs, sessions…"
              className="flex-1 bg-transparent text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] outline-none"
            />
            {isFetching && (
              <span className="text-xs text-[var(--text-muted)] animate-pulse">Searching…</span>
            )}
            <button onClick={onClose} className="text-[var(--text-muted)] hover:text-[var(--text)] transition-colors">
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Results */}
          <Command.List className="max-h-96 overflow-y-auto py-2">
            {query.length < 3 && (
              <Command.Empty>
                <p className="px-4 py-8 text-sm text-center text-[var(--text-muted)]">
                  Type at least 3 characters to search
                </p>
              </Command.Empty>
            )}

            {showEmpty && (
              <Command.Empty>
                <p className="px-4 py-8 text-sm text-center text-[var(--text-muted)]">
                  No results for <span className="font-medium text-[var(--text)]">&ldquo;{query}&rdquo;</span>
                </p>
              </Command.Empty>
            )}

            {results.length > 0 && (
              <Command.Group
                heading={
                  <span className="px-4 py-1.5 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
                    Spans — {data?.total ?? 0} total
                  </span>
                }
              >
                {results.map((result) => (
                  <Command.Item
                    key={result.SpanID + result.MatchedField}
                    value={result.SpanID}
                    onSelect={() => navigate(result.RunID)}
                    className="flex items-start gap-3 px-4 py-3 cursor-pointer aria-selected:bg-[var(--surface-2)] transition-colors"
                  >
                    <div className="pt-0.5 shrink-0">
                      <SpanKindBadge kind={result.AgentSpanKind as AgentSpanKind} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-[var(--text)] truncate">
                        {result.SpanName}
                      </p>
                      {result.Snippet && (
                        <p className="text-xs text-[var(--text-muted)] truncate mt-0.5">
                          <span className="text-indigo-400">
                            {FIELD_LABELS[result.MatchedField] ?? result.MatchedField}:
                          </span>{" "}
                          {result.Snippet}
                        </p>
                      )}
                    </div>
                    <span className="shrink-0 text-xs font-mono text-[var(--text-muted)] self-center">
                      {result.RunID.slice(0, 8)}…
                    </span>
                  </Command.Item>
                ))}
              </Command.Group>
            )}
          </Command.List>

          {/* Footer */}
          <div className="border-t border-[var(--border)] px-4 py-2 flex items-center gap-4 text-xs text-[var(--text-muted)]">
            <span><kbd className="font-mono bg-[var(--surface-2)] border border-[var(--border)] px-1 rounded">↑↓</kbd> navigate</span>
            <span><kbd className="font-mono bg-[var(--surface-2)] border border-[var(--border)] px-1 rounded">↵</kbd> open run</span>
            <span><kbd className="font-mono bg-[var(--surface-2)] border border-[var(--border)] px-1 rounded">esc</kbd> close</span>
          </div>
        </Command>
      </div>
    </div>
  );
}
