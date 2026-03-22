/**
 * Vercel AI SDK auto-instrumentation example.
 * Run: OPENAI_API_KEY=xxx npx tsx examples/vercel-ai-example.ts
 */
import { generateText } from 'ai'
import { openai } from '@ai-sdk/openai'
import { initTracer, setRunId } from '../src/index.js'
import { instrumentVercelAI } from '../src/integrations/vercel-ai.js'

initTracer({ projectId: process.env.AGENTPULSE_PROJECT_ID ?? 'demo-project', protocol: 'http' })
instrumentVercelAI()
setRunId('vercel-ai-demo-001')

async function main() {
  const { text } = await generateText({
    model: openai('gpt-4o-mini'),
    prompt: 'Explain OTel in one sentence.',
  })
  console.log(text)
}

main().catch(console.error)
