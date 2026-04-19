// Shared TypeScript types mirroring the backend domain models.

export type AgentSpanKind =
  | "llm.call"
  | "tool.call"
  | "agent.handoff"
  | "memory.read"
  | "memory.write"
  | "mcp.tool_call"
  | "mcp.list_tools"
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
  LoopDetected?: boolean;
  SessionID?: string;
  UserID?: string;
  TtftP50Ms?: number;
  TtftP95Ms?: number;
  StreamingSpanCount?: number;
  IsActive?: boolean;
  tags?: string[];
  annotation?: string | null;
}

export interface RunAnnotation {
  id: string;
  projectId: string;
  runId: string;
  note: string;
  createdAt: string;
  updatedAt: string;
}

export interface Session {
  SessionID: string;
  ProjectID: string;
  RunCount: number;
  TotalCostUSD: number;
  TotalTokens: number;
  InputTokens: number;
  OutputTokens: number;
  ErrorCount: number;
  FirstRunAt: string;
  LastRunAt: string;
}

export interface SessionsListResponse {
  sessions: Session[];
  total: number;
  limit: number;
  offset: number;
}

export interface UserStats {
  UserID: string;
  ProjectID: string;
  RunCount: number;
  TotalCostUSD: number;
  TotalTokens: number;
  InputTokens: number;
  OutputTokens: number;
  ErrorCount: number;
  CostPercent: number;
  FirstSeenAt: string;
  LastSeenAt: string;
}

export interface UsersListResponse {
  users: UserStats[];
  total: number;
  limit: number;
  offset: number;
}

export interface RunLoop {
  ID: string;
  RunID: string;
  ProjectID: string;
  DetectionType: "repeated_tool_call" | "topology_cycle";
  SpanName: string;
  InputHash: string;
  OutputHash: string;
  Confidence: "high" | "low";
  OccurrenceCount: number;
  DetectedAt: string;
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
  TtftMs?: number;
}

export type NodeType = "agent" | "tool" | "llm" | "memory" | "mcp";
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
  Scope: "run" | "agent" | "window" | "user";
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
  EvalName: string;   // e.g. "relevance", "hallucination"
  AvgScore: number;   // 0.0 – 1.0
  SpanCount: number;
}

export interface EvalConfig {
  ID: string;
  ProjectID: string;
  EvalName: string;           // built-in name or "custom:<name>"
  Enabled: boolean;
  SpanKind: "llm.call" | "tool.call";
  PromptTemplate?: string;    // undefined = built-in; present = custom
  PromptVersion: number;
  ScopeFilter?: Record<string, string[]>; // e.g. { agent_name: ["researcher"] }; absent = all agents
  JudgeModels?: string[];     // e.g. ["claude-haiku-4-5", "gpt-4o-mini"]; absent = default single model
  CreatedAt: string;
  UpdatedAt: string;
}

export interface ModelScore {
  Model: string;
  Score: number;
  Reasoning: string;
}

export interface SpanEvalGroup {
  SpanID: string;
  EvalName: string;
  Scores: ModelScore[];
  ConsensusScore: number | null;  // null if not all models scored
  Disagreement: boolean;
}

export const BUILTIN_EVAL_NAMES = ["relevance", "hallucination", "faithfulness", "toxicity", "tool_correctness"] as const;
export type BuiltinEvalName = typeof BUILTIN_EVAL_NAMES[number];

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

export type SignalType = "error_rate" | "latency_p95" | "quality_score" | "tool_failure" | "agent_loop";
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

// ── Replay ────────────────────────────────────────────────────────────────────

export interface ReplaySpan {
  SpanID: string;
  ParentSpanID: string;
  AgentSpanKind: string;
  AgentName: string;
  SpanName: string;
  ModelID: string;
  ToolName: string;
  CallIndex: number;
  StatusCode: string;
  StatusMessage: string;
  Inputs: Record<string, string>;
  Outputs: Record<string, string>;
  InputTokens: number;
  OutputTokens: number;
}

export interface ReplayBundle {
  SchemaVersion: number;
  Run: Run;
  Topology: Topology;
  Spans: ReplaySpan[];
}

// ── Run Comparison ────────────────────────────────────────────────────────────

export interface PromptFieldDiff {
  field_name: string;
  a: string;
  b: string;
  changed: boolean;
}

export interface ModelParamDiff {
  param_name: string;
  a: string;
  b: string;
  changed: boolean;
}

export interface SpanPromptDiff {
  span_key: string;
  agent_name: string;
  span_name: string;
  call_index: number;
  status: 'changed' | 'only-a' | 'only-b' | 'unchanged';
  prompt_diffs: PromptFieldDiff[];
  param_diffs: ModelParamDiff[];
}

export interface RunPromptDiff {
  run_id_a: string;
  run_id_b: string;
  spans: SpanPromptDiff[];
  unchanged_count: number;
  truncated: boolean;
}

export interface RunComparison {
  RunA: Run;
  RunB: Run;
  TopologyA: Topology | null;
  TopologyB: Topology | null;
  EvalsA: SpanEval[];
  EvalsB: SpanEval[];
}

// ── Search ────────────────────────────────────────────────────────────────────

export interface SearchResult {
  TraceID: string;
  SpanID: string;
  RunID: string;
  ProjectID: string;
  SpanName: string;
  AgentSpanKind: string;
  AgentName: string;
  ModelID: string;
  StatusCode: string;
  StartTime: string;
  DurationNS: number;
  InputTokens: number;
  OutputTokens: number;
  TotalTokens: number;
  CostUSD: number;
  MatchedField: string;
  Snippet: string;
}

export interface SearchResponse {
  results: SearchResult[];
  total: number;
  limit: number;
  offset: number;
  query: string;
}

// ── Health ────────────────────────────────────────────────────────────────────

export interface ProjectHealth {
  CollectorReachable: boolean;
  LastSpanAt: string | null;
  SpanCount: number;
}

// ── PII / Settings ────────────────────────────────────────────────────────────

export interface PIICustomRule {
  name: string;
  pattern: string;
  enabled: boolean;
}

export interface ProjectPIIConfig {
  project_id: string;
  pii_redaction_enabled: boolean;
  pii_custom_rules: PIICustomRule[];
  created_at: string;
  updated_at: string;
}

// ── Analytics ─────────────────────────────────────────────────────────────────

export type AnalyticsWindow = "24h" | "7d";

export interface ToolStats {
  ToolName: string;
  CallCount: number;
  ErrorCount: number;
  ErrorRate: number;      // 0–100 percentage, computed server-side
  AvgLatencyMS: number;
  P95LatencyMS: number;
  TotalCostUSD: number;
}

export interface AgentCostStats {
  AgentName: string;
  TotalCostUSD: number;
  CostPercent: number;    // 0–100, share of project total
  CallCount: number;
  AvgCostPerCall: number;
}

export interface ModelStats {
  ModelID: string;
  Provider: string;
  CallCount: number;
  ErrorCount: number;
  ErrorRate: number;          // 0–100
  AvgLatencyMS: number;
  P95LatencyMS: number;
  TotalCostUSD: number;
  AvgCostPerCall: number;
  InputTokens: number;
  OutputTokens: number;
  TotalTokens: number;
  CostPerMillionTokens: number;
}

export interface ModelPricing {
  [modelId: string]: {
    input_per_million: number;
    output_per_million: number;
  };
}

export interface ModelStatsResponse {
  models: ModelStats[];
  window: string;
  pricing: ModelPricing;
}

// ── Human-in-the-Loop Eval Feedback ──────────────────────────────────────────

export interface SpanFeedback {
  ID: string;
  ProjectID: string;
  SpanID: string;
  RunID: string;
  Rating: "good" | "bad";
  CorrectedOutput: string | null;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface FeedbackRequest {
  run_id: string;
  rating: "good" | "bad";
  corrected_output?: string;
}

// ── Prompt Playground ────────────────────────────────────────────────────────

export interface PlaygroundMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

export interface PlaygroundExecution {
  ID: string;
  VariantID: string;
  Output: string | null;
  InputTokens: number;
  OutputTokens: number;
  CostUSD: number;
  LatencyMS: number;
  Error: string | null;
  CreatedAt: string;
}

export interface PlaygroundVariant {
  ID: string;
  SessionID: string;
  Label: string;
  ModelID: string;
  System: string;
  Messages: PlaygroundMessage[];
  Temperature: number | null;
  MaxTokens: number | null;
  Executions: PlaygroundExecution[] | null;
  UpdatedAt: string;
}

export interface PlaygroundSession {
  ID: string;
  ProjectID: string;
  Name: string;
  SourceSpanID: string | null;
  SourceRunID: string | null;
  Variants: PlaygroundVariant[] | null;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface PlaygroundSessionsListResponse {
  sessions: PlaygroundSession[];
  total: number;
  limit: number;
  offset: number;
}

export interface ModelInfo {
  model_id: string;
  provider: string;
  input_per_million: number;
  output_per_million: number;
  available: boolean;
}
