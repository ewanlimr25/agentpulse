"use client";

import { useMemo } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  BackgroundVariant,
  MarkerType,
} from "@xyflow/react";
import type { Node, Edge } from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import type { Topology } from "@/lib/types";
import { nodeTypes } from "@/components/topology/nodeTypes";
import { applyDagreLayout } from "@/components/topology/layout";
import { diffNodes, diffEdges, type DiffStatus } from "./diff";

const edgeStyle: Record<string, { stroke: string; label: string }> = {
  invocation: { stroke: "#6366f1", label: "calls" },
  handoff: { stroke: "#22c55e", label: "handoff" },
  memory_access: { stroke: "#f59e0b", label: "memory" },
};

const diffRingClass: Record<DiffStatus, string> = {
  shared: "",
  "only-a": "ring-2 ring-blue-400",
  "only-b": "ring-2 ring-orange-400",
};

interface PanelProps {
  topology: Topology;
  label: string;
  diffStatusMap: Map<string, DiffStatus>;
  edgeDiffMap: Map<string, DiffStatus>;
}

function TopologyPanel({ topology, label, diffStatusMap, edgeDiffMap }: PanelProps) {
  const { nodes, edges } = useMemo(() => {
    const rawNodes: Node[] = (topology.Nodes ?? []).map((n) => {
      const status = diffStatusMap.get(n.ID) ?? "shared";
      return {
        id: n.ID,
        type: "agentNode",
        position: { x: 0, y: 0 },
        data: {
          label: n.NodeName,
          nodeType: n.NodeType,
          status: n.Status,
          costUSD: n.CostUSD,
          tokenCount: n.TokenCount,
          metadata: n.Metadata as Record<string, string>,
          diffStatus: status,
        },
        className: diffRingClass[status],
      };
    });

    const rawEdges: Edge[] = (topology.Edges ?? []).map((e) => {
      const style = edgeStyle[e.EdgeType] ?? { stroke: "#4b5563", label: "" };
      const diff = edgeDiffMap.get(e.ID) ?? "shared";
      const opacity = diff === "shared" ? 1 : 0.85;
      return {
        id: e.ID,
        source: e.SourceNodeID,
        target: e.TargetNodeID,
        label: style.label,
        animated: e.EdgeType === "handoff",
        style: { stroke: style.stroke, strokeWidth: 2, opacity },
        markerEnd: { type: MarkerType.ArrowClosed, color: style.stroke },
        labelStyle: { fill: "#94a3b8", fontSize: 11 },
        labelBgStyle: { fill: "#1a1d27" },
      };
    });

    return applyDagreLayout(rawNodes, rawEdges);
  }, [topology, diffStatusMap, edgeDiffMap]);

  if (!topology.Nodes?.length) {
    return (
      <div className="flex-1 flex items-center justify-center text-[var(--text-muted)] min-h-[300px]">
        No topology data
      </div>
    );
  }

  return (
    <div className="flex-1 min-w-0">
      <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)] mb-2 px-1">
        {label}
      </p>
      <div className="h-[400px] rounded-xl border border-[var(--border)] overflow-hidden">
        <ReactFlowProvider>
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
          </ReactFlow>
        </ReactFlowProvider>
      </div>
    </div>
  );
}

interface Props {
  topologyA: Topology | null;
  topologyB: Topology | null;
}

export function ComparisonTopology({ topologyA, topologyB }: Props) {
  const nodesA = topologyA?.Nodes ?? [];
  const nodesB = topologyB?.Nodes ?? [];
  const edgesA = topologyA?.Edges ?? [];
  const edgesB = topologyB?.Edges ?? [];

  const nodeDiffMap = useMemo(() => diffNodes(nodesA, nodesB), [nodesA, nodesB]);
  const edgeDiffMap = useMemo(
    () => diffEdges(edgesA, edgesB, nodesA, nodesB),
    [edgesA, edgesB, nodesA, nodesB]
  );

  if (!topologyA && !topologyB) {
    return (
      <div className="text-[var(--text-muted)] text-sm">
        No topology data available for either run.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Legend */}
      <div className="flex items-center gap-4 text-xs text-[var(--text-muted)]">
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-full bg-zinc-500 inline-block" />
          Shared
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-full bg-blue-400 inline-block" />
          Only in A
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-3 h-3 rounded-full bg-orange-400 inline-block" />
          Only in B
        </span>
      </div>

      {/* Two panels - side by side on wide screens, stacked on narrow */}
      <div className="flex flex-col min-[1400px]:flex-row gap-4">
        {topologyA ? (
          <TopologyPanel
            topology={topologyA}
            label="Run A Topology"
            diffStatusMap={nodeDiffMap}
            edgeDiffMap={edgeDiffMap}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center min-h-[300px] rounded-xl border border-[var(--border)] text-[var(--text-muted)] text-sm">
            No topology for Run A
          </div>
        )}
        {topologyB ? (
          <TopologyPanel
            topology={topologyB}
            label="Run B Topology"
            diffStatusMap={nodeDiffMap}
            edgeDiffMap={edgeDiffMap}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center min-h-[300px] rounded-xl border border-[var(--border)] text-[var(--text-muted)] text-sm">
            No topology for Run B
          </div>
        )}
      </div>
    </div>
  );
}
