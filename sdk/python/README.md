# AgentPulse Python SDK

OpenTelemetry-native observability SDK for AI agent workloads. Instrument your Python agents with semantic span kinds and ship traces to AgentPulse.

## Quick Start

```bash
pip install agentpulse-sdk
# or from source:
pip install -e sdk/python
```

```python
from agentpulse import init_tracer, set_run_id, llm_call, record_llm_usage
from opentelemetry import trace

# One-line setup
init_tracer()  # reads AGENTPULSE_PROJECT_ID from env
set_run_id('run-abc-123')

# Wrap LLM calls
@llm_call(model='gpt-4o', agent_name='MyAgent')
def call_llm(prompt: str) -> str:
    response = my_client.chat(prompt)
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=response.usage.prompt_tokens,
        output_tokens=response.usage.completion_tokens,
        completion=response.text,
    )
    return response.text
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `AGENTPULSE_PROJECT_ID` | (required) | Project ID from AgentPulse dashboard |
| `AGENTPULSE_ENDPOINT` | `localhost:4317` (gRPC) | OTel collector endpoint |
| `AGENTPULSE_PROTOCOL` | `grpc` | Transport: `grpc` or `http` |
| `AGENTPULSE_SERVICE` | `agentpulse-agent` | `service.name` resource attribute |

## Span Kinds

### `llm_call` — LLM inference

```python
from agentpulse import llm_call, record_llm_usage
from opentelemetry import trace

@llm_call(model='claude-sonnet-4-6', agent_name='Planner')
def infer(prompt: str) -> str:
    result = call_llm(prompt)
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=100,
        output_tokens=200,
        prompt=prompt,
        completion=result,
    )
    return result
```

Or use the context manager form:

```python
from agentpulse import llm_call_ctx, record_llm_usage

with llm_call_ctx(model='gpt-4o', agent_name='Assistant') as span:
    result = call_llm(prompt)
    record_llm_usage(span, input_tokens=50, output_tokens=100)
```

### `tool_call` — External tool execution

```python
from agentpulse import tool_call

@tool_call(tool_name='web_search', agent_name='Researcher')
def search(query: str) -> list[dict]:
    return web_search(query)

# Or with context manager
from agentpulse import tool_call_ctx

with tool_call_ctx(tool_name='web_search') as span:
    results = web_search(query)
```

### `handoff` — Agent-to-agent delegation

```python
from agentpulse import handoff

@handoff(target_agent='SummaryAgent', agent_name='OrchestratorAgent')
async def delegate(context: str) -> str:
    return await summary_agent.run(context)
```

### `memory_read` / `memory_write` — Memory operations

```python
from agentpulse import memory_read, memory_write

@memory_read(key='user-profile', agent_name='MyAgent')
def get_profile(user_id: str) -> dict:
    return store.get(f'profile:{user_id}')

@memory_write(key='session-summary', agent_name='MyAgent')
def save_summary(summary: str) -> None:
    store.set('session-summary', summary)
```

## Framework Integrations

### LangChain

```python
from agentpulse import init_tracer
from agentpulse.integrations.langchain import AgentPulseCallbackHandler

init_tracer()
handler = AgentPulseCallbackHandler()

# Pass to any LangChain runnable
chain.invoke({'input': 'hello'}, config={'callbacks': [handler]})
```

Requires: `pip install 'agentpulse[langchain]'`

### CrewAI

```python
from agentpulse import init_tracer
from agentpulse.integrations.crewai import instrument_crewai

init_tracer()
instrument_crewai()  # Patches Crew, Agent, and BaseTool globally
```

Requires: `pip install 'agentpulse[crewai]'`

### AutoGen / AG2

```python
from agentpulse import init_tracer
from agentpulse.integrations.autogen import instrument_autogen

init_tracer()
instrument_autogen()  # Auto-detects AG2 vs legacy autogen
```

Requires: `pip install 'agentpulse[autogen]'`

### LlamaIndex

```python
from agentpulse import init_tracer
from agentpulse.integrations.llamaindex import instrument_llamaindex

init_tracer()
instrument_llamaindex()  # Registers on the root Dispatcher
```

Requires: `pip install 'agentpulse[llamaindex]'`

### OpenAI Agents SDK

```python
from agentpulse import init_tracer
from agentpulse.integrations.openai_agents import instrument_openai_agents

init_tracer()
hooks = instrument_openai_agents()

result = await Runner.run(agent, 'task', hooks=hooks)
```

Requires: `pip install 'agentpulse[openai]'`

## Run ID Context

Group spans into logical runs and sessions:

```python
from agentpulse import set_run_id, get_run_id, reset_run
from agentpulse import set_session_id, generate_session_id, reset_session
from agentpulse import set_user_id

# Pin a run ID for all spans in this execution
set_run_id('my-run-id')

# Or group multiple runs into a session (multi-turn conversation)
session_id = generate_session_id()
set_session_id(session_id)

# Attribute cost to a specific user (opaque identifier, not email)
set_user_id('user-12345')

# Read current values (run_id auto-generates if unset)
print(get_run_id())
```

## More Information

Full documentation: [docs/sdk-getting-started.md](../../docs/sdk-getting-started.md)
