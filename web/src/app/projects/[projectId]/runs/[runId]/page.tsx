"use client";

import { use, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { runsApi, evalsApi, loopsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { MetricCard } from "@/components/ui/MetricCard";
import { SpanKindBadge } from "@/components/spans/SpanKindBadge";
import { SpanDetailDrawer } from "@/components/spans/SpanDetailDrawer";
import { LoopBanner } from "@/components/loops/LoopBanner";
import type { Span, SpanEval } from "@/lib/types";
import { formatDurationNS } from "@/lib/format";

function scoreColorClasses(score: number): string {
  if (score >= 0.7) return "bg-green-950/40 border border-green-700 text-green-400";
  if (score >= 0.4) return "bg-amber-950/40 border border-amber-700 text-amber-400";
  return "bg-red-950/40 border border-red-700 text-red-400";
}

function ScoreBadge({ eval: e }: { eval: SpanEval }) {
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono tabular-nums shrink-0 ${scoreColorClasses(e.Score)}`}>
      <span>●</span>
      <span>{e.Score.toFixed(2)}</span>
    </span>
  );
}

function SpanRow({ span, eval: spanEval, onClick }: { span: Span; eval?: SpanEval; onClick: () => void }) {
  return (
    <div
      onClick={onClick}
      className="flex items-start gap-4 px-4 py-3 border border-[var(--border)] bg-[var(--surface)] rounded-lg text-sm cursor-pointer hover:border-indigo-600/60 transition-colors"
    >
      <SpanKindBadge kind={span.AgentSpanKind} />
      <div className="flex-1 min-w-0">
        <p className="font-medium text-[var(--text)] truncate">{span.SpanName}</p>
        {span.AgentName && (
          <p className="text-xs text-[var(--text-muted)]">
            agent: <span className="text-indigo-400">{span.AgentName}</span>
            {span.ModelID && (
              <> · model: <span className="text-violet-400">{span.ModelID}</span></>
            )}
          </p>
        )}
      </div>
      <div className="flex items-center gap-4 text-xs tabular-nums text-[var(--text-muted)] shrink-0">
        {spanEval && <ScoreBadge eval={spanEval} />}
        {span.TotalTokens > 0 && <span>{span.TotalTokens.toLocaleString()} tok</span>}
        {span.CostUSD > 0 && <span>${span.CostUSD.toFixed(5)}</span>}
        <span>{formatDurationNS(span.DurationNS)}</span>
      </div>
    </div>
  );
}

export default function RunPage({
  params,
}: {
  params: Promise<{ projectId: string; runId: string }>;
}) {
  const { projectId, runId } = use(params);
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  const { data: run } = useQuery({
    queryKey: ["run", runId],
    queryFn: () => runsApi.get(runId),
  });

  const { data: spans, isLoading: spansLoading } = useQuery({
    queryKey: ["spans", runId],
    queryFn: () => runsApi.spans(runId),
  });

  const { data: evals } = useQuery({
    queryKey: ["evals", runId],
    queryFn: () => evalsApi.listByRun(runId),
  });

  const { data: loops } = useQuery({
    queryKey: ["loops", runId],
    queryFn: () => loopsApi.listByRun(runId),
  });

  // Build a map from spanId to the latest eval (for the "relevance" eval_name)
  const evalsBySpan = new Map<string, SpanEval>(
    evals?.map((e) => [e.SpanID, e]) ?? []
  );

  const selectedSpan = spans?.find((s) => s.SpanID === selectedSpanId);
  const selectedEval = selectedSpanId ? evalsBySpan.get(selectedSpanId) : undefined;

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
        <div className="mb-2 flex items-center gap-2 text-sm text-[var(--text-muted)]">
          <Link href="/" className="hover:text-indigo-400">Projects</Link>
          <span>/</span>
          <Link href={`/projects/${projectId}`} className="hover:text-indigo-400">{projectId.slice(0, 8)}…</Link>
          <span>/</span>
          <span className="text-[var(--text)]">{runId.slice(0, 8)}…</span>
        </div>

        <div className="flex items-center gap-4 mb-6">
          <h1 className="text-xl font-bold text-[var(--text)] font-mono">{runId}</h1>
          {run && (
            <StatusBadge status={run.Status === "ok" ? "ok" : "error"} size="md" />
          )}
          <Link
            href={`/projects/${projectId}/runs/${runId}/topology`}
            className="ml-auto px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium transition-colors"
          >
            View Topology →
          </Link>
        </div>

        <LoopBanner loops={loops ?? []} />

        {run && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
            <MetricCard
              label="Duration"
              value={run.DurationMS < 1000 ? `${run.DurationMS.toFixed(0)}ms` : `${(run.DurationMS / 1000).toFixed(1)}s`}
            />
            <MetricCard label="Cost" value={`$${run.TotalCostUSD.toFixed(4)}`} accent />
            <MetricCard
              label="Tokens"
              value={run.TotalTokens.toLocaleString()}
              sub={`${run.TotalInputTokens.toLocaleString()} in / ${run.TotalOutputTokens.toLocaleString()} out`}
            />
            <MetricCard
              label="Spans"
              value={run.SpanCount}
              sub={`${run.LLMCallCount} LLM · ${run.ToolCallCount} tool`}
            />
          </div>
        )}

        <h2 className="text-lg font-semibold text-[var(--text)] mb-4">Spans</h2>

        {spansLoading && <div className="text-[var(--text-muted)]">Loading spans...</div>}

        <div className="flex flex-col gap-2">
          {spans?.map((s) => (
            <SpanRow
              key={s.SpanID}
              span={s}
              eval={evalsBySpan.get(s.SpanID)}
              onClick={() => setSelectedSpanId(s.SpanID)}
            />
          ))}
          {!spansLoading && spans?.length === 0 && (
            <div className="text-[var(--text-muted)] text-center py-8">No spans found.</div>
          )}
        </div>
      </main>

      <SpanDetailDrawer
        span={selectedSpan}
        eval={selectedEval}
        runStartTime={run?.StartTime ?? ""}
        onClose={() => setSelectedSpanId(null)}
      />
    </div>
  );
}
