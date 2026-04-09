"use client";

import { useMemo } from "react";
import type { Span } from "@/lib/types";

interface Props {
  spansA: Span[];
  spansB: Span[];
}

type DiffKind = "shared" | "diverged" | "override" | "only-a" | "only-b";

interface Row {
  key: string;
  agentName: string;
  spanName: string;
  callIndex: number;
  a?: Span;
  b?: Span;
  kind: DiffKind;
}

function callIndexMap(spans: Span[]): Map<string, number> {
  const counters = new Map<string, number>();
  const out = new Map<string, number>();
  // Sort by start_time ascending to assign deterministic call indices.
  const sorted = [...spans].sort((x, y) => x.StartTime.localeCompare(y.StartTime));
  for (const s of sorted) {
    const base = `${s.AgentName}::${s.SpanName}`;
    const next = (counters.get(base) ?? 0);
    counters.set(base, next + 1);
    out.set(s.SpanID, next);
  }
  return out;
}

const colorClass: Record<DiffKind, string> = {
  shared: "border-[var(--border)] bg-[var(--surface)] text-[var(--text-muted)]",
  diverged: "border-amber-700/60 bg-amber-950/30 text-amber-300",
  override: "border-violet-700/60 bg-violet-950/30 text-violet-300",
  "only-a": "border-blue-700/60 bg-blue-950/30 text-blue-300",
  "only-b": "border-orange-700/60 bg-orange-950/30 text-orange-300",
};

const labelMap: Record<DiffKind, string> = {
  shared: "shared",
  diverged: "diverged",
  override: "override",
  "only-a": "only original",
  "only-b": "only replay",
};

export function ReplaySpanDiff({ spansA, spansB }: Props) {
  const rows = useMemo<Row[]>(() => {
    const ciA = callIndexMap(spansA);
    const ciB = callIndexMap(spansB);

    const keyOf = (s: Span, ci: Map<string, number>) =>
      `${s.AgentName}::${s.SpanName}::${ci.get(s.SpanID) ?? 0}`;

    const bByKey = new Map<string, Span>();
    for (const s of spansB) bByKey.set(keyOf(s, ciB), s);

    const seenB = new Set<string>();
    const out: Row[] = [];

    const sortedA = [...spansA].sort((x, y) => x.StartTime.localeCompare(y.StartTime));
    for (const a of sortedA) {
      const key = keyOf(a, ciA);
      const b = bByKey.get(key);
      if (b) seenB.add(key);

      let kind: DiffKind = "shared";
      if (!b) {
        kind = "only-a";
      } else if (b.Attributes?.["agentpulse.replay.override"] === "true") {
        kind = "override";
      } else if (b.Attributes?.["agentpulse.replay.diverged"] === "true") {
        kind = "diverged";
      }

      out.push({
        key,
        agentName: a.AgentName,
        spanName: a.SpanName,
        callIndex: ciA.get(a.SpanID) ?? 0,
        a,
        b,
        kind,
      });
    }

    const sortedB = [...spansB].sort((x, y) => x.StartTime.localeCompare(y.StartTime));
    for (const b of sortedB) {
      const key = keyOf(b, ciB);
      if (seenB.has(key)) continue;
      out.push({
        key,
        agentName: b.AgentName,
        spanName: b.SpanName,
        callIndex: ciB.get(b.SpanID) ?? 0,
        b,
        kind: "only-b",
      });
    }

    return out;
  }, [spansA, spansB]);

  if (rows.length === 0) {
    return (
      <div className="text-sm text-[var(--text-muted)]">No spans to diff.</div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3 text-xs text-[var(--text-muted)]">
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm border border-amber-600 bg-amber-950/40 inline-block" />
          Diverged input
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm border border-violet-600 bg-violet-950/40 inline-block" />
          Override applied
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm border border-blue-600 bg-blue-950/40 inline-block" />
          Only in original
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-sm border border-orange-600 bg-orange-950/40 inline-block" />
          Only in replay
        </span>
      </div>

      <div className="flex flex-col gap-2">
        {rows.map((r) => (
          <div
            key={r.key}
            className={`flex items-center gap-3 px-3 py-2 rounded-lg border text-xs ${colorClass[r.kind]}`}
          >
            <span className="font-mono text-[10px] uppercase tracking-wide opacity-80 w-24 shrink-0">
              {labelMap[r.kind]}
            </span>
            <div className="flex-1 min-w-0">
              <p className="font-medium text-[var(--text)] truncate">
                {r.spanName}
                {r.callIndex > 0 && (
                  <span className="opacity-60"> · #{r.callIndex}</span>
                )}
              </p>
              {r.agentName && (
                <p className="opacity-70 truncate">agent: {r.agentName}</p>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
