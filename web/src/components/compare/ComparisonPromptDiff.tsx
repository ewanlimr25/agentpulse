"use client";

import { useState } from "react";
import { diffLines } from "diff";
import type { RunPromptDiff, SpanPromptDiff, PromptFieldDiff, ModelParamDiff } from "@/lib/types";

// ── Status badge ─────────────────────────────────────────────────────────────

const STATUS_BADGE: Record<SpanPromptDiff["status"], { label: string; className: string }> = {
  changed: {
    label: "changed",
    className: "bg-amber-900/40 text-amber-300 border border-amber-700/50",
  },
  "only-a": {
    label: "only in A",
    className: "bg-red-900/40 text-red-300 border border-red-700/50",
  },
  "only-b": {
    label: "only in B",
    className: "bg-green-900/40 text-green-300 border border-green-700/50",
  },
  unchanged: {
    label: "unchanged",
    className: "bg-zinc-800 text-zinc-400 border border-zinc-700",
  },
};

// ── Model params table ────────────────────────────────────────────────────────

interface ParamsTableProps {
  params: ModelParamDiff[];
}

function ParamsTable({ params }: ParamsTableProps) {
  if (params.length === 0) return null;

  return (
    <div className="mb-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)] mb-2">
        Model Parameters
      </p>
      <div className="rounded-lg border border-[var(--border)] overflow-hidden">
        <table className="w-full text-xs font-mono">
          <thead>
            <tr className="border-b border-[var(--border)] bg-[var(--bg)]">
              <th className="px-3 py-2 text-left text-[var(--text-muted)] font-semibold uppercase tracking-wide">
                Param
              </th>
              <th className="px-3 py-2 text-left text-blue-400 font-semibold uppercase tracking-wide">
                Run A
              </th>
              <th className="px-3 py-2 text-left text-orange-400 font-semibold uppercase tracking-wide">
                Run B
              </th>
            </tr>
          </thead>
          <tbody>
            {params.map((p) => (
              <tr
                key={p.param_name}
                className={`border-b border-[var(--border)] last:border-0 ${
                  p.changed ? "bg-amber-950/20" : ""
                }`}
              >
                <td className="px-3 py-2 text-[var(--text-muted)]">{p.param_name}</td>
                <td className="px-3 py-2 text-blue-300">
                  {p.a === "" ? (
                    <span className="text-zinc-600 italic" title="not reported by this SDK">
                      —
                    </span>
                  ) : (
                    p.a
                  )}
                </td>
                <td className="px-3 py-2 text-orange-300">
                  {p.b === "" ? (
                    <span className="text-zinc-600 italic" title="not reported by this SDK">
                      —
                    </span>
                  ) : (
                    p.b
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── Prompt field diff ─────────────────────────────────────────────────────────

const LARGE_PROMPT_THRESHOLD = 50_000;

interface FieldDiffViewProps {
  diff: PromptFieldDiff;
}

function FieldDiffView({ diff }: FieldDiffViewProps) {
  const isLarge = diff.a.length > LARGE_PROMPT_THRESHOLD || diff.b.length > LARGE_PROMPT_THRESHOLD;

  if (isLarge) {
    return (
      <div className="mb-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)] mb-2">
          {diff.field_name}
        </p>
        <p className="text-xs text-amber-400 mb-2 italic">
          Prompt too large for line diff — showing raw text
        </p>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <p className="text-xs text-blue-400 mb-1 font-semibold uppercase tracking-wide">Run A</p>
            <pre className="text-xs font-mono bg-[var(--bg)] border border-[var(--border)] rounded-lg p-3 overflow-x-auto whitespace-pre-wrap break-words text-[var(--text-muted)] max-h-64 overflow-y-auto">
              {diff.a || <span className="italic text-zinc-600">empty</span>}
            </pre>
          </div>
          <div>
            <p className="text-xs text-orange-400 mb-1 font-semibold uppercase tracking-wide">Run B</p>
            <pre className="text-xs font-mono bg-[var(--bg)] border border-[var(--border)] rounded-lg p-3 overflow-x-auto whitespace-pre-wrap break-words text-[var(--text-muted)] max-h-64 overflow-y-auto">
              {diff.b || <span className="italic text-zinc-600">empty</span>}
            </pre>
          </div>
        </div>
      </div>
    );
  }

  const hunks = diffLines(diff.a, diff.b);

  return (
    <div className="mb-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)] mb-2">
        {diff.field_name}
      </p>
      <pre className="text-xs font-mono rounded-lg border border-[var(--border)] overflow-x-auto">
        {hunks.map((hunk, idx) => {
          const lines = hunk.value.split("\n");
          // diffLines may include a trailing empty string from the split
          const trimmed = lines[lines.length - 1] === "" ? lines.slice(0, -1) : lines;

          if (hunk.added) {
            return trimmed.map((line, li) => (
              <div key={`${idx}-${li}`} className="bg-green-950/40 text-green-300 px-3 py-0.5 leading-5">
                <span className="select-none text-green-600 mr-2">+</span>
                {line}
              </div>
            ));
          }

          if (hunk.removed) {
            return trimmed.map((line, li) => (
              <div key={`${idx}-${li}`} className="bg-red-950/40 text-red-300 px-3 py-0.5 leading-5">
                <span className="select-none text-red-600 mr-2">-</span>
                {line}
              </div>
            ));
          }

          return trimmed.map((line, li) => (
            <div key={`${idx}-${li}`} className="bg-zinc-900/60 text-zinc-500 px-3 py-0.5 leading-5">
              <span className="select-none mr-2"> </span>
              {line}
            </div>
          ));
        })}
      </pre>
    </div>
  );
}

// ── Span diff card ────────────────────────────────────────────────────────────

interface SpanDiffCardProps {
  span: SpanPromptDiff;
}

function SpanDiffCard({ span }: SpanDiffCardProps) {
  const [open, setOpen] = useState(true);
  const badge = STATUS_BADGE[span.status];
  const changedPrompts = span.prompt_diffs.filter((d) => d.changed);

  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] overflow-hidden">
      {/* Card header */}
      <button
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        className="w-full flex items-center justify-between px-4 py-3 text-left hover:bg-white/[0.02] transition-colors"
      >
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-sm font-mono text-[var(--text)] truncate">
            <span className="text-blue-300">{span.agent_name}</span>
            <span className="text-zinc-600 mx-1">::</span>
            <span>{span.span_name}</span>
            <span className="text-zinc-600 ml-1">[{span.call_index}]</span>
          </span>
        </div>
        <div className="flex items-center gap-2 shrink-0 ml-3">
          <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${badge.className}`}>
            {badge.label}
          </span>
          <span className="text-zinc-500 text-xs">{open ? "▲" : "▼"}</span>
        </div>
      </button>

      {/* Card body */}
      {open && (
        <div className="px-4 pb-4 pt-1 border-t border-[var(--border)]">
          <ParamsTable params={span.param_diffs} />

          {changedPrompts.length > 0 && (
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)] mb-3">
                Prompt Diffs
              </p>
              {changedPrompts.map((d) => (
                <FieldDiffView key={d.field_name} diff={d} />
              ))}
            </div>
          )}

          {changedPrompts.length === 0 && span.param_diffs.length === 0 && (
            <p className="text-xs text-[var(--text-muted)] italic">No detailed diff available.</p>
          )}
        </div>
      )}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

interface ComparisonPromptDiffProps {
  data: RunPromptDiff;
}

export function ComparisonPromptDiff({ data }: ComparisonPromptDiffProps) {
  const changedCount = data.spans.length;
  const unchangedCount = data.unchanged_count;

  return (
    <div className="space-y-3">
      {/* Section summary */}
      <div className="flex items-center gap-3 flex-wrap">
        <p className="text-sm text-[var(--text-muted)]">
          <span className="font-semibold text-[var(--text)]">{changedCount}</span>{" "}
          {changedCount === 1 ? "span" : "spans"} changed
          {unchangedCount > 0 && (
            <>
              {" "}
              <span className="text-zinc-600">·</span>{" "}
              <span className="font-semibold text-[var(--text)]">{unchangedCount}</span> unchanged
            </>
          )}
        </p>
      </div>

      {/* Truncation warning */}
      {data.truncated && (
        <div className="rounded-lg border border-amber-700/50 bg-amber-950/20 px-4 py-3 flex items-center gap-2">
          <span className="text-amber-400 text-sm font-medium">
            Results truncated — only the first batch of changed spans is shown.
          </span>
        </div>
      )}

      {/* No changes */}
      {changedCount === 0 && (
        <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] px-6 py-8 text-center">
          <p className="text-sm text-[var(--text-muted)]">No prompt changes detected between these runs.</p>
        </div>
      )}

      {/* Span diff cards */}
      {data.spans.map((span) => (
        <SpanDiffCard key={span.span_key} span={span} />
      ))}
    </div>
  );
}
