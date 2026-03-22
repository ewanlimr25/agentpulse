# @agentpulse/sdk

OpenTelemetry-native observability SDK for AI agent workloads. Instrument your TypeScript/JavaScript agents with 5 semantic span kinds and ship traces to AgentPulse.

## Quick Start

```bash
npm install @agentpulse/sdk @opentelemetry/api
```

```typescript
import { initTracer, setRunId, llmCall, toolCall, recordLlmUsage } from '@agentpulse/sdk'

// One-line setup
initTracer({ projectId: 'your-project-id' })
setRunId('run-abc-123')

// Wrap LLM calls
const result = await llmCall({ model: 'gpt-4o', agentName: 'MyAgent' }, async (span) => {
  const response = await openai.chat.completions.create({ ... })
  recordLlmUsage(span, {
    inputTokens: response.usage.prompt_tokens,
    outputTokens: response.usage.completion_tokens,
    costUsd: 0.002,
  })
  return response
})
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `AGENTPULSE_PROJECT_ID` | (required) | Project ID from AgentPulse dashboard |
| `AGENTPULSE_ENDPOINT` | `localhost:4317` (grpc) | OTel collector endpoint |
| `AGENTPULSE_PROTOCOL` | `grpc` | Transport: `grpc` or `http` |
| `AGENTPULSE_SERVICE` | `agentpulse-agent` | `service.name` resource attribute |

## 5 Span Kinds

### `llmCall` — LLM inference

```typescript
import { llmCall, recordLlmUsage } from '@agentpulse/sdk'

await llmCall({ model: 'claude-sonnet-4-6', agentName: 'Planner' }, async (span) => {
  const res = await callLLM(prompt)
  recordLlmUsage(span, { inputTokens: 100, outputTokens: 200, prompt, completion: res.text })
  return res
})
```

### `toolCall` — External tool execution

```typescript
import { toolCall } from '@agentpulse/sdk'

await toolCall({ toolName: 'web_search', agentName: 'Researcher' }, async () => {
  return await searchWeb(query)
})
```

### `handoff` — Agent-to-agent delegation

```typescript
import { handoff } from '@agentpulse/sdk'

await handoff({ targetAgent: 'SummaryAgent', agentName: 'OrchestratorAgent' }, async () => {
  return await summaryAgent.run(context)
})
```

### `memoryRead` / `memoryWrite` — Memory operations

```typescript
import { memoryRead, memoryWrite } from '@agentpulse/sdk'

const profile = await memoryRead({ key: 'user-profile' }, async () => {
  return await store.get('user-profile')
})

await memoryWrite({ key: 'session-summary' }, async () => {
  await store.set('session-summary', summary)
})
```

## Auto-Instrumentation Integrations

### OpenAI JS SDK

```typescript
import OpenAI from 'openai'
import { instrumentOpenAI } from '@agentpulse/sdk/integrations/openai'

const client = new OpenAI()
instrumentOpenAI(client)
// All client.chat.completions.create() calls now emit spans automatically
```

### Vercel AI SDK

```typescript
import { instrumentVercelAI } from '@agentpulse/sdk/integrations/vercel-ai'

instrumentVercelAI() // Call once before using the ai package
// generateText(), streamText(), generateObject() now emit spans
```

### LangChain.js

```typescript
import { AgentPulseCallbackHandler } from '@agentpulse/sdk/integrations/langchain'

const handler = new AgentPulseCallbackHandler()
await chain.invoke({ input: 'hello' }, { callbacks: [handler] })
```

## Edge Runtime Note

`setRunId()` requires `AsyncLocalStorage` (available in Node.js 18+). In environments where it is unavailable (e.g., Cloudflare Workers without the `nodejs_compat` flag), calling `setRunId()` will throw with a descriptive error.

**Workaround for edge runtimes**: pass `runId` explicitly to each span function:

```typescript
await llmCall({ model: 'gpt-4o', runId: myRunId }, async (span) => { ... })
```

## Run ID Context

```typescript
import { setRunId, getRunId, resetRun } from '@agentpulse/sdk'

setRunId('my-run-id')   // All subsequent spans in this async context use this ID
getRunId()              // Returns current run ID (auto-generates UUID if unset)
resetRun()              // Clear the current run ID
```
