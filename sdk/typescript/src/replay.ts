/**
 * Agent Replay / Sandbox Debugging — TypeScript SDK side.
 *
 * Loads a replay bundle (from a backend run, or a local JSON file) and
 * intercepts the SDK span wrappers to return recorded outputs in place of
 * real LLM/tool calls. See docs/roadmap.md §H Component 2.
 */

import { promises as fs } from 'node:fs'
import { trace, type Span } from '@opentelemetry/api'
import * as attrs from './generated/attributes.js'
import { __setReplayHook, type ReplayHook } from './spans.js'

// ── Wire format ───────────────────────────────────────────────────────────────

export interface ReplaySpan {
  SpanID: string
  ParentSpanID?: string
  AgentSpanKind: string
  AgentName: string
  SpanName: string
  ModelID?: string
  ToolName?: string
  CallIndex: number
  StatusCode?: string
  StatusMessage?: string
  Inputs?: Record<string, string>
  Outputs?: Record<string, string>
  InputTokens?: number
  OutputTokens?: number
}

export interface ReplayBundle {
  SchemaVersion: number
  Run: Record<string, unknown>
  Topology: Record<string, unknown>
  Spans: ReplaySpan[]
}

interface BundleEnvelope {
  data?: Partial<ReplayBundle>
}

function normalizeBundle(raw: unknown): ReplayBundle {
  const obj = raw as BundleEnvelope & Partial<ReplayBundle>
  const inner = (obj && typeof obj === 'object' && 'data' in obj && obj.data ? obj.data : obj) as Partial<ReplayBundle>
  return {
    SchemaVersion: inner.SchemaVersion ?? 1,
    Run: (inner.Run as Record<string, unknown>) ?? {},
    Topology: (inner.Topology as Record<string, unknown>) ?? {},
    Spans: (inner.Spans ?? []).map((s) => ({
      SpanID: s.SpanID ?? '',
      ParentSpanID: s.ParentSpanID ?? '',
      AgentSpanKind: s.AgentSpanKind ?? '',
      AgentName: s.AgentName ?? '',
      SpanName: s.SpanName ?? '',
      ModelID: s.ModelID ?? '',
      ToolName: s.ToolName ?? '',
      CallIndex: s.CallIndex ?? 0,
      StatusCode: s.StatusCode ?? '',
      StatusMessage: s.StatusMessage ?? '',
      Inputs: s.Inputs ?? {},
      Outputs: s.Outputs ?? {},
      InputTokens: s.InputTokens ?? 0,
      OutputTokens: s.OutputTokens ?? 0,
    })),
  }
}

// ── Loader ────────────────────────────────────────────────────────────────────

export interface LoadBundleOptions {
  apiUrl?: string
  apiKey?: string
}

/**
 * Load a replay bundle from a JSON file or the backend.
 *
 * If `source` is a path that exists or ends in `.json`, the file is read.
 * Otherwise it is treated as a run id and fetched from the backend.
 */
export async function loadBundle(
  source: string,
  opts: LoadBundleOptions = {},
): Promise<ReplayBundle> {
  const looksLikePath = source.endsWith('.json') || source.startsWith('/') || source.startsWith('./')
  if (looksLikePath) {
    const text = await fs.readFile(source, 'utf-8')
    return normalizeBundle(JSON.parse(text))
  }

  const baseUrl = opts.apiUrl ?? process.env.AGENTPULSE_API_URL
  const apiKey = opts.apiKey ?? process.env.AGENTPULSE_API_KEY
  if (!baseUrl) {
    throw new Error('apiUrl or AGENTPULSE_API_URL env var must be set to fetch a bundle')
  }
  const url = `${baseUrl.replace(/\/$/, '')}/api/v1/runs/${source}/replay-bundle`
  const headers: Record<string, string> = {}
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`
  const resp = await fetch(url, { headers })
  if (!resp.ok) {
    throw new Error(`replay bundle fetch failed: ${resp.status} ${resp.statusText}`)
  }
  const json = (await resp.json()) as unknown
  return normalizeBundle(json)
}

// ── Replay engine ─────────────────────────────────────────────────────────────

type SpanKey = string
function key(agentName: string, spanName: string, callIndex: number): SpanKey {
  return `${agentName}\u0000${spanName}\u0000${callIndex}`
}

export type Overrides = Record<string, unknown>

export class ReplayEngine {
  readonly bundle: ReplayBundle
  readonly overrides: Overrides
  private readonly index: Map<SpanKey, ReplaySpan>
  private readonly callCounts: Map<string, number>
  private active = false

  constructor(bundle: ReplayBundle, overrides: Overrides = {}) {
    this.bundle = bundle
    this.overrides = { ...overrides }
    this.index = new Map()
    this.callCounts = new Map()
    for (const s of bundle.Spans) {
      this.index.set(key(s.AgentName, s.SpanName, s.CallIndex), s)
    }
  }

  private nextCallIndex(agentName: string, spanName: string): number {
    const k = `${agentName}\u0000${spanName}`
    const idx = this.callCounts.get(k) ?? 0
    this.callCounts.set(k, idx + 1)
    return idx
  }

  lookup(agentName: string, spanName: string, callIndex: number): ReplaySpan | undefined {
    return this.index.get(key(agentName, spanName, callIndex))
  }

  /**
   * Apply replay matching for a single span invocation.
   *
   * Returns `{matched: true, value}` on hit, `{matched: false}` on miss.
   * As a side-effect, writes replay provenance / divergence attributes
   * onto `span` (or the active span if not supplied).
   */
  intercept(
    spanKind: string,
    agentName: string,
    spanName: string,
    actualInput: string | undefined,
    span?: Span,
  ): { matched: true; value: unknown } | { matched: false } {
    const callIndex = this.nextCallIndex(agentName, spanName)
    const recorded = this.lookup(agentName, spanName, callIndex)
    const target = span ?? trace.getActiveSpan()

    if (!recorded) {
      target?.setAttribute('agentpulse.replay.unmatched', true)
      return { matched: false }
    }

    target?.setAttribute('agentpulse.replay_source_run_id', String((this.bundle.Run as { ID?: string }).ID ?? ''))
    target?.setAttribute('agentpulse.replay_source_span_id', recorded.SpanID)

    const recordedInput = recorded.Inputs?.['gen_ai.prompt'] ?? recorded.Inputs?.['tool.input']
    if (actualInput != null && recordedInput != null && actualInput !== recordedInput) {
      target?.setAttribute('agentpulse.replay.diverged', true)
      target?.setAttribute('agentpulse.replay.input.actual', actualInput)
      target?.setAttribute('agentpulse.replay.input.recorded', recordedInput)
    }

    // Re-record token counts and payloads onto the new span.
    if (spanKind === attrs.LLM_CALL && target) {
      target.setAttribute(attrs.INPUT_TOKENS, recorded.InputTokens ?? 0)
      target.setAttribute(attrs.OUTPUT_TOKENS, recorded.OutputTokens ?? 0)
      const prompt = recorded.Inputs?.['gen_ai.prompt']
      const completion = recorded.Outputs?.['gen_ai.completion']
      if (prompt != null) target.setAttribute(attrs.PROMPT, prompt)
      if (completion != null) target.setAttribute(attrs.COMPLETION, completion)
    } else if ((spanKind === attrs.TOOL_CALL || spanKind === attrs.MCP_TOOL_CALL) && target) {
      const toolInput = recorded.Inputs?.['tool.input']
      const toolOutput = recorded.Outputs?.['tool.output']
      if (toolInput != null) target.setAttribute('tool.input', toolInput)
      if (toolOutput != null) target.setAttribute('tool.output', toolOutput)
    }

    // Override resolution: tool name first, then span name.
    if (recorded.ToolName && recorded.ToolName in this.overrides) {
      return { matched: true, value: this.overrides[recorded.ToolName] }
    }
    if (spanName in this.overrides) {
      return { matched: true, value: this.overrides[spanName] }
    }

    if (spanKind === attrs.LLM_CALL) {
      return { matched: true, value: recorded.Outputs?.['gen_ai.completion'] ?? '' }
    }
    return { matched: true, value: recorded.Outputs?.['tool.output'] ?? '' }
  }

  enter(): void {
    if (this.active) return
    this.active = true
    const hook: ReplayHook = ({ spanKind, spanName, agentName, span }) => {
      return this.intercept(spanKind, agentName ?? '', spanName, undefined, span)
    }
    __setReplayHook(hook)
  }

  exit(): void {
    if (!this.active) return
    this.active = false
    __setReplayHook(undefined)
  }
}

/** Convenience helper that mirrors the Python `with` form. */
export async function withReplay<T>(
  bundle: ReplayBundle,
  overrides: Overrides,
  fn: () => Promise<T> | T,
): Promise<T> {
  const engine = new ReplayEngine(bundle, overrides)
  engine.enter()
  try {
    return await fn()
  } finally {
    engine.exit()
  }
}
