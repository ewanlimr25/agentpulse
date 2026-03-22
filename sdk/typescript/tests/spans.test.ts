import { describe, it, expect } from 'vitest'
import { llmCall, toolCall, handoff, memoryRead, memoryWrite } from '../src/spans.js'
import { setRunId, setProjectId } from '../src/context.js'
import * as attrs from '../src/generated/attributes.js'
import { getExportedSpans } from './setup.js'

describe('llmCall', () => {
  it('sets span_kind to llm.call', async () => {
    await llmCall({ model: 'gpt-4o' }, async () => 'result')
    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span).toBeDefined()
  })

  it('sets model attribute', async () => {
    await llmCall({ model: 'claude-sonnet-4-6' }, async () => 'result')
    const spans = getExportedSpans()
    const span = spans[0]
    expect(span.attributes[attrs.MODEL_ID]).toBe('claude-sonnet-4-6')
  })

  it('sets run_id from context', async () => {
    setRunId('run-xyz')
    await llmCall({ model: 'gpt-4o' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.RUN_ID]).toBe('run-xyz')
  })

  it('explicit runId overrides context', async () => {
    setRunId('context-run')
    await llmCall({ model: 'gpt-4o', runId: 'explicit-run' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.RUN_ID]).toBe('explicit-run')
  })

  it('sets project_id from context', async () => {
    setProjectId('proj-123')
    await llmCall({ model: 'gpt-4o' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.PROJECT_ID]).toBe('proj-123')
  })

  it('sets agentName', async () => {
    await llmCall({ model: 'gpt-4o', agentName: 'TestAgent' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.AGENT_NAME]).toBe('TestAgent')
  })

  it('sets ERROR status on exception', async () => {
    await expect(
      llmCall({ model: 'gpt-4o' }, async () => { throw new Error('LLM failed') })
    ).rejects.toThrow('LLM failed')
    const spans = getExportedSpans()
    expect(spans[0].status.code).toBe(2) // SpanStatusCode.ERROR
  })

  it('works with sync-returning function', async () => {
    const result = await llmCall({ model: 'gpt-4o' }, () => 'sync-result')
    expect(result).toBe('sync-result')
  })
})

describe('toolCall', () => {
  it('sets span_kind to tool.call', async () => {
    await toolCall({ toolName: 'web_search' }, async () => 'results')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.SPAN_KIND]).toBe(attrs.TOOL_CALL)
  })

  it('sets tool.name attribute', async () => {
    await toolCall({ toolName: 'code_exec' }, async () => 'ok')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.TOOL_NAME]).toBe('code_exec')
  })
})

describe('handoff', () => {
  it('sets span_kind to agent.handoff', async () => {
    await handoff({ targetAgent: 'SubAgent' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.SPAN_KIND]).toBe(attrs.AGENT_HANDOFF)
  })

  it('sets handoff target attribute', async () => {
    await handoff({ targetAgent: 'AnalysisAgent' }, async () => 'done')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.HANDOFF_TARGET]).toBe('AnalysisAgent')
  })
})

describe('memoryRead', () => {
  it('sets span_kind to memory.read', async () => {
    await memoryRead({}, async () => 'data')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.SPAN_KIND]).toBe(attrs.MEMORY_READ)
  })

  it('sets memory key when provided', async () => {
    await memoryRead({ key: 'user-profile' }, async () => 'data')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.MEMORY_KEY]).toBe('user-profile')
  })

  it('omits memory key when not provided', async () => {
    await memoryRead({}, async () => 'data')
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.MEMORY_KEY]).toBeUndefined()
  })
})

describe('memoryWrite', () => {
  it('sets span_kind to memory.write', async () => {
    await memoryWrite({ key: 'cache' }, async () => undefined)
    const spans = getExportedSpans()
    expect(spans[0].attributes[attrs.SPAN_KIND]).toBe(attrs.MEMORY_WRITE)
  })
})
