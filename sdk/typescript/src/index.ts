export { loadConfig, resolvedEndpoint } from './config.js'
export type { AgentPulseConfig } from './config.js'

export { initTracer, shutdown } from './tracer.js'

export {
  setRunId, getRunId, resetRun,
  setProjectId, getProjectId,
  setSessionId, getSessionId, generateSessionId, resetSession,
  setUserId, getUserId, resetUser,
} from './context.js'

export {
  llmCall,
  toolCall,
  handoff,
  memoryRead,
  memoryWrite,
  mcpToolCall,
  mcpListTools,
  recordMcpToolResult,
  recordMcpDiscovery,
} from './spans.js'
export type {
  LlmCallOptions,
  ToolCallOptions,
  HandoffOptions,
  MemoryReadOptions,
  MemoryWriteOptions,
  McpToolCallOptions,
  McpListToolsOptions,
  McpToolResultOptions,
  McpDiscoveryOptions,
} from './spans.js'

export { recordLlmUsage } from './usage.js'
export type { LlmUsage } from './usage.js'

export { recordStreamFirstToken } from './streaming.js'

export * as replay from './replay.js'
export { ReplayEngine, loadBundle, withReplay } from './replay.js'
export type { ReplayBundle, ReplaySpan, LoadBundleOptions, Overrides } from './replay.js'

export * as attributes from './generated/attributes.js'
export type { AgentSpanKind } from './generated/attributes.js'
