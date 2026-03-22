/**
 * LangChain.js integration for AgentPulse.
 * Direct port of the Python AgentPulseCallbackHandler.
 *
 * Usage:
 *   import { AgentPulseCallbackHandler } from '@agentpulse/sdk/integrations/langchain'
 *   const handler = new AgentPulseCallbackHandler()
 *   chain.invoke({ input: '...' }, { callbacks: [handler] })
 */

import { trace, SpanStatusCode, type Span, type Tracer } from '@opentelemetry/api'
import * as attrs from '../generated/attributes.js'
import { getRunId, getProjectId } from '../context.js'

let _tracerOverride: Tracer | undefined
/** @internal — for testing only */
export function _setTracerOverride(t: Tracer | undefined): void { _tracerOverride = t }
function getTracer(name: string) { return _tracerOverride ?? trace.getTracer(name) }

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let BaseCallbackHandlerClass: any

try {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  BaseCallbackHandlerClass = require('@langchain/core/callbacks/base').BaseCallbackHandler
} catch {
  throw new Error(
    'LangChain integration requires @langchain/core. ' +
    'Install with: npm install @langchain/core'
  )
}

export class AgentPulseCallbackHandler extends BaseCallbackHandlerClass {
  name = 'agentpulse'
  private get _tracer() { return getTracer('agentpulse.langchain') }
  private _spans = new Map<string, Span>()

  // ── LLM callbacks ──────────────────────────────────────────────────────────

  handleLLMStart(
    serialized: Record<string, unknown>,
    prompts: string[],
    runId: string,
  ): void {
    const kwargs = serialized.kwargs as Record<string, unknown> | undefined
    const id = serialized.id as string[] | undefined
    const model =
      kwargs?.model_name ??
      kwargs?.model ??
      (id && id.length > 0 ? id[id.length - 1] : 'unknown')
    const span = this._tracer.startSpan(`llm.${model}`)
    span.setAttribute(attrs.SPAN_KIND, attrs.LLM_CALL)
    span.setAttribute(attrs.MODEL_ID, String(model))
    span.setAttribute(attrs.RUN_ID, getRunId())
    const projectId = getProjectId()
    if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)
    if (prompts?.length > 0) span.setAttribute(attrs.PROMPT, prompts[0].slice(0, 2000))
    this._spans.set(runId, span)
  }

  handleLLMEnd(
    response: { llm_output?: Record<string, unknown>; generations?: unknown[][] },
    runId: string,
  ): void {
    const span = this._spans.get(runId)
    if (!span) return
    this._spans.delete(runId)

    const usage = (response.llm_output?.token_usage ?? response.llm_output?.usage ?? {}) as Record<string, number>
    const inputTokens = usage.prompt_tokens ?? usage.input_tokens ?? 0
    const outputTokens = usage.completion_tokens ?? usage.output_tokens ?? 0
    if (inputTokens) span.setAttribute(attrs.INPUT_TOKENS, inputTokens)
    if (outputTokens) span.setAttribute(attrs.OUTPUT_TOKENS, outputTokens)

    const firstGen = response.generations?.[0]?.[0]
    if (firstGen) {
      const text = (firstGen as { text?: string }).text ?? String(firstGen)
      span.setAttribute(attrs.COMPLETION, text.slice(0, 2000))
    }

    span.end()
  }

  handleLLMError(error: Error, runId: string): void {
    const span = this._spans.get(runId)
    if (span) {
      this._spans.delete(runId)
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) })
      span.end()
    }
  }

  // ── Tool callbacks ─────────────────────────────────────────────────────────

  handleToolStart(
    serialized: Record<string, unknown>,
    _inputStr: string,
    runId: string,
  ): void {
    const toolName = String(serialized.name ?? 'unknown_tool')
    const span = this._tracer.startSpan(`tool.${toolName}`)
    span.setAttribute(attrs.SPAN_KIND, attrs.TOOL_CALL)
    span.setAttribute(attrs.TOOL_NAME, toolName)
    span.setAttribute(attrs.RUN_ID, getRunId())
    const projectId = getProjectId()
    if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)
    this._spans.set(runId, span)
  }

  handleToolEnd(_output: unknown, runId: string): void {
    const span = this._spans.get(runId)
    if (span) { this._spans.delete(runId); span.end() }
  }

  handleToolError(error: Error, runId: string): void {
    const span = this._spans.get(runId)
    if (span) {
      this._spans.delete(runId)
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) })
      span.end()
    }
  }

  // ── Chain callbacks ────────────────────────────────────────────────────────

  handleChainStart(
    serialized: Record<string, unknown>,
    _inputs: Record<string, unknown>,
    runId: string,
  ): void {
    const id = serialized.id as string[] | undefined
    const chainName = id && id.length > 0 ? id[id.length - 1] : 'chain'
    if (!String(chainName).toLowerCase().includes('agent')) return

    const span = this._tracer.startSpan(`agent.${chainName}`)
    span.setAttribute(attrs.SPAN_KIND, attrs.AGENT_HANDOFF)
    span.setAttribute(attrs.AGENT_NAME, String(chainName))
    span.setAttribute(attrs.RUN_ID, getRunId())
    const projectId = getProjectId()
    if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)
    this._spans.set(runId, span)
  }

  handleChainEnd(_outputs: unknown, runId: string): void {
    const span = this._spans.get(runId)
    if (span) { this._spans.delete(runId); span.end() }
  }

  handleChainError(error: Error, runId: string): void {
    const span = this._spans.get(runId)
    if (span) {
      this._spans.delete(runId)
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) })
      span.end()
    }
  }
}
