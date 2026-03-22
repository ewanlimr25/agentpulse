/**
 * Span kind wrapper functions for the 5 AgentPulse span types.
 *
 * Each span kind has a callback form: llmCall(options, fn)
 * The optional runId parameter is the edge-runtime escape hatch when
 * AsyncLocalStorage is unavailable.
 */

import { trace, SpanStatusCode, type Span, type Tracer } from '@opentelemetry/api'
import * as attrs from './generated/attributes.js'
import { getRunId, getProjectId } from './context.js'

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
