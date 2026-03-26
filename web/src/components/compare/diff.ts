import type { TopologyNode, TopologyEdge } from "@/lib/types";

export type DiffStatus = "shared" | "only-a" | "only-b";

/**
 * Returns a Map<nodeId, DiffStatus> by matching nodes on (NodeName, NodeType).
 * Nodes that exist in both topologies are "shared"; nodes unique to A are
 * "only-a"; nodes unique to B are "only-b".
 */
export function diffNodes(
  nodesA: TopologyNode[],
  nodesB: TopologyNode[]
): Map<string, DiffStatus> {
  const result = new Map<string, DiffStatus>();

  // Build a key set from B for O(1) look-up
  const bKeys = new Set<string>(
    nodesB.map((n) => `${n.NodeName}::${n.NodeType}`)
  );
  const aKeys = new Set<string>(
    nodesA.map((n) => `${n.NodeName}::${n.NodeType}`)
  );

  for (const n of nodesA) {
    const key = `${n.NodeName}::${n.NodeType}`;
    result.set(n.ID, bKeys.has(key) ? "shared" : "only-a");
  }

  for (const n of nodesB) {
    const key = `${n.NodeName}::${n.NodeType}`;
    result.set(n.ID, aKeys.has(key) ? "shared" : "only-b");
  }

  return result;
}

/**
 * Returns a Map<edgeId, DiffStatus> by matching edges on
 * (sourceNodeName, targetNodeName, EdgeType).
 *
 * Because the edge IDs differ between runs we resolve node names first by
 * building an id→name index from the supplied node arrays.
 */
export function diffEdges(
  edgesA: TopologyEdge[],
  edgesB: TopologyEdge[],
  nodesA: TopologyNode[],
  nodesB: TopologyNode[]
): Map<string, DiffStatus> {
  const result = new Map<string, DiffStatus>();

  const nameA = new Map<string, string>(nodesA.map((n) => [n.ID, n.NodeName]));
  const nameB = new Map<string, string>(nodesB.map((n) => [n.ID, n.NodeName]));

  const edgeKey = (
    edge: TopologyEdge,
    nameMap: Map<string, string>
  ): string => {
    const src = nameMap.get(edge.SourceNodeID) ?? edge.SourceNodeID;
    const tgt = nameMap.get(edge.TargetNodeID) ?? edge.TargetNodeID;
    return `${src}->${tgt}::${edge.EdgeType}`;
  };

  const bEdgeKeys = new Set<string>(edgesB.map((e) => edgeKey(e, nameB)));
  const aEdgeKeys = new Set<string>(edgesA.map((e) => edgeKey(e, nameA)));

  for (const e of edgesA) {
    const key = edgeKey(e, nameA);
    result.set(e.ID, bEdgeKeys.has(key) ? "shared" : "only-a");
  }

  for (const e of edgesB) {
    const key = edgeKey(e, nameB);
    result.set(e.ID, aEdgeKeys.has(key) ? "shared" : "only-b");
  }

  return result;
}
