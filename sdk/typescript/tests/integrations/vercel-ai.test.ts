import { describe, it, expect } from 'vitest'
import { getExportedSpans, ctx } from '../setup.js'
import * as attrs from '../../src/generated/attributes.js'
import { llmCall } from '../../src/spans.js'
import { recordLlmUsage } from '../../src/usage.js'
import { instrumentVercelAI, uninstrumentVercelAI } from '../../src/integrations/vercel-ai.js'
import { _setTracerOverride } from '../../src/integrations/vercel-ai.js'

/**
 * Vercel AI integration tests.
 *
 * Since the `ai` ESM module namespace uses non-configurable properties that
 * prevent direct monkey-patching in the test environment, we test the
 * instrumentation plumbing indirectly:
 * - span creation correctness via llmCall + recordLlmUsage (the same internal
 *   path that instrumentVercelAI uses)
 * - idempotency and uninstrumentation guards
 */

describe('vercel-ai integration', () => {
  it('llmCall emits llm.call span with correct attributes', async () => {
    const mockResult = {
      text: 'Hello world',
      usage: { promptTokens: 10, completionTokens: 20 },
    }

    await llmCall({ model: 'gpt-4o', spanName: 'llm.gpt-4o' }, async (span) => {
      recordLlmUsage(span, {
        inputTokens: mockResult.usage.promptTokens,
        outputTokens: mockResult.usage.completionTokens,
        completion: mockResult.text,
      })
      return mockResult
    })

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span).toBeDefined()
    expect(span?.attributes[attrs.MODEL_ID]).toBe('gpt-4o')
    expect(span?.attributes[attrs.INPUT_TOKENS]).toBe(10)
    expect(span?.attributes[attrs.OUTPUT_TOKENS]).toBe(20)
    expect(span?.attributes[attrs.COMPLETION]).toBe('Hello world')
  })

  it('instrumentVercelAI is idempotent — calling twice does not throw', () => {
    // First call instruments (or skips gracefully if ai not patchable in test env)
    expect(() => instrumentVercelAI()).not.toThrow()
    // Second call should be a no-op due to _instrumented guard
    expect(() => instrumentVercelAI()).not.toThrow()
    // Clean up
    uninstrumentVercelAI()
  })

  it('uninstrumentVercelAI is safe to call when not instrumented', () => {
    // Ensure we start un-instrumented
    uninstrumentVercelAI()
    expect(() => uninstrumentVercelAI()).not.toThrow()
  })

  it('_setTracerOverride is exported for test injection', () => {
    expect(typeof _setTracerOverride).toBe('function')
    // Set and clear without error
    _setTracerOverride(ctx.provider.getTracer('agentpulse'))
    _setTracerOverride(undefined)
  })
})
