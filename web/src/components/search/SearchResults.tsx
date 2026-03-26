"use client";

import React from "react";
import Link from "next/link";
import type { SearchResult } from "@/lib/types";
import { SpanKindBadge } from "@/components/spans/SpanKindBadge";
import { formatCost } from "@/components/runs/RunRow";

// Labels for the matched field shown on each result card.
const FIELD_LABELS: Record<string, string> = {
  "gen_ai.prompt": "prompt",
  "gen_ai.completion": "completion",
  "tool.input": "tool input",
  "tool.output": "tool output",
};

// highlightSnippet renders text with the matching query term highlighted.
// Does NOT use dangerouslySetInnerHTML — safe JSX only.
function highlightSnippet(snippet: string, query: string): React.ReactNode {
  const lowerSnippet = snippet.toLowerCase();
  const lowerQuery = query.toLowerCase();
  const pos = lowerSnippet.indexOf(lowerQuery);
  if (pos < 0) return <>{snippet}</>;
  return (
    <>
      {snippet.slice(0, pos)}
      <mark className="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5 not-italic font-medium text-inherit">
        {snippet.slice(pos, pos + query.length)}
      </mark>
      {snippet.slice(pos + query.length)}
    </>
  );
}

// SkeletonCard is a placeholder shown while the first search is loading.
function SkeletonCard() {
  return (
    <div className="border border-[var(--border)] bg-[var(--surface)] rounded-xl px-5 py-4 animate-pulse">
      <div className="flex items-center gap-3 mb-3">
        <div className="h-5 w-20 bg-[var(--border)] rounded" />
        <div className="h-5 w-16 bg-[var(--border)] rounded" />
      </div>
      <div className="h-4 w-full bg-[var(--border)] rounded mb-2" />
      <div className="h-4 w-3/4 bg-[var(--border)] rounded" />
    </div>
  );
}

interface Props {
  results: SearchResult[];
  total: number;
  query: string;
  projectId: string;
  isLoading: boolean;
  onLoadMore: () => void;
  hasMore: boolean;
}

export function SearchResults({ results, total, query, projectId, isLoading, onLoadMore, hasMore }: Props) {
  // Show skeleton when loading with no results yet.
  if (isLoading && results.length === 0) {
    return (
      <div className="space-y-3">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
    );
  }

  if (!isLoading && results.length === 0) {
    return (
      <div className="border border-[var(--border)] bg-[var(--surface)] rounded-xl px-5 py-10 text-center">
        <p className="text-[var(--text-muted)] text-sm">
          No results for <span className="font-medium text-[var(--text)]">&ldquo;{query}&rdquo;</span>
        </p>
      </div>
    );
  }

  return (
    <div>
      {/* Results header */}
      <p className="text-sm text-[var(--text-muted)] mb-3">
        <span className="font-medium text-[var(--text)]">{total.toLocaleString()}</span> result{total !== 1 ? "s" : ""} for{" "}
        <span className="font-medium text-[var(--text)]">&ldquo;{query}&rdquo;</span>
      </p>

      <div className="space-y-3">
        {results.map((result) => (
          <Link
            key={`${result.SpanID}-${result.MatchedField}`}
            href={`/projects/${projectId}/runs/${result.RunID}`}
            className="block border border-[var(--border)] bg-[var(--surface)] rounded-xl px-5 py-4 hover:border-indigo-600 transition-colors"
          >
            {/* Header row: span name + kind badge + field tag */}
            <div className="flex items-center gap-2 mb-2 flex-wrap">
              <span className="text-sm font-medium text-[var(--text)] truncate max-w-xs">
                {result.SpanName}
              </span>
              <SpanKindBadge kind={result.AgentSpanKind} />
              {result.MatchedField && (
                <span className="text-xs px-2 py-0.5 rounded border border-amber-700 bg-amber-950/30 text-amber-400">
                  {FIELD_LABELS[result.MatchedField] ?? result.MatchedField}
                </span>
              )}
              {result.AgentName && (
                <span className="text-xs text-[var(--text-muted)]">{result.AgentName}</span>
              )}
            </div>

            {/* Snippet */}
            {result.Snippet && (
              <p className="text-sm text-[var(--text-muted)] font-mono leading-relaxed mb-2 line-clamp-3">
                {highlightSnippet(result.Snippet, query)}
              </p>
            )}

            {/* Footer: timestamp, cost, tokens */}
            <div className="flex items-center gap-4 text-xs text-[var(--text-muted)]">
              <span>{new Date(result.StartTime).toLocaleString()}</span>
              {result.CostUSD > 0 && (
                <span className="text-indigo-400">{formatCost(result.CostUSD)}</span>
              )}
              {result.TotalTokens > 0 && (
                <span>{result.TotalTokens.toLocaleString()} tok</span>
              )}
            </div>
          </Link>
        ))}
      </div>

      {/* Load more */}
      {hasMore && (
        <div className="mt-4 text-center">
          <button
            onClick={onLoadMore}
            disabled={isLoading}
            className="px-4 py-2 text-sm bg-[var(--surface)] border border-[var(--border)] rounded-lg text-[var(--text)] hover:border-indigo-500 transition-colors disabled:opacity-50"
          >
            {isLoading ? "Loading…" : "Load more"}
          </button>
        </div>
      )}
    </div>
  );
}
