/**
 * Singleton integrity test.
 *
 * Validates that the CJS-primary + ESM-wrapper packaging pattern ensures
 * a single module instance when both require() and import() are used in
 * the same process.
 *
 * NOTE: This test runs against the SOURCE (not dist) because vitest transforms
 * both paths through the same module graph. For a true dist-level test, run:
 *   npm run build && node --input-type=module <<< "
 *     import { setRunId } from './dist/index.mjs'
 *     const { getRunId } = require('./dist/index.js')
 *     setRunId('test')
 *     console.assert(getRunId() === 'test', 'singleton broken')
 *   "
 *
 * The source-level test below validates the context module's state isolation
 * properties that the singleton guarantee depends on.
 */

import { describe, it, expect, beforeEach } from 'vitest'
import { setRunId, getRunId, resetRun, setProjectId, getProjectId } from '../src/context.js'
import { recordLlmUsage } from '../src/usage.js'
import { llmCall } from '../src/spans.js'
import { getExportedSpans } from './setup.js'
import * as attrs from '../src/generated/attributes.js'

describe('singleton integrity', () => {
  beforeEach(() => {
    resetRun()
  })

  it('setRunId from one import path is visible via getRunId from same module', () => {
    setRunId('singleton-run-001')
    expect(getRunId()).toBe('singleton-run-001')
  })

  it('setProjectId state flows into span attributes', async () => {
    setProjectId('proj-singleton')
    setRunId('run-singleton')

    await llmCall({ model: 'test-model' }, async (span) => {
      recordLlmUsage(span, { inputTokens: 5, outputTokens: 10 })
    })

    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.PROJECT_ID]).toBe('proj-singleton')
    expect(spans[0].attributes[attrs.RUN_ID]).toBe('run-singleton')
  })

  it('resetRun clears state visible to all callers', () => {
    setRunId('will-be-cleared')
    resetRun()
    const after = getRunId()
    expect(after).not.toBe('will-be-cleared')
  })

  it('context module exports are the same function references across imports', async () => {
    // Dynamic re-import of the same module must resolve to the same instance
    const mod1 = await import('../src/context.js')
    const mod2 = await import('../src/context.js')
    expect(mod1.setRunId).toBe(mod2.setRunId)
    expect(mod1.getRunId).toBe(mod2.getRunId)
  })
})
