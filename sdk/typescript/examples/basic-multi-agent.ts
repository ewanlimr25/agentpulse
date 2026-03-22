/**
 * Basic multi-agent example demonstrating manual instrumentation.
 * Run: npx tsx examples/basic-multi-agent.ts
 */
import { initTracer, setRunId, llmCall, toolCall, handoff, recordLlmUsage } from '../src/index.js'

initTracer({ projectId: process.env.AGENTPULSE_PROJECT_ID ?? 'demo-project', protocol: 'http' })
setRunId('demo-run-001')

async function searchTool(query: string): Promise<string> {
  return toolCall({ toolName: 'web_search', agentName: 'ResearchAgent' }, async () => {
    console.log(`Searching for: ${query}`)
    return `Results for "${query}": some relevant data`
  })
}

async function researchAgent(topic: string): Promise<string> {
  return llmCall({ model: 'claude-sonnet-4-6', agentName: 'ResearchAgent' }, async (span) => {
    const searchResults = await searchTool(topic)
    recordLlmUsage(span, { inputTokens: 150, outputTokens: 300, prompt: topic, completion: searchResults })
    return `Research summary for ${topic}: ${searchResults}`
  })
}

async function summaryAgent(research: string): Promise<string> {
  return handoff({ targetAgent: 'SummaryAgent', agentName: 'ResearchAgent' }, async () => {
    return llmCall({ model: 'claude-haiku-4-5', agentName: 'SummaryAgent' }, async (span) => {
      recordLlmUsage(span, { inputTokens: 200, outputTokens: 100, prompt: research, completion: 'Summary...' })
      return `Executive summary: ${research.slice(0, 50)}...`
    })
  })
}

async function main() {
  const research = await researchAgent('AI agent observability')
  const summary = await summaryAgent(research)
  console.log('Final summary:', summary)
}

main().catch(console.error)
