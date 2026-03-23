export { loadConfig, resolvedEndpoint } from './config.js'
export type { AgentPulseConfig } from './config.js'

export { initTracer, shutdown } from './tracer.js'

export {
  setRunId, getRunId, resetRun,
  setProjectId, getProjectId,
  setSessionId, getSessionId, generateSessionId, resetSession,
} from './context.js'

export {
  llmCall,
  toolCall,
  handoff,
  memoryRead,
  memoryWrite,
} from './spans.js'
export type {
  LlmCallOptions,
  ToolCallOptions,
  HandoffOptions,
  MemoryReadOptions,
  MemoryWriteOptions,
} from './spans.js'

export { recordLlmUsage } from './usage.js'
export type { LlmUsage } from './usage.js'

export * as attributes from './generated/attributes.js'
export type { AgentSpanKind } from './generated/attributes.js'
