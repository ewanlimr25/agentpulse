import type { Span } from "@/lib/types";
import { SpanKindBadge } from "./SpanKindBadge";
import { CollapsibleSection } from "./CollapsibleSection";
import { formatDurationNS } from "@/lib/format";

interface Props {
  span: Span;
  runStartTime: string;
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

export function SpanDetailContent({ span, runStartTime }: Props) {
  const runStart = new Date(runStartTime).getTime();
  const spanStart = new Date(span.StartTime).getTime();
  const offsetMS = spanStart - runStart;

  const attrs = span.Attributes ?? {};
  const prompt = attrs["gen_ai.prompt"] ?? attrs["llm.prompt"] ?? null;
  const completion = attrs["gen_ai.completion"] ?? attrs["llm.completion"] ?? null;

  // Exclude I/O keys from the attribute table (shown separately)
  const ioKeys = new Set(["gen_ai.prompt", "gen_ai.completion", "llm.prompt", "llm.completion"]);
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

      {/* Input / Output */}
      {(prompt || completion) && (
        <div className="flex flex-col gap-2">
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Input / Output</p>
          {prompt && (
            <CollapsibleSection title="Prompt (input)">
              <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                {prompt}
              </pre>
            </CollapsibleSection>
          )}
          {completion && (
            <CollapsibleSection title="Completion (output)">
              <pre className="text-xs font-mono text-[var(--text-muted)] whitespace-pre-wrap break-all max-h-64 overflow-y-auto leading-relaxed">
                {completion}
              </pre>
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
