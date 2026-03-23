import { describe, it, expect, beforeEach } from 'vitest'
import {
  setSessionId,
  getSessionId,
  generateSessionId,
  resetSession,
  resetRun,
} from '../src/context.js'
import { llmCall } from '../src/spans.js'
import { SESSION_ID } from '../src/generated/attributes.js'
import { getExportedSpans } from './setup.js'

describe('generateSessionId', () => {
  it('returns a valid UUID v4', () => {
    const id = generateSessionId()
    expect(id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  })

  it('returns unique values on each call', () => {
    const ids = new Set(Array.from({ length: 50 }, () => generateSessionId()))
    expect(ids.size).toBe(50)
  })
})

describe('setSessionId / getSessionId', () => {
  beforeEach(() => {
    resetSession()
  })

  it('returns undefined when not set', () => {
    expect(getSessionId()).toBeUndefined()
  })

  it('round-trips a set value', () => {
    setSessionId('my-session-abc')
    expect(getSessionId()).toBe('my-session-abc')
  })

  it('round-trips a generated UUID', () => {
    const id = generateSessionId()
    setSessionId(id)
    expect(getSessionId()).toBe(id)
  })
})

describe('resetSession', () => {
  it('clears the session ID', () => {
    setSessionId('to-clear')
    resetSession()
    expect(getSessionId()).toBeUndefined()
  })
})

describe('session_id span attribute', () => {
  beforeEach(() => {
    resetSession()
    resetRun()
  })

  it('stamps agentpulse.session_id on spans when session is set', async () => {
    setSessionId('session-xyz')
    await llmCall({ model: 'gpt-4o', spanName: 'test-llm' }, async () => 'ok')
    const spans = getExportedSpans()
    expect(spans).toHaveLength(1)
    expect(spans[0].attributes[SESSION_ID]).toBe('session-xyz')
  })

  it('does not stamp session_id when not set', async () => {
    await llmCall({ model: 'gpt-4o', spanName: 'test-llm' }, async () => 'ok')
    const spans = getExportedSpans()
    expect(spans).toHaveLength(1)
    expect(spans[0].attributes[SESSION_ID]).toBeUndefined()
  })

  it('stamps the same session_id on multiple spans', async () => {
    setSessionId('shared-session')
    await llmCall({ model: 'gpt-4o', spanName: 'span-1' }, async () => 'a')
    await llmCall({ model: 'gpt-4o', spanName: 'span-2' }, async () => 'b')
    const spans = getExportedSpans()
    expect(spans).toHaveLength(2)
    for (const span of spans) {
      expect(span.attributes[SESSION_ID]).toBe('shared-session')
    }
  })

  it('stops stamping session_id after resetSession', async () => {
    setSessionId('before-reset')
    await llmCall({ model: 'gpt-4o', spanName: 'before' }, async () => 'a')
    resetSession()
    resetRun()
    await llmCall({ model: 'gpt-4o', spanName: 'after' }, async () => 'b')
    const spans = getExportedSpans()
    expect(spans).toHaveLength(2)
    // spans are in insertion order
    const beforeSpan = spans.find(s => s.name === 'before')!
    const afterSpan = spans.find(s => s.name === 'after')!
    expect(beforeSpan.attributes[SESSION_ID]).toBe('before-reset')
    expect(afterSpan.attributes[SESSION_ID]).toBeUndefined()
  })
})
