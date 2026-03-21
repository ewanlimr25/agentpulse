import type { AgentSpanKind } from "@/lib/types";

const kindColor: Record<string, string> = {
  "llm.call": "text-violet-400 bg-violet-950/40 border-violet-800",
  "tool.call": "text-sky-400 bg-sky-950/40 border-sky-800",
  "agent.handoff": "text-green-400 bg-green-950/40 border-green-800",
  "memory.read": "text-amber-400 bg-amber-950/40 border-amber-800",
  "memory.write": "text-orange-400 bg-orange-950/40 border-orange-800",
  unknown: "text-zinc-400 bg-zinc-800 border-zinc-700",
};

interface Props {
  kind: AgentSpanKind | string;
}

export function SpanKindBadge({ kind }: Props) {
  const cls = kindColor[kind] ?? kindColor.unknown;
  return (
    <span className={`shrink-0 rounded border px-2 py-0.5 text-xs font-medium ${cls}`}>
      {kind}
    </span>
  );
}
