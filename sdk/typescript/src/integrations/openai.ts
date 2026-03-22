/**
 * OpenAI JS SDK auto-instrumentation.
 *
 * Version detection via openai/package.json (standard dd-trace pattern).
 * Duck-types response for v4/v5 streaming compatibility.
 *
 * Usage:
 *   import OpenAI from 'openai'
 *   import { instrumentOpenAI } from '@agentpulse/sdk/integrations/openai'
 *   const client = new OpenAI()
 *   instrumentOpenAI(client)
 */

import { trace, SpanStatusCode, type Span, type Tracer } from '@opentelemetry/api'
import * as attrs from '../generated/attributes.js'
import { getRunId, getProjectId } from '../context.js'

let _tracerOverride: Tracer | undefined
/** @internal — for testing only */
export function _setTracerOverride(t: Tracer | undefined): void { _tracerOverride = t }
function getTracer() { return _tracerOverride ?? trace.getTracer('agentpulse') }

const MIN_VERSION = [4, 28, 0]

function checkOpenAIVersion(): boolean {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const pkg = require('openai/package.json') as { version: string }
    const parts = pkg.version.split('.').map(Number)
    const [major, minor, patch] = parts
    if (major < MIN_VERSION[0]) {
      console.warn(`[AgentPulse] OpenAI instrumentation requires openai>=4.28.0, found ${pkg.version}. Skipping.`)
      return false
    }
    if (major === 4 && (minor < MIN_VERSION[1] || (minor === MIN_VERSION[1] && patch < MIN_VERSION[2]))) {
      console.warn(`[AgentPulse] OpenAI instrumentation requires openai>=4.28.0, found ${pkg.version}. Skipping.`)
      return false
    }
    return true
  } catch {
    return true // Proceed optimistically if version can't be read
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type OpenAIClient = any

const _originals = new WeakMap<OpenAIClient, unknown>()

export function instrumentOpenAI(client: OpenAIClient): OpenAIClient {
  if (!checkOpenAIVersion()) return client
  if (_originals.has(client)) return client // already patched

  const origCreate = client.chat.completions.create
  _originals.set(client, origCreate)

  client.chat.completions.create = async function instrumentedCreate(
    params: Record<string, unknown>,
    options?: unknown
  ) {
    const model = String(params.model ?? 'unknown')
    const span = getTracer().startSpan(`llm.${model}`)
    span.setAttribute(attrs.SPAN_KIND, attrs.LLM_CALL)
    span.setAttribute(attrs.MODEL_ID, model)
    span.setAttribute(attrs.RUN_ID, getRunId())
    const projectId = getProjectId()
    if (projectId) span.setAttribute(attrs.PROJECT_ID, projectId)

    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const result = await (origCreate as any)(params, options)

      // Duck-type: streaming or non-streaming?
      const isAsyncIterable = result != null && typeof result[Symbol.asyncIterator] === 'function'
      const isReadableStream = result != null && typeof result.toReadableStream === 'function'

      if (isAsyncIterable || isReadableStream) {
        // Streaming: wrap the async iterator, extract usage from last chunk
        return wrapStream(result, span)
      }

      // Non-streaming: extract usage directly
      if (result?.usage) {
        span.setAttribute(attrs.INPUT_TOKENS, result.usage.prompt_tokens ?? 0)
        span.setAttribute(attrs.OUTPUT_TOKENS, result.usage.completion_tokens ?? 0)
      }
      const text = result?.choices?.[0]?.message?.content
      if (text) span.setAttribute(attrs.COMPLETION, String(text).slice(0, 2000))
      span.end()
      return result
    } catch (err) {
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
      span.end()
      throw err
    }
  }

  return client
}

function wrapStream(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  stream: any,
  span: Span,
) {
  return {
    ...stream,
    [Symbol.asyncIterator]: async function* () {
      try {
        for await (const chunk of stream) {
          if (chunk?.usage) {
            span.setAttribute(attrs.INPUT_TOKENS, chunk.usage.prompt_tokens ?? 0)
            span.setAttribute(attrs.OUTPUT_TOKENS, chunk.usage.completion_tokens ?? 0)
          }
          yield chunk
        }
        span.end()
      } catch (err) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: String(err) })
        span.end()
        throw err
      }
    },
  }
}

export function uninstrumentOpenAI(client: OpenAIClient): void {
  const orig = _originals.get(client)
  if (orig) {
    client.chat.completions.create = orig
    _originals.delete(client)
  }
}
