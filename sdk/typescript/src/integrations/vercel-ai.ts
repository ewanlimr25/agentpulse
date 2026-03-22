/**
 * Vercel AI SDK auto-instrumentation.
 *
 * Wraps generateText, streamText, generateObject at the public API boundary.
 * Does NOT depend on LanguageModelV1 internals.
 *
 * Usage:
 *   import { instrumentVercelAI } from '@agentpulse/sdk/integrations/vercel-ai'
 *   instrumentVercelAI()  // Call once before using the ai package
 */

import { trace, SpanStatusCode, type Tracer } from '@opentelemetry/api'
import * as attrs from '../generated/attributes.js'
import { getRunId, getProjectId } from '../context.js'

let _tracerOverride: Tracer | undefined
/** @internal — for testing only */
export function _setTracerOverride(t: Tracer | undefined): void { _tracerOverride = t }
function getTracer() { return _tracerOverride ?? trace.getTracer('agentpulse') }

let _instrumented = false
let _origGenerateText: unknown
let _origStreamText: unknown
let _origGenerateObject: unknown

function getAiModule(): Record<string, unknown> | null {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require('ai') as Record<string, unknown>
  } catch {
    console.warn(
      '[AgentPulse] Vercel AI SDK instrumentation skipped: "ai" package not installed.\n' +
      'Install with: npm install ai'
    )
    return null
  }
}

function checkVersion(): boolean {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const pkg = require('ai/package.json') as { version?: string; default?: { version: string } }
    const version = pkg.version ?? pkg.default?.version ?? '0.0.0'
    const [major] = version.split('.').map(Number)
    if (major < 3) {
      console.warn(
        `[AgentPulse] Vercel AI SDK instrumentation requires ai>=3.0.0, found ${version}. ` +
        'Instrumentation disabled — spans will not be emitted for AI SDK calls.'
      )
      return false
    }
    return true
  } catch {
    return true // Can't read version, proceed optimistically
  }
}

/**
 * Safe property setter — falls back to Object.defineProperty, then no-ops.
 * ESM namespace objects may have non-configurable properties; we warn and skip
 * rather than throw, so instrumentation is best-effort in those environments.
 */
function safeSet(obj: Record<string, unknown>, key: string, value: unknown): boolean {
  try {
    obj[key] = value
    return true
  } catch {
    try {
      Object.defineProperty(obj, key, { value, writable: true, configurable: true })
      return true
    } catch {
      console.warn(
        `[AgentPulse] Could not patch ai.${key} — ` +
        'ESM module namespace may be non-configurable. ' +
        'Instrumentation for this function is disabled. ' +
        'Use manual llmCall() wrappers instead.'
      )
      return false
    }
  }
}

function startSpan(model: string, agentName?: string, runId?: string) {
  const span = getTracer().startSpan(`llm.${model}`)
  span.setAttribute(attrs.SPAN_KIND, attrs.LLM_CALL)
  span.setAttribute(attrs.MODEL_ID, model)
  span.setAttribute(attrs.RUN_ID, getRunId(runId))
  const projectId = getProjectId()
  if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)
  if (agentName) span.setAttribute(attrs.AGENT_NAME, agentName)
  return span
}

export interface InstrumentVercelAIOptions {
  agentName?: string
}

export function instrumentVercelAI(options: InstrumentVercelAIOptions = {}): void {
  if (_instrumented) return

  const ai = getAiModule()
  if (!ai) return
  if (!checkVersion()) return

  // Wrap generateText
  if (typeof ai.generateText === 'function') {
    _origGenerateText = ai.generateText
    safeSet(ai, 'generateText', async function instrumentedGenerateText(params: Record<string, unknown>) {
      const model = (params.model as { modelId?: string })?.modelId ?? 'unknown'
      const span = startSpan(model, options.agentName, params.runId as string | undefined)
      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const result = await (_origGenerateText as any)(params)
        if (result?.usage) {
          span.setAttribute(attrs.INPUT_TOKENS, result.usage.promptTokens ?? 0)
          span.setAttribute(attrs.OUTPUT_TOKENS, result.usage.completionTokens ?? 0)
        }
        if (result?.text) span.setAttribute(attrs.COMPLETION, String(result.text).slice(0, 2000))
        return result
      } catch (err) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
        throw err
      } finally {
        span.end()
      }
    })
  }

  // Wrap generateObject
  if (typeof ai.generateObject === 'function') {
    _origGenerateObject = ai.generateObject
    safeSet(ai, 'generateObject', async function instrumentedGenerateObject(params: Record<string, unknown>) {
      const model = (params.model as { modelId?: string })?.modelId ?? 'unknown'
      const span = startSpan(model, options.agentName, params.runId as string | undefined)
      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const result = await (_origGenerateObject as any)(params)
        if (result?.usage) {
          span.setAttribute(attrs.INPUT_TOKENS, result.usage.promptTokens ?? 0)
          span.setAttribute(attrs.OUTPUT_TOKENS, result.usage.completionTokens ?? 0)
        }
        return result
      } catch (err) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
        throw err
      } finally {
        span.end()
      }
    })
  }

  // Wrap streamText
  if (typeof ai.streamText === 'function') {
    _origStreamText = ai.streamText
    safeSet(ai, 'streamText', async function instrumentedStreamText(params: Record<string, unknown>) {
      const model = (params.model as { modelId?: string })?.modelId ?? 'unknown'
      const span = startSpan(model, options.agentName, params.runId as string | undefined)
      try {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const result = await (_origStreamText as any)(params)
        // usage is a Promise that resolves when the stream completes
        if (result?.usage && typeof result.usage.then === 'function') {
          result.usage.then((usage: { promptTokens?: number; completionTokens?: number }) => {
            span.setAttribute(attrs.INPUT_TOKENS, usage?.promptTokens ?? 0)
            span.setAttribute(attrs.OUTPUT_TOKENS, usage?.completionTokens ?? 0)
            span.end()
          }).catch(() => { span.end() })
        } else {
          span.end()
        }
        return result
      } catch (err) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
        span.end()
        throw err
      }
    })
  }

  _instrumented = true
}

export function uninstrumentVercelAI(): void {
  if (!_instrumented) return
  let ai: Record<string, unknown>
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    ai = require('ai') as Record<string, unknown>
  } catch { return }

  if (_origGenerateText) safeSet(ai, 'generateText', _origGenerateText)
  if (_origGenerateObject) safeSet(ai, 'generateObject', _origGenerateObject)
  if (_origStreamText) safeSet(ai, 'streamText', _origStreamText)
  _instrumented = false
}
