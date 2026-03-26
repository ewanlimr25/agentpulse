"use client";

import { Handle, Position } from "@xyflow/react";
import type { NodeType, NodeStatus } from "@/lib/types";

interface NodeData {
  label: string;
  nodeType: NodeType;
  status: NodeStatus;
  costUSD: number;
  tokenCount: number;
  metadata?: Record<string, string>;
}

const nodeColors: Record<NodeType, { bg: string; border: string; text: string }> = {
  agent: { bg: "bg-indigo-950/70", border: "border-indigo-600", text: "text-indigo-300" },
  llm: { bg: "bg-violet-950/70", border: "border-violet-600", text: "text-violet-300" },
  tool: { bg: "bg-sky-950/70", border: "border-sky-600", text: "text-sky-300" },
  memory: { bg: "bg-amber-950/70", border: "border-amber-600", text: "text-amber-300" },
  mcp: { bg: "bg-teal-950/70", border: "border-teal-600", text: "text-teal-300" },
};

const nodeIcons: Record<NodeType, string> = {
  agent: "🤖",
  llm: "💬",
  tool: "🔧",
  memory: "🧠",
  mcp: "🔌",
};

const statusDot: Record<NodeStatus, string> = {
  ok: "bg-green-400",
  error: "bg-red-400",
  running: "bg-blue-400 animate-pulse",
  unset: "bg-zinc-500",
};

export function AgentNode({ data }: { data: NodeData }) {
  const colors = nodeColors[data.nodeType];
  return (
    <div
      className={`rounded-xl border-2 ${colors.border} ${colors.bg} px-4 py-3 min-w-[160px] shadow-lg`}
    >
      <Handle type="target" position={Position.Top} className="!bg-[var(--border)]" />

      <div className="flex items-center gap-2 mb-2">
        <span className="text-base">{nodeIcons[data.nodeType]}</span>
        <span className={`text-xs font-semibold uppercase tracking-wide ${colors.text}`}>
          {data.nodeType}
        </span>
        <span className={`ml-auto w-2 h-2 rounded-full ${statusDot[data.status]}`} />
      </div>

      <p className="text-sm font-medium text-[var(--text)] truncate max-w-[180px]">
        {data.label}
      </p>
      {data.nodeType === "llm" && data.metadata?.model_id && (
        <p className="text-xs text-[var(--text-muted)] truncate max-w-[180px] mt-0.5">
          {data.metadata.model_id}
        </p>
      )}
      {data.nodeType === "mcp" && data.metadata?.mcp_server_name && (
        <p className="text-xs text-[var(--text-muted)] truncate max-w-[180px] mt-0.5">
          {data.metadata.mcp_server_name}
        </p>
      )}

      <div className="mt-2 flex gap-3 text-xs text-[var(--text-muted)]">
        {data.costUSD > 0 && (
          <span>${data.costUSD.toFixed(4)}</span>
        )}
        {data.tokenCount > 0 && (
          <span>{data.tokenCount.toLocaleString()} tok</span>
        )}
      </div>

      <Handle type="source" position={Position.Bottom} className="!bg-[var(--border)]" />
    </div>
  );
}

export const nodeTypes = {
  agentNode: AgentNode,
};
