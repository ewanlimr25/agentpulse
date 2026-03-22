import {
  InMemorySpanExporter,
  SimpleSpanProcessor,
} from '@opentelemetry/sdk-trace-base'
import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node'
import { beforeEach, afterEach } from 'vitest'
import { resetRun } from '../src/context.js'
import { _setTracerOverride } from '../src/spans.js'
import { _setTracerOverride as _setOpenAITracerOverride } from '../src/integrations/openai.js'
import { _setTracerOverride as _setVercelAITracerOverride } from '../src/integrations/vercel-ai.js'
import { _setTracerOverride as _setLangChainTracerOverride } from '../src/integrations/langchain.js'

// Use a wrapper object for live bindings
export const ctx: {
  provider: NodeTracerProvider
  exporter: InMemorySpanExporter
} = { provider: null!, exporter: null! }

beforeEach(() => {
  // Create a fresh exporter each test — avoid shutdown state from prior test
  ctx.exporter = new InMemorySpanExporter()
  ctx.provider = new NodeTracerProvider()
  ctx.provider.addSpanProcessor(new SimpleSpanProcessor(ctx.exporter))
  // Use provider.getTracer() directly — avoids OTel global singleton lock
  const tracer = ctx.provider.getTracer('agentpulse')
  _setTracerOverride(tracer)
  _setOpenAITracerOverride(tracer)
  _setVercelAITracerOverride(tracer)
  _setLangChainTracerOverride(tracer)
  resetRun()
})

afterEach(async () => {
  _setTracerOverride(undefined)
  _setOpenAITracerOverride(undefined)
  _setVercelAITracerOverride(undefined)
  _setLangChainTracerOverride(undefined)
  await ctx.provider.shutdown()
})

export function getExportedSpans() {
  return ctx.exporter.getFinishedSpans()
}

export function getSpanAttribute(span: ReturnType<typeof getExportedSpans>[number], key: string) {
  return span.attributes[key]
}
