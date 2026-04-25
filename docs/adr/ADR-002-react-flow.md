# ADR-002: React Flow for agent topology visualization

**Status:** Accepted  
**Date:** 2025-Q1

## Context

A central differentiator for AgentPulse over every current competitor is the agent topology view: a real directed graph showing which agent called which, with handoff edges, live state, and loop detection banners. The feasibility analysis notes that "for multi-agent runs this is the #1 debugging view, and nobody else renders it well" — competitors render either a flat waterfall (Langfuse, LangSmith, Phoenix) or nothing at all (Helicone).

Rendering a live-updating directed acyclic graph (DAG) in the browser is not a trivial UI problem. The graph must:

- Accept dynamic node/edge data streamed in as a run progresses (via SSE).
- Support interactive pan/zoom and node dragging so users can explore complex topologies.
- Render loop-detection annotations (cycle highlights, repeat-tool badges) without a full redraw.
- Integrate cleanly with the existing Next.js 15 / React 18 stack.
- Have an ecosystem of maintained plugins (minimap, controls, custom node types).

The underlying data is produced by the `topologyexporter` in the collector, which UPSERTs `topology_nodes` and `topology_edges` rows into Postgres keyed by `(project_id, run_id)`. The UI reads these rows via the backend REST API.

## Decision

Use **React Flow** (`@xyflow/react`) as the graph rendering library for the topology view.

React Flow is built on React primitives: nodes and edges are standard React components, which means custom node types (e.g., an LLM-call node with token count badge, a tool-call node with error state) are written exactly like any other component. The library handles pan/zoom, node dragging, edge routing, and selection out of the box. Live updates are achieved by replacing the `nodes` and `edges` arrays — React Flow diffs and re-renders incrementally without destroying graph layout state.

## Consequences

### Positive

- Custom node and edge types are plain React components — no separate templating system to learn. The `AgentNode`, `ToolNode`, and `HandoffEdge` components in `web/src/components/topology/` are each under 80 lines.
- Interactive pan/zoom and minimap come for free; no implementation cost.
- Live updates via SSE work naturally: the topology panel holds nodes/edges in React state and merges incoming events, which React Flow re-renders without a full layout recalculation.
- Strong TypeScript types throughout; integrates without friction into the Next.js 15 App Router codebase.
- Active maintenance (xyflow/react), good documentation, and a large community of examples for DAG-style agent graphs specifically.
- Loop detection banners (`LoopBanner`) attach as React siblings to the topology panel — they read from the same `run_loops` data and render without touching the graph library internals.

### Negative / Trade-offs

- React Flow carries a meaningful bundle weight (~150 KB gzipped with dependencies). For a full-stack observability dashboard this is acceptable; it would be a concern for a lightweight embed.
- Very large graphs (hundreds of nodes) can have layout performance issues. At the scales AgentPulse targets (tens of agents per run), this is not a current concern.
- The library is not rendering-engine-agnostic — switching away from it in the future would require rewriting the topology panel.

## Alternatives Considered

**D3.js (force-directed or Dagre layout).** Maximum flexibility, but requires writing all interaction (pan, zoom, drag, selection) and all React integration from scratch. The engineering cost is high and the result would be bespoke code that new contributors need to understand. D3's imperative mutation model also fights React's declarative rendering, requiring careful ref-based management.

**Cytoscape.js.** Mature graph library with many layout algorithms. However, Cytoscape renders to a `<canvas>` or DOM elements outside React's control, making custom node templates (with React components, tooltips, badges) awkward. Less commonly used in React codebases; the xyflow ecosystem is a better fit for this stack.

**Mermaid (static diagrams).** Cannot support interactive pan/zoom, live updates, or custom node rendering. Suitable for documentation; not suitable for a live debugging view where the graph changes as a run progresses.

**vis.js Network.** Renders to a canvas element outside React, similar downsides to Cytoscape. Development activity has slowed; xyflow has overtaken it as the React-native choice.
