// Shared TypeScript types mirroring the backend domain models.

export type AgentSpanKind =
  | "llm.call"
  | "tool.call"
  | "agent.handoff"
  | "memory.read"
  | "memory.write"
  | "unknown";

export interface Project {
  ID: string;
  Name: string;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface Run {
  RunID: string;
  ProjectID: string;
  TraceID: string;
  StartTime: string;
  EndTime: string;
  DurationMS: number;
  SpanCount: number;
  LLMCallCount: number;
  ToolCallCount: number;
  TotalInputTokens: number;
  TotalOutputTokens: number;
  TotalTokens: number;
  TotalCostUSD: number;
  ErrorCount: number;
  Status: "ok" | "error";
}

export interface SpanEvent {
  Name: string;
  Timestamp: string;
  Attributes: Record<string, string>;
}

export interface Span {
  TraceID: string;
  SpanID: string;
  ParentSpanID: string;
  RunID: string;
  ProjectID: string;
  AgentSpanKind: AgentSpanKind;
  AgentName: string;
  ModelID: string;
  SpanName: string;
  ServiceName: string;
  StatusCode: string;
  StatusMessage: string;
  StartTime: string;
  EndTime: string;
  DurationNS: number;
  InputTokens: number;
  OutputTokens: number;
  TotalTokens: number;
  CostUSD: number;
  Attributes: Record<string, string>;
  ResourceAttrs: Record<string, string>;
  Events: SpanEvent[];
}

export type NodeType = "agent" | "tool" | "llm" | "memory";
export type NodeStatus = "ok" | "error" | "running" | "unset";
export type EdgeType = "invocation" | "handoff" | "memory_access";

export interface TopologyNode {
  ID: string;
  ProjectID: string;
  RunID: string;
  TraceID: string;
  SpanID: string;
  NodeType: NodeType;
  NodeName: string;
  Status: NodeStatus;
  StartTime: string | null;
  EndTime: string | null;
  CostUSD: number;
  TokenCount: number;
  Metadata: Record<string, unknown>;
}

export interface TopologyEdge {
  ID: string;
  ProjectID: string;
  RunID: string;
  SourceNodeID: string;
  TargetNodeID: string;
  EdgeType: EdgeType;
  Metadata: Record<string, unknown>;
}

export interface Topology {
  RunID: string;
  Nodes: TopologyNode[];
  Edges: TopologyEdge[];
}

export interface RunsListResponse {
  runs: Run[];
  total: number;
  limit: number;
  offset: number;
}

export interface BudgetRule {
  ID: string;
  ProjectID: string;
  Name: string;
  ThresholdUSD: number;
  Action: "notify" | "halt";
  Scope: "run" | "agent" | "window";
  WindowSeconds?: number;
  WebhookURL?: string;
  Enabled: boolean;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface BudgetAlert {
  ID: string;
  RuleID: string;
  ProjectID: string;
  RunID?: string;
  TriggeredAt: string;
  CurrentCost: number;
  ThresholdUSD: number;
  ActionTaken: string;
}

// RecentBudgetAlert is a cross-project alert enriched with project and rule names.
export interface RecentBudgetAlert extends BudgetAlert {
  ProjectName: string;
  RuleName: string;
}

export interface SpanEval {
  ProjectID: string;
  RunID: string;
  SpanID: string;
  EvalName: string;
  Score: number;      // 0.0 – 1.0
  Reasoning: string;
  JudgeModel: string;
  EvalVersion: number;
  CreatedAt: string;
}

export interface RunEvalSummary {
  RunID: string;
  AvgScore: number;   // 0.0 – 1.0
  SpanCount: number;
}

// WsAlertEvent is the real-time alert pushed over WebSocket by the backend hub.
// Field names match the Go alert.Event JSON serialisation (snake_case).
export interface WsAlertEvent {
  type: string;        // "budget.alert" | "signal.alert"
  project_id: string;
  run_id?: string;
  rule_id: string;
  rule_name: string;
  action: string;      // "notify" | "halt"
  // Budget-specific
  cost_usd?: number;
  limit_usd?: number;
  // Signal-specific
  signal_type?: SignalType;
  current_value?: number;
  threshold?: number;
}

// ── Multi-signal alerting ─────────────────────────────────────────────────────

export type SignalType = "error_rate" | "latency_p95" | "quality_score" | "tool_failure";
export type CompareOp = "gt" | "lt";

export interface AlertRule {
  ID: string;
  ProjectID: string;
  Name: string;
  SignalType: SignalType;
  Threshold: number;
  CompareOp: CompareOp;
  WindowSeconds: number;
  ScopeFilter?: string;
  WebhookURL?: string;
  Enabled: boolean;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface AlertEvent {
  ID: string;
  RuleID: string;
  ProjectID: string;
  TriggeredAt: string;
  SignalType: SignalType;
  CurrentValue: number;
  Threshold: number;
  CompareOp: CompareOp;
  ActionTaken: string;
  Metadata: Record<string, unknown>;
}

export interface RecentAlertEvent extends AlertEvent {
  ProjectName: string;
  RuleName: string;
}
