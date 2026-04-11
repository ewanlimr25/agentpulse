"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { Span, SpanEval, SpanEvalGroup, SpanFeedback, FeedbackRequest, PlaygroundMessage } from "@/lib/types";
import { SpanKindBadge } from "./SpanKindBadge";
import { CollapsibleSection } from "./CollapsibleSection";
import { formatDurationNS } from "@/lib/format";
import { spanFeedbackApi, playgroundApi } from "@/lib/api";
import { Beaker } from "lucide-react";

interface Props {
  span: Span;
  evals?: SpanEval[];
  evalGroups?: SpanEvalGroup[];
  runStartTime: string;
  projectId: string;
  runId: string;
  feedback?: SpanFeedback | null;
  onFeedbackChange?: (feedback: SpanFeedback | null) => void;
  isResolvingPayload?: boolean;
}

function evalScoreClasses(score: number): string {
  if (score >= 0.7) return "bg-green-950/40 border border-green-700 text-green-400";
  if (score >= 0.4) return "bg-amber-950/40 border border-amber-700 text-amber-400";
  return "bg-red-950/40 border border-red-700 text-red-400";
}

function StatusDot({ code }: { code: string }) {
  const isOk = code === "STATUS_CODE_OK" || code === "Ok" || code === "ok";
  const isErr = code === "STATUS_CODE_ERROR" || code === "Error" || code === "error";
  const color = isOk ? "bg-green-500" : isErr ? "bg-red-500" : "bg-zinc-500";
  const label = isOk ? "ok" : isErr ? "error" : "unset";
  return (
    <span className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
      <span className={`inline-block w-2 h-2 rounded-full ${color}`} />
      {label}
    </span>
  );
}

function KVTable({ entries }: { entries: [string, string][] }) {
  if (entries.length === 0) return null;
  return (
    <table className="w-full text-xs">
      <tbody>
        {entries.map(([k, v]) => (
          <tr key={k} className="border-t border-[var(--border)] first:border-0">
            <td className="py-1.5 pr-3 font-mono text-[var(--text-muted)] align-top w-2/5 break-all">{k}</td>
            <td className="py-1.5 font-mono text-[var(--text)] align-top break-all">{v}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function groupAttributes(attrs: Record<string, string>): [string, [string, string][]][] {
  const groups: Record<string, [string, string][]> = {};

  for (const [k, v] of Object.entries(attrs)) {
    const dot = k.indexOf(".");
    const prefix = dot > -1 ? k.slice(0, dot) : "_other";
    if (!groups[prefix]) groups[prefix] = [];
    groups[prefix].push([k, v]);
  }

  // Sort: known prefixes first, then alphabetical
  const order = ["gen_ai", "llm", "tool", "agent", "memory"];
  const sorted = Object.entries(groups).sort(([a], [b]) => {
    const ai = order.indexOf(a);
    const bi = order.indexOf(b);
    if (ai > -1 && bi > -1) return ai - bi;
    if (ai > -1) return -1;
    if (bi > -1) return 1;
    return a.localeCompare(b);
  });

  return sorted;
}

export function SpanDetailContent({ span, evals, evalGroups, runStartTime, projectId, runId, feedback, onFeedbackChange, isResolvingPayload }: Props) {
  const router = useRouter();
  const [localFeedback, setLocalFeedback] = useState<SpanFeedback | null>(feedback ?? null);
  const [correctedOutput, setCorrectedOutput] = useState("");
  const [saving, setSaving] = useState(false);
  const [creating, setCreating] = useState(false);

  async function handleRate(rating: "good" | "bad") {
    if (saving) return;
    setSaving(true);
    try {
      const req: FeedbackRequest = { run_id: runId, rating };
      if (rating === "bad" && correctedOutput.trim()) {
        req.corrected_output = correctedOutput.trim();
      }
      const updated = await spanFeedbackApi.upsert(projectId, span.SpanID, req);
      setLocalFeedback(updated);
      onFeedbackChange?.(updated);
    } finally {
      setSaving(false);
    }
  }

  async function handleRemove() {
    if (saving) return;
    setSaving(true);
    try {
      await spanFeedbackApi.delete(projectId, span.SpanID);
      setLocalFeedback(null);
      onFeedbackChange?.(null);
    } finally {
      setSaving(false);
    }
  }

  const runStart = new Date(runStartTime).getTime();
  const spanStart = new Date(span.StartTime).getTime();
  const offsetMS = spanStart - runStart;

  const attrs = span.Attributes ?? {};

  function resolvePayloadField(value: string | undefined): string | null {
    if (!value) return null;
    if (value.startsWith("payload_ref:")) return null;
    return value;
  }

  const rawPrompt = attrs["gen_ai.prompt"] ?? attrs["llm.prompt"];
  const rawCompletion = attrs["gen_ai.completion"] ?? attrs["llm.completion"];
  const rawToolInput = attrs["tool.input"];
  const rawToolOutput = attrs["tool.output"];
  const prompt = resolvePayloadField(rawPrompt);
  const completion = resolvePayloadField(rawCompletion);
  const toolInput = resolvePayloadField(rawToolInput);
  const toolOutput = resolvePayloadField(rawToolOutput);
  const promptOffloaded = isResolvingPayload && (rawPrompt === "" || (rawPrompt?.startsWith("payload_ref:") ?? false));
  const completionOffloaded = isResolvingPayload && (rawCompletion === "" || (rawCompletion?.startsWith("payload_ref:") ?? false));
  const toolInputOffloaded = isResolvingPayload && (rawToolInput === "" || (rawToolInput?.startsWith("payload_ref:") ?? false));
  const toolOutputOffloaded = isResolvingPayload && (rawToolOutput === "" || (rawToolOutput?.startsWith("payload_ref:") ?? false));

  async function handleOpenInPlayground() {
    if (creating || !prompt) return;
    setCreating(true);
    try {
      let messages: PlaygroundMessage[];
      const trimmed = prompt.trim();
      if (trimmed.startsWith("[") || trimmed.startsWith("{")) {
        try {
          const parsed = JSON.parse(trimmed);
          const arr = Array.isArray(parsed) ? parsed : [parsed];
          messages = arr.map((m: { role?: string; content?: string }) => ({
            role: (m.role as PlaygroundMessage["role"]) ?? "user",
            content: m.content ?? String(m),
          }));
        } catch {
          messages = [{ role: "user", content: prompt }];
        }
      } else {
        messages = [{ role: "user", content: prompt }];
      }

      const tempRaw = attrs["gen_ai.request.temperature"];
      const temperature = tempRaw ? parseFloat(tempRaw) : undefined;
      const maxRaw = attrs["gen_ai.request.max_tokens"];
      const maxTokens = maxRaw ? parseInt(maxRaw, 10) : undefined;
      const modelId = span.ModelID || "gpt-4o";

      const name = `Playground from ${span.SpanName}`.slice(0, 60);

      const variantConfig = {
        model_id: modelId,
        messages,
        ...(temperature != null && !isNaN(temperature) ? { temperature } : {}),
        ...(maxTokens != null && !isNaN(maxTokens) ? { max_tokens: maxTokens } : {}),
      };

      const session = await playgroundApi.createSession(projectId, {
        name,
        source_span_id: span.SpanID,
        source_run_id: runId,
        variants: [
          { label: "Variant A", ...variantConfig },
          { label: "Variant B", ...variantConfig },
        ],
      });

      router.push(`/projects/${projectId}/playground/${session.ID}`);
    } finally {
      setCreating(false);
    }
  }

  // Exclude I/O keys from the attribute table (shown separately)
  const ioKeys = new Set(["gen_ai.prompt", "gen_ai.completion", "llm.prompt", "llm.completion", "tool.input", "tool.output"]);
  const filteredAttrs = Object.fromEntries(Object.entries(attrs).filter(([k]) => !ioKeys.has(k)));
  const attrGroups = groupAttributes(filteredAttrs);
  const resourceGroups = groupAttributes(span.ResourceAttrs ?? {});

  return (
    <div className="flex flex-col gap-5 px-5 py-4">
      {/* Header */}
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2 flex-wrap">
          <SpanKindBadge kind={span.AgentSpanKind} />
          <StatusDot code={span.StatusCode} />
        </div>
        <h3 className="text-base font-semibold text-[var(--text)] break-all">{span.SpanName}</h3>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]">
          {span.AgentName && (
            <span>agent: <span className="text-indigo-400">{span.AgentName}</span></span>
          )}
          {span.ServiceName && (
            <span>service: <span className="text-[var(--text)]">{span.ServiceName}</span></span>
          )}
          {span.ModelID && (
            <span>model: <span className="text-violet-400">{span.ModelID}</span></span>
          )}
        </div>
        {span.StatusMessage && (
          <p className="text-xs text-red-400 break-all">{span.StatusMessage}</p>
        )}
      </div>

      {/* Timing */}
      <div>
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">Timing</p>
        <div className="grid grid-cols-2 gap-2 text-xs">
          <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
            <p className="text-[var(--text-muted)] mb-0.5">Start</p>
            <p className="font-mono text-[var(--text)]">{new Date(span.StartTime).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3 })}</p>
          </div>
          <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
            <p className="text-[var(--text-muted)] mb-0.5">Duration</p>
            <p className="font-mono text-[var(--text)]">{formatDurationNS(span.DurationNS)}</p>
          </div>
          <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2 col-span-2">
            <p className="text-[var(--text-muted)] mb-0.5">Offset from run start</p>
            <p className="font-mono text-[var(--text)]">+{offsetMS >= 0 ? (offsetMS / 1000).toFixed(3) : "?"}s</p>
          </div>
        </div>
      </div>

      {/* Quality Scores — grouped multi-model view when available, flat fallback otherwise */}
      {(() => {
        // Build a map of eval name -> group for this span
        const groupMap = new Map<string, SpanEvalGroup>();
        if (evalGroups) {
          for (const g of evalGroups) {
            if (g.SpanID === span.SpanID) {
              groupMap.set(g.EvalName, g);
            }
          }
        }

        const hasGroups = groupMap.size > 0;
        const hasEvals = evals && evals.length > 0;

        if (!hasGroups && !hasEvals) return null;

        // If we have group data, render grouped view for all evals in groups,
        // then fall back to flat evals for any not covered by groups.
        const coveredEvalNames = new Set(groupMap.keys());
        const ungroupedEvals = (evals ?? []).filter((e) => !coveredEvalNames.has(e.EvalName));

        const totalCount = groupMap.size + ungroupedEvals.length;

        return (
          <div>
            <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">
              {totalCount === 1 ? "Quality Score" : "Quality Scores"}
            </p>
            <div className="flex flex-col gap-3">
              {/* Multi-model grouped evals */}
              {Array.from(groupMap.values()).map((group) => {
                if (group.Scores.length < 2) {
                  // Single-score group — render as flat card
                  const score = group.Scores[0];
                  if (!score) return null;
                  return (
                    <div key={group.EvalName} className={`rounded-lg p-3 ${evalScoreClasses(score.Score)}`}>
                      <div className="flex items-center gap-3 mb-1.5">
                        <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-sm font-mono tabular-nums ${evalScoreClasses(score.Score)}`}>
                          <span>●</span>
                          <span>{score.Score.toFixed(2)}</span>
                        </span>
                        <span className="text-xs text-[var(--text-muted)] capitalize">{group.EvalName.replace(/_/g, " ")}</span>
                      </div>
                      {score.Reasoning && (
                        <p className="text-xs text-[var(--text-muted)] leading-relaxed">{score.Reasoning}</p>
                      )}
                      <p className="text-xs text-[var(--text-muted)] mt-1">
                        judge: <span className="font-mono text-[var(--text)]">{score.Model}</span>
                      </p>
                    </div>
                  );
                }

                // Multi-model comparison table
                const consensus = group.ConsensusScore;
                return (
                  <div key={group.EvalName} className="rounded-lg border border-[var(--border)] bg-[var(--surface-2)] overflow-hidden">
                    {/* Header row */}
                    <div className="flex items-center gap-2 px-3 py-2 border-b border-[var(--border)]">
                      <span className="text-xs font-medium text-[var(--text)] capitalize flex-1">
                        {group.EvalName.replace(/_/g, " ")}
                      </span>
                      {consensus !== null && (
                        <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono tabular-nums ${evalScoreClasses(consensus)}`}>
                          <span>●</span>
                          <span>{(consensus * 100).toFixed(0)}%</span>
                        </span>
                      )}
                      {group.Disagreement && (
                        <span className="text-amber-400 text-sm" title="Models disagree on this score">&#9888;</span>
                      )}
                    </div>
                    {/* Per-model rows */}
                    <table className="w-full text-xs">
                      <tbody>
                        {group.Scores.map((ms) => (
                          <tr key={ms.Model} className="border-t border-[var(--border)] first:border-0">
                            <td className="px-3 py-2 font-mono text-[var(--text-muted)] w-2/5 truncate">{ms.Model}</td>
                            <td className="px-2 py-2 w-16">
                              <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded font-mono tabular-nums ${evalScoreClasses(ms.Score)}`}>
                                <span>●</span>
                                <span>{ms.Score.toFixed(2)}</span>
                              </span>
                            </td>
                            <td className="px-2 py-2 text-[var(--text-muted)] leading-relaxed">
                              {ms.Reasoning
                                ? ms.Reasoning.length > 100
                                  ? `${ms.Reasoning.slice(0, 100)}…`
                                  : ms.Reasoning
                                : null}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                );
              })}

              {/* Flat fallback for evals not in any group */}
              {ungroupedEvals.map((spanEval) => (
                <div key={spanEval.EvalName} className={`rounded-lg p-3 ${evalScoreClasses(spanEval.Score)}`}>
                  <div className="flex items-center gap-3 mb-1.5">
                    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-sm font-mono tabular-nums ${evalScoreClasses(spanEval.Score)}`}>
                      <span>●</span>
                      <span>{spanEval.Score.toFixed(2)}</span>
                    </span>
                    <span className="text-xs text-[var(--text-muted)] capitalize">{spanEval.EvalName.replace(/_/g, " ")}</span>
                  </div>
                  {spanEval.Reasoning && (
                    <p className="text-xs text-[var(--text-muted)] leading-relaxed">{spanEval.Reasoning}</p>
                  )}
                  <p className="text-xs text-[var(--text-muted)] mt-1">
                    judge: <span className="font-mono text-[var(--text)]">{spanEval.JudgeModel}</span>
                  </p>
                </div>
              ))}
            </div>
          </div>
        );
      })()}

      {/* Human Feedback */}
      <div>
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">Human Feedback</p>
        <div className="flex items-center gap-2 mb-2">
          <button
            type="button"
            disabled={saving}
            onClick={() => handleRate("good")}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 ${
              localFeedback?.Rating === "good"
                ? "bg-green-700 text-white"
                : "bg-zinc-800 text-[var(--text-muted)] hover:bg-zinc-700"
            }`}
          >
            <span>&#128077;</span> Good
          </button>
          <button
            type="button"
            disabled={saving}
            onClick={() => handleRate("bad")}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors disabled:opacity-50 ${
              localFeedback?.Rating === "bad"
                ? "bg-red-700 text-white"
                : "bg-zinc-800 text-[var(--text-muted)] hover:bg-zinc-700"
            }`}
          >
            <span>&#128078;</span> Bad
          </button>
          {localFeedback && (
            <button
              type="button"
              disabled={saving}
              onClick={handleRemove}
              className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] underline ml-1 disabled:opacity-50"
            >
              Remove feedback
            </button>
          )}
        </div>
        {localFeedback?.Rating === "bad" && (
          <textarea
            value={correctedOutput}
            onChange={(e) => setCorrectedOutput(e.target.value)}
            placeholder="Optional: enter corrected output..."
            rows={3}
            className="w-full text-xs font-mono bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-[var(--text)] placeholder:text-[var(--text-muted)] resize-y focus:outline-none focus:border-zinc-500"
          />
        )}
      </div>

      {/* Cost & Tokens (LLM only) */}
      {span.AgentSpanKind === "llm.call" && (span.TotalTokens > 0 || span.CostUSD > 0) && (
        <div>
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">Cost &amp; Tokens</p>
          <div className="grid grid-cols-2 gap-2 text-xs">
            <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
              <p className="text-[var(--text-muted)] mb-0.5">Input tokens</p>
              <p className="font-mono text-[var(--text)]">{span.InputTokens.toLocaleString()}</p>
            </div>
            <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
              <p className="text-[var(--text-muted)] mb-0.5">Output tokens</p>
              <p className="font-mono text-[var(--text)]">{span.OutputTokens.toLocaleString()}</p>
            </div>
            <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
              <p className="text-[var(--text-muted)] mb-0.5">Total tokens</p>
              <p className="font-mono text-[var(--text)]">{span.TotalTokens.toLocaleString()}</p>
            </div>
            <div className="bg-[var(--surface-2)] rounded-lg px-3 py-2">
              <p className="text-[var(--text-muted)] mb-0.5">Cost</p>
              <p className="font-mono text-indigo-400">${span.CostUSD.toFixed(5)}</p>
            </div>
          </div>
        </div>
      )}

      {/* Open in Playground (LLM only, prompt resolved) */}
      {span.AgentSpanKind === "llm.call" && prompt && (
        <div>
          <button
            type="button"
            disabled={creating || !prompt}
            onClick={handleOpenInPlayground}
            className="flex items-center gap-1.5 text-xs font-medium text-indigo-400 hover:text-indigo-300 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            <Beaker className="w-3.5 h-3.5" />
            {creating ? "Creating..." : "Open in Playground"}
          </button>
        </div>
      )}

      {/* Input / Output */}
      {(prompt || completion || promptOffloaded || completionOffloaded || toolInput || toolOutput || toolInputOffloaded || toolOutputOffloaded) && (
        <div className="flex flex-col gap-2">
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Input / Output</p>
          {(prompt || promptOffloaded) && (
            <CollapsibleSection title="Prompt (input)" defaultOpen={promptOffloaded}>
              {promptOffloaded ? (
                <div className="animate-pulse flex flex-col gap-2 py-1">
                  <div className="h-3 bg-[var(--border)] rounded w-full" />
                  <div className="h-3 bg-[var(--border)] rounded w-5/6" />
                  <div className="h-3 bg-[var(--border)] rounded w-4/6" />
                </div>
              ) : (
                <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                  {prompt}
                </pre>
              )}
            </CollapsibleSection>
          )}
          {(completion || completionOffloaded) && (
            <CollapsibleSection title="Completion (output)" defaultOpen={completionOffloaded}>
              {completionOffloaded ? (
                <div className="animate-pulse flex flex-col gap-2 py-1">
                  <div className="h-3 bg-[var(--border)] rounded w-full" />
                  <div className="h-3 bg-[var(--border)] rounded w-3/4" />
                </div>
              ) : (
                <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                  {completion}
                </pre>
              )}
            </CollapsibleSection>
          )}
          {(toolInput || toolInputOffloaded) && (
            <CollapsibleSection title="Tool input" defaultOpen={toolInputOffloaded}>
              {toolInputOffloaded ? (
                <div className="animate-pulse flex flex-col gap-2 py-1">
                  <div className="h-3 bg-[var(--border)] rounded w-full" />
                  <div className="h-3 bg-[var(--border)] rounded w-5/6" />
                </div>
              ) : (
                <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                  {toolInput}
                </pre>
              )}
            </CollapsibleSection>
          )}
          {(toolOutput || toolOutputOffloaded) && (
            <CollapsibleSection title="Tool output" defaultOpen={toolOutputOffloaded}>
              {toolOutputOffloaded ? (
                <div className="animate-pulse flex flex-col gap-2 py-1">
                  <div className="h-3 bg-[var(--border)] rounded w-full" />
                  <div className="h-3 bg-[var(--border)] rounded w-3/4" />
                </div>
              ) : (
                <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                  {toolOutput}
                </pre>
              )}
            </CollapsibleSection>
          )}
        </div>
      )}

      {/* Attributes */}
      {attrGroups.length > 0 && (
        <div className="flex flex-col gap-2">
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Attributes</p>
          {attrGroups.map(([prefix, entries]) => (
            <CollapsibleSection
              key={prefix}
              title={prefix === "_other" ? "other" : prefix}
              defaultOpen={["gen_ai", "llm", "tool", "agent"].includes(prefix)}
            >
              <KVTable entries={entries} />
            </CollapsibleSection>
          ))}
        </div>
      )}

      {/* Resource Attributes */}
      {resourceGroups.length > 0 && (
        <div className="flex flex-col gap-2">
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Resource Attributes</p>
          {resourceGroups.map(([prefix, entries]) => (
            <CollapsibleSection key={prefix} title={prefix === "_other" ? "other" : prefix}>
              <KVTable entries={entries} />
            </CollapsibleSection>
          ))}
        </div>
      )}

      {/* Span IDs */}
      <div>
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">IDs</p>
        <div className="flex flex-col gap-1 text-xs font-mono">
          <div className="flex gap-2">
            <span className="text-[var(--text-muted)] w-20 shrink-0">Span</span>
            <span className="text-[var(--text)] break-all">{span.SpanID}</span>
          </div>
          {span.ParentSpanID && (
            <div className="flex gap-2">
              <span className="text-[var(--text-muted)] w-20 shrink-0">Parent</span>
              <span className="text-[var(--text)] break-all">{span.ParentSpanID}</span>
            </div>
          )}
          <div className="flex gap-2">
            <span className="text-[var(--text-muted)] w-20 shrink-0">Trace</span>
            <span className="text-[var(--text)] break-all">{span.TraceID}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
