/**
 * OpenAI auto-instrumentation example.
 * Run: OPENAI_API_KEY=xxx npx tsx examples/openai-example.ts
 */
import OpenAI from 'openai'
import { initTracer, setRunId } from '../src/index.js'
import { instrumentOpenAI } from '../src/integrations/openai.js'

initTracer({ projectId: process.env.AGENTPULSE_PROJECT_ID ?? 'demo-project', protocol: 'http' })
setRunId('openai-demo-001')

const client = new OpenAI()
instrumentOpenAI(client)

async function main() {
  const response = await client.chat.completions.create({
    model: 'gpt-4o-mini',
    messages: [{ role: 'user', content: 'Explain observability in one sentence.' }],
  })
  console.log(response.choices[0].message.content)
}

main().catch(console.error)
