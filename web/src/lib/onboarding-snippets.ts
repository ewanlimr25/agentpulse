// Framework-specific onboarding snippets for the Getting Started wizard.
// Placeholders {{PROJECT_ID}}, {{API_KEY}}, {{INGEST_TOKEN}}, and {{ENDPOINT}} are replaced at render time.

export type FrameworkLanguage = "python" | "typescript" | "shell";

export interface Framework {
  id: string;
  label: string;
  language: FrameworkLanguage;
  installCmd: string;
  envVars: string;
  code: string;
}

const ENDPOINT_DEFAULT = "http://localhost:4318";

export const FRAMEWORKS: Framework[] = [
  // ── Python ───────────────────────────────────────────────────────────────
  {
    id: "python-langchain",
    label: "LangChain",
    language: "python",
    installCmd: "pip install 'agentpulse[langchain]'",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `from agentpulse import init_tracer
from agentpulse.integrations.langchain import AgentPulseCallbackHandler

init_tracer()  # reads env vars automatically
handler = AgentPulseCallbackHandler()

# Pass to any LangChain runnable
chain.invoke({"input": "..."}, config={"callbacks": [handler]})`,
  },
  {
    id: "python-openai-agents",
    label: "OpenAI Agents",
    language: "python",
    installCmd: "pip install 'agentpulse[openai-agents]'",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `from agentpulse import init_tracer
from agentpulse.integrations.openai_agents import instrument_openai_agents

init_tracer()
instrument_openai_agents()  # patches the SDK globally

# Use the OpenAI Agents SDK normally — spans are emitted automatically
from agents import Agent, Runner
agent = Agent(name="MyAgent", instructions="You are helpful.")
result = Runner.run_sync(agent, "Hello!")`,
  },
  {
    id: "python-crewai",
    label: "CrewAI",
    language: "python",
    installCmd: "pip install 'agentpulse[crewai]'",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `from agentpulse import init_tracer
from agentpulse.integrations.crewai import instrument_crewai

init_tracer()
instrument_crewai()  # monkey-patches Crew and Agent at the class level

# Use CrewAI normally — spans are emitted automatically
crew = Crew(agents=[...], tasks=[...])
result = crew.kickoff()`,
  },
  {
    id: "python-autogen",
    label: "AutoGen",
    language: "python",
    installCmd: "pip install 'agentpulse[autogen]'",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `from agentpulse import init_tracer
from agentpulse.integrations.autogen import instrument_autogen

init_tracer()
instrument_autogen()  # instruments ConversableAgent message passing

# Use AutoGen normally — spans are emitted automatically
import autogen
assistant = autogen.AssistantAgent("assistant", llm_config={...})
user = autogen.UserProxyAgent("user", human_input_mode="NEVER")
user.initiate_chat(assistant, message="Hello!")`,
  },
  {
    id: "python-llamaindex",
    label: "LlamaIndex",
    language: "python",
    installCmd: "pip install 'agentpulse[llamaindex]'",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `from agentpulse import init_tracer
from agentpulse.integrations.llamaindex import instrument_llamaindex

init_tracer()
instrument_llamaindex()  # registers OTel callback with LlamaIndex global settings

# Use LlamaIndex normally — spans are emitted automatically
from llama_index.core import VectorStoreIndex, SimpleDirectoryReader
documents = SimpleDirectoryReader("data").load_data()
index = VectorStoreIndex.from_documents(documents)
query_engine = index.as_query_engine()
response = query_engine.query("What is this about?")`,
  },

  // ── TypeScript ───────────────────────────────────────────────────────────
  {
    id: "ts-vercel-ai",
    label: "Vercel AI SDK",
    language: "typescript",
    installCmd: "npm install @agentpulse/sdk",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `import { initTracer } from '@agentpulse/sdk'
import { instrumentVercelAI } from '@agentpulse/sdk/integrations/vercel-ai'
import { generateText } from 'ai'
import { openai } from '@ai-sdk/openai'

initTracer()          // reads env vars automatically
instrumentVercelAI()  // wraps generateText / streamText / generateObject

// Use the Vercel AI SDK normally — spans are emitted automatically
const { text } = await generateText({
  model: openai('gpt-4o'),
  prompt: 'Hello!',
})`,
  },
  {
    id: "ts-langchain",
    label: "LangChain",
    language: "typescript",
    installCmd: "npm install @agentpulse/sdk",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `import { initTracer } from '@agentpulse/sdk'
import { AgentPulseCallbackHandler } from '@agentpulse/sdk/integrations/langchain'

initTracer()
const handler = new AgentPulseCallbackHandler()

// Pass to any LangChain runnable
await chain.invoke({ input: '...' }, { callbacks: [handler] })`,
  },
  {
    id: "ts-openai",
    label: "OpenAI",
    language: "typescript",
    installCmd: "npm install @agentpulse/sdk",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_API_KEY={{API_KEY}}
AGENTPULSE_ENDPOINT=${ENDPOINT_DEFAULT}`,
    code: `import OpenAI from 'openai'
import { initTracer } from '@agentpulse/sdk'
import { instrumentOpenAI } from '@agentpulse/sdk/integrations/openai'

initTracer()
const client = new OpenAI()
instrumentOpenAI(client)  // patches this client instance

// Use the OpenAI client normally — spans are emitted automatically
const completion = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Hello!' }],
})`,
  },

  // ── CLI Tools ────────────────────────────────────────────────────────────────
  {
    id: "claude-code",
    label: "Claude Code",
    language: "shell",
    installCmd: "# One-time setup",
    envVars: `AGENTPULSE_PROJECT_ID={{PROJECT_ID}}
AGENTPULSE_INGEST_TOKEN={{INGEST_TOKEN}}
AGENTPULSE_ENDPOINT={{ENDPOINT}}`,
    code: `# Download and run the installer
curl -fsSL https://raw.githubusercontent.com/agentpulse/agentpulse/main/tools/claude-code-hook/install.sh | bash

# The installer will prompt for your:
#   Project ID:    {{PROJECT_ID}}
#   Ingest Token:  {{INGEST_TOKEN}}
#   Endpoint:      {{ENDPOINT}}

# That's it — every Claude Code tool call now appears in AgentPulse.
# Sessions show up under the Sessions tab within seconds.`,
  },
];

/** Returns the index of the first Python framework. */
export const DEFAULT_FRAMEWORK_INDEX = 0;
