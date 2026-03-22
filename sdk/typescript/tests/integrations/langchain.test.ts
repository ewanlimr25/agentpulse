import { describe, it, expect, beforeEach, vi } from 'vitest'
import { getExportedSpans, ctx } from '../setup.js'
import * as attrs from '../../src/generated/attributes.js'

class MockBaseCallbackHandler {
  name = 'base'
}

vi.mock('@langchain/core/callbacks/base', () => ({
  BaseCallbackHandler: MockBaseCallbackHandler,
}))

describe('AgentPulseCallbackHandler', () => {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let handler: any

  beforeEach(async () => {
    const mod = await import('../../src/integrations/langchain.js')
    // Update tracer override to current test provider
    mod._setTracerOverride(ctx.provider.getTracer('agentpulse'))
    handler = new mod.AgentPulseCallbackHandler()
  })

  it('handleLLMStart + handleLLMEnd emits llm.call span', () => {
    const runId = 'run-001'
    handler.handleLLMStart(
      { kwargs: { model_name: 'gpt-4o' } },
      ['hello world'],
      runId,
    )
    handler.handleLLMEnd(
      {
        llm_output: { token_usage: { prompt_tokens: 10, completion_tokens: 20 } },
        generations: [[{ text: 'Hi there!' }]],
      },
      runId,
    )

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span).toBeDefined()
    expect(span?.attributes[attrs.MODEL_ID]).toBe('gpt-4o')
    expect(span?.attributes[attrs.INPUT_TOKENS]).toBe(10)
    expect(span?.attributes[attrs.OUTPUT_TOKENS]).toBe(20)
    expect(span?.attributes[attrs.COMPLETION]).toBe('Hi there!')
  })

  it('handleToolStart + handleToolEnd emits tool.call span', () => {
    const runId = 'run-002'
    handler.handleToolStart({ name: 'web_search' }, 'query', runId)
    handler.handleToolEnd('results', runId)

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.TOOL_CALL)
    expect(span).toBeDefined()
    expect(span?.attributes[attrs.TOOL_NAME]).toBe('web_search')
  })

  it('handleChainStart only emits span when name contains "agent"', () => {
    // Should NOT emit span for non-agent chain
    handler.handleChainStart({ id: ['LLMChain'] }, {}, 'run-non-agent')

    // Should emit span for agent chain
    handler.handleChainStart({ id: ['AgentExecutor'] }, {}, 'run-agent')
    handler.handleChainEnd({}, 'run-agent')

    const spans = getExportedSpans()
    const agentSpans = spans.filter(s => s.attributes[attrs.SPAN_KIND] === attrs.AGENT_HANDOFF)
    expect(agentSpans).toHaveLength(1)
  })

  it('handleLLMError sets ERROR status', () => {
    const runId = 'run-003'
    handler.handleLLMStart({ kwargs: { model_name: 'gpt-4o' } }, [], runId)
    handler.handleLLMError(new Error('API error'), runId)

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span?.status.code).toBe(2) // ERROR
  })
})
