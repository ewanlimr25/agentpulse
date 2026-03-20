"use client";

import { useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  BackgroundVariant,
  MarkerType,
} from "@xyflow/react";
import type { Node, Edge } from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import type { Topology } from "@/lib/types";
import { nodeTypes } from "./nodeTypes";
import { applyDagreLayout } from "./layout";

interface Props {
  topology: Topology;
}

const edgeStyle: Record<string, { stroke: string; label: string }> = {
  invocation: { stroke: "#6366f1", label: "calls" },
  handoff: { stroke: "#22c55e", label: "handoff" },
  memory_access: { stroke: "#f59e0b", label: "memory" },
};

export function TopologyGraph({ topology }: Props) {
  const { nodes, edges } = useMemo(() => {
    const rawNodes: Node[] = (topology.Nodes ?? []).map((n) => ({
      id: n.ID,
      type: "agentNode",
      position: { x: 0, y: 0 }, // overwritten by layout
      data: {
        label: n.NodeName,
        nodeType: n.NodeType,
        status: n.Status,
        costUSD: n.CostUSD,
        tokenCount: n.TokenCount,
      },
    }));

    const rawEdges: Edge[] = (topology.Edges ?? []).map((e) => {
      const style = edgeStyle[e.EdgeType] ?? { stroke: "#4b5563", label: "" };
      return {
        id: e.ID,
        source: e.SourceNodeID,
        target: e.TargetNodeID,
        label: style.label,
        animated: e.EdgeType === "handoff",
        style: { stroke: style.stroke, strokeWidth: 2 },
        markerEnd: { type: MarkerType.ArrowClosed, color: style.stroke },
        labelStyle: { fill: "#94a3b8", fontSize: 11 },
        labelBgStyle: { fill: "#1a1d27" },
      };
    });

    return applyDagreLayout(rawNodes, rawEdges);
  }, [topology]);

  if (!topology.Nodes?.length) {
    return (
      <div className="flex items-center justify-center h-full text-[var(--text-muted)]">
        No topology data for this run.
      </div>
    );
  }

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.2 }}
      nodesDraggable
      nodesConnectable={false}
      elementsSelectable
    >
      <Background variant={BackgroundVariant.Dots} gap={24} size={1} color="#2d3148" />
      <Controls className="!bg-[var(--surface)] !border-[var(--border)]" />
      <MiniMap
        className="!bg-[var(--surface)] !border-[var(--border)]"
        nodeColor={(n) => {
          const type = (n.data as { nodeType: string }).nodeType;
          const map: Record<string, string> = {
            agent: "#6366f1",
            llm: "#a855f7",
            tool: "#38bdf8",
            memory: "#f59e0b",
          };
          return map[type] ?? "#4b5563";
        }}
      />
    </ReactFlow>
  );
}
