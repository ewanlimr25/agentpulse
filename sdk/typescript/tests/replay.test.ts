import { describe, it, expect } from 'vitest'
import { promises as fs } from 'node:fs'
import path from 'node:path'
import os from 'node:os'

import { ReplayEngine, loadBundle, type ReplayBundle } from '../src/replay.js'
import * as attrs from '../src/generated/attributes.js'

function makeBundle(): ReplayBundle {
  return {
    SchemaVersion: 1,
    Run: { ID: 'run-original-123' },
    Topology: {},
    Spans: [
      {
        SpanID: 'span-llm-1',
        AgentSpanKind: attrs.LLM_CALL,
        AgentName: 'Researcher',
        SpanName: 'ask_llm',
        ModelID: 'gpt-4o',
        CallIndex: 0,
        Inputs: { 'gen_ai.prompt': 'What is 2+2?' },
        Outputs: { 'gen_ai.completion': '4' },
        InputTokens: 10,
        OutputTokens: 2,
      },
      {
        SpanID: 'span-tool-1',
        AgentSpanKind: attrs.TOOL_CALL,
        AgentName: 'Researcher',
        SpanName: 'web_search',
        ToolName: 'web_search',
        CallIndex: 0,
        Inputs: { 'tool.input': 'claude code' },
        Outputs: { 'tool.output': 'RECORDED_RESULTS' },
      },
      {
        SpanID: 'span-llm-2',
        AgentSpanKind: attrs.LLM_CALL,
        AgentName: 'Researcher',
        SpanName: 'ask_llm',
        ModelID: 'gpt-4o',
        CallIndex: 1,
        Inputs: { 'gen_ai.prompt': 'Second prompt' },
        Outputs: { 'gen_ai.completion': 'second answer' },
        InputTokens: 5,
        OutputTokens: 3,
      },
    ],
  }
}

function fakeSpan() {
  const calls: Array<[string, unknown]> = []
  return {
    setAttribute: (k: string, v: unknown) => {
      calls.push([k, v])
    },
    _calls: calls,
  } as unknown as { setAttribute: (k: string, v: unknown) => void; _calls: Array<[string, unknown]> }
}

describe('loadBundle from file', () => {
  it('roundtrips JSON envelope', async () => {
    const bundle = makeBundle()
    const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'replay-'))
    const file = path.join(dir, 'bundle.json')
    await fs.writeFile(file, JSON.stringify({ data: bundle }))

    const loaded = await loadBundle(file)
    expect(loaded.SchemaVersion).toBe(1)
    expect((loaded.Run as { ID: string }).ID).toBe('run-original-123')
    expect(loaded.Spans).toHaveLength(3)
    expect(loaded.Spans[0].Outputs?.['gen_ai.completion']).toBe('4')
    expect(loaded.Spans[1].ToolName).toBe('web_search')
  })
})

describe('ReplayEngine.intercept', () => {
  it('matches by (agent_name, span_name, call_index) and returns recorded output', () => {
    const engine = new ReplayEngine(makeBundle())
    const span = fakeSpan()
    const r = engine.intercept(attrs.LLM_CALL, 'Researcher', 'ask_llm', 'What is 2+2?', span as any)
    expect(r.matched).toBe(true)
    if (r.matched) expect(r.value).toBe('4')
    expect(span._calls).toContainEqual(['agentpulse.replay_source_run_id', 'run-original-123'])
    expect(span._calls).toContainEqual(['agentpulse.replay_source_span_id', 'span-llm-1'])
  })

  it('increments call_index for repeated (agent_name, span_name)', () => {
    const engine = new ReplayEngine(makeBundle())
    const r1 = engine.intercept(attrs.LLM_CALL, 'Researcher', 'ask_llm', 'What is 2+2?', fakeSpan() as any)
    const r2 = engine.intercept(attrs.LLM_CALL, 'Researcher', 'ask_llm', 'Second prompt', fakeSpan() as any)
    expect(r1.matched && r2.matched).toBe(true)
    if (r1.matched && r2.matched) {
      expect(r1.value).toBe('4')
      expect(r2.value).toBe('second answer')
    }
  })

  it('returns override value when set', () => {
    const engine = new ReplayEngine(makeBundle(), { web_search: 'OVERRIDE' })
    const r = engine.intercept(attrs.TOOL_CALL, 'Researcher', 'web_search', 'claude code', fakeSpan() as any)
    expect(r.matched).toBe(true)
    if (r.matched) expect(r.value).toBe('OVERRIDE')
  })

  it('marks unmatched on miss', () => {
    const engine = new ReplayEngine(makeBundle())
    const span = fakeSpan()
    const r = engine.intercept(attrs.LLM_CALL, 'Unknown', 'no_such_span', undefined, span as any)
    expect(r.matched).toBe(false)
    expect(span._calls).toContainEqual(['agentpulse.replay.unmatched', true])
  })

  it('marks divergence on input mismatch', () => {
    const engine = new ReplayEngine(makeBundle())
    const span = fakeSpan()
    engine.intercept(attrs.TOOL_CALL, 'Researcher', 'web_search', 'DIFFERENT', span as any)
    expect(span._calls).toContainEqual(['agentpulse.replay.diverged', true])
    expect(span._calls).toContainEqual(['agentpulse.replay.input.actual', 'DIFFERENT'])
    expect(span._calls).toContainEqual(['agentpulse.replay.input.recorded', 'claude code'])
  })
})
