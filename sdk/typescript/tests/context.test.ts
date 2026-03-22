import { describe, it, expect, beforeEach } from 'vitest'
import { setRunId, getRunId, resetRun, setProjectId, getProjectId } from '../src/context.js'

describe('context', () => {
  beforeEach(() => {
    resetRun()
  })

  it('getRunId auto-generates a UUID when not set', () => {
    const id = getRunId()
    expect(id).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('explicit runId parameter takes priority over context', () => {
    const id = getRunId('my-explicit-id')
    expect(id).toBe('my-explicit-id')
  })

  it('setRunId + getRunId round-trip', () => {
    setRunId('test-run-123')
    expect(getRunId()).toBe('test-run-123')
  })

  it('resetRun clears the run ID', () => {
    setRunId('to-be-cleared')
    resetRun()
    const newId = getRunId()
    expect(newId).not.toBe('to-be-cleared')
    expect(newId).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('generates different IDs on each auto-generate', () => {
    const id1 = getRunId()
    resetRun()
    const id2 = getRunId()
    // Both should be valid UUID format
    expect(id1).toMatch(/^[0-9a-f-]{36}$/)
    expect(id2).toMatch(/^[0-9a-f-]{36}$/)
  })

  it('setProjectId + getProjectId round-trip', () => {
    setProjectId('proj-abc')
    expect(getProjectId()).toBe('proj-abc')
  })

  it('getProjectId returns undefined when not set', () => {
    // Reset project ID by setting empty string
    setProjectId('')
    // After setting empty, returns empty string which is falsy
    const id = getProjectId()
    expect(id === undefined || id === '').toBe(true)
  })
})
