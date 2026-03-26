/**
 * Span kind wrapper functions for the 5 AgentPulse span types.
 *
 * Each span kind has a callback form: llmCall(options, fn)
 * The optional runId parameter is the edge-runtime escape hatch when
 * AsyncLocalStorage is unavailable.
 */

import { trace, SpanStatusCode, type Span, type Tracer } from '@opentelemetry/api'
import * as attrs from './generated/attributes.js'
import { getRunId, getProjectId, getSessionId, getUserId } from './context.js'

// Allow tests to override the tracer
let _tracerOverride: Tracer | undefined

/** @internal — for testing only */
export function _setTracerOverride(t: Tracer | undefined): void {
  _tracerOverride = t
}

function getTracer() {
  return _tracerOverride ?? trace.getTracer('agentpulse')
}

function setCommonAttrs(
  span: Span,
  spanKind: attrs.AgentSpanKind,
  agentName?: string,
  runId?: string,
): void {
  span.setAttribute(attrs.SPAN_KIND, spanKind)
  span.setAttribute(attrs.RUN_ID, getRunId(runId))
  const projectId = getProjectId()
  if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)
  const sessionId = getSessionId()
  if (sessionId) span.setAttribute(attrs.SESSION_ID, sessionId)
  const userId = getUserId()
  if (userId) span.setAttribute(attrs.USER_ID, userId)
  if (agentName) span.setAttribute(attrs.AGENT_NAME, agentName)
}

async function withSpan<T>(
  spanName: string,
  spanKind: attrs.AgentSpanKind,
  setExtra: (span: Span) => void,
  fn: (span: Span) => T | Promise<T>,
  agentName?: string,
  runId?: string,
): Promise<T> {
  return getTracer().startActiveSpan(spanName, async (span) => {
    setCommonAttrs(span, spanKind, agentName, runId)
    setExtra(span)
    try {
      const result = await fn(span)
      return result
    } catch (err) {
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
      throw err
    } finally {
      span.end()
    }
  })
}

// ── LLM call ──────────────────────────────────────────────────────────────────

export interface LlmCallOptions {
  model: string
  agentName?: string
  spanName?: string
  runId?: string
}

export function llmCall<T>(
  options: LlmCallOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? `llm.${options.model}`
  return withSpan(
    name,
    attrs.LLM_CALL,
    (span) => span.setAttribute(attrs.MODEL_ID, options.model),
    fn,
    options.agentName,
    options.runId,
  )
}

// ── Tool call ─────────────────────────────────────────────────────────────────

export interface ToolCallOptions {
  toolName: string
  agentName?: string
  spanName?: string
  runId?: string
}

export function toolCall<T>(
  options: ToolCallOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? `tool.${options.toolName}`
  return withSpan(
    name,
    attrs.TOOL_CALL,
    (span) => span.setAttribute(attrs.TOOL_NAME, options.toolName),
    fn,
    options.agentName,
    options.runId,
  )
}

// ── Agent handoff ─────────────────────────────────────────────────────────────

export interface HandoffOptions {
  targetAgent: string
  agentName?: string
  spanName?: string
  runId?: string
}

export function handoff<T>(
  options: HandoffOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? 'agent.handoff'
  return withSpan(
    name,
    attrs.AGENT_HANDOFF,
    (span) => span.setAttribute(attrs.HANDOFF_TARGET, options.targetAgent),
    fn,
    options.agentName,
    options.runId,
  )
}

// ── Memory read ───────────────────────────────────────────────────────────────

export interface MemoryReadOptions {
  key?: string
  agentName?: string
  spanName?: string
  runId?: string
}

export function memoryRead<T>(
  options: MemoryReadOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? 'memory.read'
  return withSpan(
    name,
    attrs.MEMORY_READ,
    (span) => { if (options.key) span.setAttribute(attrs.MEMORY_KEY, options.key) },
    fn,
    options.agentName,
    options.runId,
  )
}

// ── Memory write ──────────────────────────────────────────────────────────────

export interface MemoryWriteOptions {
  key?: string
  agentName?: string
  spanName?: string
  runId?: string
}

export function memoryWrite<T>(
  options: MemoryWriteOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? 'memory.write'
  return withSpan(
    name,
    attrs.MEMORY_WRITE,
    (span) => { if (options.key) span.setAttribute(attrs.MEMORY_KEY, options.key) },
    fn,
    options.agentName,
    options.runId,
  )
}

// ── MCP tool call ─────────────────────────────────────────────────────────────

export interface McpToolCallOptions {
  serverName: string
  toolName: string
  agentName?: string
  spanName?: string
  runId?: string
}

/** Wrap an MCP tool invocation in an mcp.tool_call span.
 *
 * Also sets tool.name so existing tool analytics capture MCP tool calls.
 */
export function mcpToolCall<T>(
  options: McpToolCallOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? `mcp.${options.toolName}`
  return withSpan(
    name,
    attrs.MCP_TOOL_CALL,
    (span) => {
      span.setAttribute(attrs.MCP_SERVER_NAME, options.serverName)
      span.setAttribute(attrs.MCP_TOOL_NAME, options.toolName)
      span.setAttribute(attrs.TOOL_NAME, options.toolName)
    },
    fn,
    options.agentName,
    options.runId,
  )
}

// ── MCP list tools ─────────────────────────────────────────────────────────────

export interface McpListToolsOptions {
  serverName: string
  agentName?: string
  spanName?: string
  runId?: string
}

/** Wrap an MCP tool discovery call in an mcp.list_tools span.
 *
 * Use recordMcpDiscovery() inside fn to attach discovered tool names.
 */
export function mcpListTools<T>(
  options: McpListToolsOptions,
  fn: (span: Span) => T | Promise<T>,
): Promise<T> {
  const name = options.spanName ?? 'mcp.list_tools'
  return withSpan(
    name,
    attrs.MCP_LIST_TOOLS,
    (span) => span.setAttribute(attrs.MCP_SERVER_NAME, options.serverName),
    fn,
    options.agentName,
    options.runId,
  )
}

// ── MCP recording helpers ─────────────────────────────────────────────────────

export interface McpToolResultOptions {
  inputSchema?: string
  outputSchema?: string
  toolInput?: string
  toolOutput?: string
}

/** Attach MCP tool call data to an active span. */
export function recordMcpToolResult(span: Span, opts: McpToolResultOptions): void {
  if (opts.inputSchema != null) span.setAttribute(attrs.MCP_INPUT_SCHEMA, opts.inputSchema)
  if (opts.outputSchema != null) span.setAttribute(attrs.MCP_OUTPUT_SCHEMA, opts.outputSchema)
  if (opts.toolInput != null) span.setAttribute('tool.input', opts.toolInput)
  if (opts.toolOutput != null) span.setAttribute('tool.output', opts.toolOutput)
}

export interface McpDiscoveryOptions {
  toolCount: number
  discoveredTools: string[]
}

/** Attach tool discovery data to an mcp.list_tools span. */
export function recordMcpDiscovery(span: Span, opts: McpDiscoveryOptions): void {
  span.setAttribute(attrs.MCP_TOOL_COUNT, String(opts.toolCount))
  if (opts.discoveredTools.length > 0) {
    span.setAttribute(attrs.MCP_DISCOVERED_TOOLS, opts.discoveredTools.join(','))
  }
}
