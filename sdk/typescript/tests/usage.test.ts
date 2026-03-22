import { describe, it, expect } from 'vitest'
import { recordLlmUsage } from '../src/usage.js'
import * as attrs from '../src/generated/attributes.js'
import { getExportedSpans, ctx } from './setup.js'

describe('recordLlmUsage', () => {
  it('sets input and output token counts', () => {
    const tracer = ctx.provider.getTracer('test')
    tracer.startActiveSpan('test', (span) => {
      recordLlmUsage(span, { inputTokens: 100, outputTokens: 200 })
      span.end()
    })
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.INPUT_TOKENS]).toBe(100)
    expect(spans[0].attributes[attrs.OUTPUT_TOKENS]).toBe(200)
  })

  it('sets prompt and completion when provided', () => {
    const tracer = ctx.provider.getTracer('test')
    tracer.startActiveSpan('test', (span) => {
      recordLlmUsage(span, {
        inputTokens: 50,
        outputTokens: 75,
        prompt: 'hello',
        completion: 'world',
      })
      span.end()
    })
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.PROMPT]).toBe('hello')
    expect(spans[0].attributes[attrs.COMPLETION]).toBe('world')
  })

  it('omits prompt and completion when not provided', () => {
    const tracer = ctx.provider.getTracer('test')
    tracer.startActiveSpan('test', (span) => {
      recordLlmUsage(span, { inputTokens: 10, outputTokens: 20 })
      span.end()
    })
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.PROMPT]).toBeUndefined()
    expect(spans[0].attributes[attrs.COMPLETION]).toBeUndefined()
  })

  it('sets costUsd when provided', () => {
    const tracer = ctx.provider.getTracer('test')
    tracer.startActiveSpan('test', (span) => {
      recordLlmUsage(span, { inputTokens: 100, outputTokens: 100, costUsd: 0.005 })
      span.end()
    })
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.COST_USD]).toBe(0.005)
  })

  it('omits costUsd when not provided', () => {
    const tracer = ctx.provider.getTracer('test')
    tracer.startActiveSpan('test', (span) => {
      recordLlmUsage(span, { inputTokens: 100, outputTokens: 100 })
      span.end()
    })
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.COST_USD]).toBeUndefined()
  })
})
