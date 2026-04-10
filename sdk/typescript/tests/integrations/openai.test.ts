import { describe, it, expect, vi } from 'vitest'
import { getExportedSpans } from '../setup.js'
import * as attrs from '../../src/generated/attributes.js'
import { instrumentOpenAI, uninstrumentOpenAI } from '../../src/integrations/openai.js'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function makeMockClient(): any {
  return {
    chat: {
      completions: {
        create: vi.fn(async () => ({
          choices: [{ message: { content: 'Hello!' } }],
          usage: { prompt_tokens: 20, completion_tokens: 30 },
        })),
      },
    },
  }
}

describe('instrumentOpenAI', () => {
  it('patches chat.completions.create and emits llm.call span', async () => {
    const client = makeMockClient()
    instrumentOpenAI(client)

    await client.chat.completions.create({ model: 'gpt-4o', messages: [] })

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span).toBeDefined()
    expect(span?.attributes[attrs.MODEL_ID]).toBe('gpt-4o')
  })

  it('records token usage', async () => {
    const client = makeMockClient()
    instrumentOpenAI(client)

    await client.chat.completions.create({ model: 'gpt-4o', messages: [] })

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span?.attributes[attrs.INPUT_TOKENS]).toBe(20)
    expect(span?.attributes[attrs.OUTPUT_TOKENS]).toBe(30)
  })

  it('uninstrumentOpenAI restores original', async () => {
    const client = makeMockClient()
    const orig = client.chat.completions.create
    instrumentOpenAI(client)
    uninstrumentOpenAI(client)
    expect(client.chat.completions.create).toBe(orig)
  })

  it('sets ERROR status on exception', async () => {
    const client = makeMockClient()
    client.chat.completions.create.mockRejectedValueOnce(new Error('rate limit'))
    instrumentOpenAI(client)

    await expect(client.chat.completions.create({ model: 'gpt-4o', messages: [] }))
      .rejects.toThrow('rate limit')

    const spans = getExportedSpans()
    const span = spans.find(s => s.attributes[attrs.SPAN_KIND] === attrs.LLM_CALL)
    expect(span?.status.code).toBe(2)
  })
})
