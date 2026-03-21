# AgentPulse SDK — Getting Started

Instrument any Python AI agent in under 5 minutes. This guide covers setup,
all five span kinds, the LangChain integration, and what you'll see in the UI.

---

## Prerequisites

- AgentPulse running locally — see [Running locally](#running-locally)
- Python 3.10+
- A project created in the AgentPulse UI (you need its UUID)

---

## Running locally

```bash
# 1. Start infrastructure (ClickHouse, Postgres, MinIO)
make dev-up

# 2. Apply migrations
make migrate-up

# 3. Start the collector (accepts OTel on :4317 gRPC / :4318 HTTP)
make collector-run

# 4. Start the backend API (on :8080)
make backend-run

# 5. Start the frontend (on :3000)
make web-dev
```

Open `http://localhost:3000`, create a project, and copy its ID.

---

## Install

```bash
cd sdk/python
pip install -e .
```

For HTTP transport instead of gRPC:
```bash
pip install -e ".[http]"
```

For LangChain integration:
```bash
pip install -e ".[langchain]"
```

---

## 1. Minimal setup

```python
import os
from agentpulse import init_tracer

# Option A: pass directly
init_tracer(AgentPulseConfig(project_id="your-project-uuid-here"))

# Option B: via environment variable (recommended)
# export AGENTPULSE_PROJECT_ID=your-project-uuid-here
init_tracer()  # reads AGENTPULSE_PROJECT_ID automatically
```

Call `init_tracer()` **once at startup**, before any instrumented code runs.
It configures the global OTel TracerProvider and connects to the collector.

---

## 2. Instrumenting your agent

### LLM calls

Wrap every function that calls an LLM with `@llm_call`. After the model
responds, call `record_llm_usage()` to attach token counts and the
prompt/completion text (required for quality scoring).

```python
from opentelemetry import trace
from agentpulse import init_tracer, llm_call, record_llm_usage

init_tracer()

@llm_call(model="claude-sonnet-4-6", agent_name="MyAgent")
def call_claude(prompt: str) -> str:
    response = anthropic_client.messages.create(
        model="claude-sonnet-4-6",
        max_tokens=1024,
        messages=[{"role": "user", "content": prompt}],
    )
    completion = response.content[0].text

    record_llm_usage(
        trace.get_current_span(),
        input_tokens=response.usage.input_tokens,
        output_tokens=response.usage.output_tokens,
        prompt=prompt,
        completion=completion,
    )
    return completion
```

**Why `record_llm_usage`?**

- `input_tokens` + `output_tokens` → the collector uses these to compute cost
  against the model pricing table. If you omit them, cost shows as $0.
- `prompt` + `completion` → the eval worker uses these to score response quality
  with an LLM-as-judge. If you omit them, no quality scores appear in the UI.

### Tool calls

```python
from agentpulse import tool_call

@tool_call(tool_name="web_search", agent_name="ResearchAgent")
def search(query: str) -> str:
    return requests.get(f"https://api.search.example.com?q={query}").json()
```

### Agent handoffs

Wrap the function that delegates control to another agent. All child spans
created inside the decorated function will appear as children in the DAG.

```python
from agentpulse import handoff

@handoff(target_agent="ResearchAgent", agent_name="OrchestratorAgent")
def delegate_research(task: str) -> str:
    # Everything called from here becomes a child of this handoff span
    return research_agent.run(task)
```

This is what creates the arrows between nodes in the topology graph.

### Memory

```python
from agentpulse import memory_read, memory_write

@memory_read(key="user_profile", agent_name="PersonalizationAgent")
def load_profile(user_id: str) -> dict:
    return memory_store.get(f"profile:{user_id}")

@memory_write(key="conversation_history", agent_name="PersonalizationAgent")
def save_turn(user_id: str, message: str) -> None:
    memory_store.append(f"history:{user_id}", message)
```

---

## 3. Context manager form

Use the `_ctx` variants when you need to set attributes mid-execution or
when decorating isn't practical:

```python
from agentpulse import llm_call_ctx, record_llm_usage

def process_batch(prompts: list[str]) -> list[str]:
    results = []
    for prompt in prompts:
        with llm_call_ctx(model="gpt-4o", agent_name="BatchAgent") as span:
            response = openai_client.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": prompt}],
            )
            completion = response.choices[0].message.content
            record_llm_usage(
                span,
                input_tokens=response.usage.prompt_tokens,
                output_tokens=response.usage.completion_tokens,
                prompt=prompt,
                completion=completion,
            )
            results.append(completion)
    return results
```

---

## 4. Multi-agent pipelines

The key to the topology DAG is OTel's automatic context propagation. When you
nest decorated functions, the parent-child `span_id` relationships are
preserved automatically:

```python
from agentpulse import init_tracer, handoff, llm_call, tool_call, record_llm_usage
from opentelemetry import trace

init_tracer()

@tool_call(tool_name="web_search", agent_name="ResearchAgent")
def web_search(query: str) -> str:
    return search_api.query(query)

@llm_call(model="claude-sonnet-4-6", agent_name="ResearchAgent")
def synthesize(context: str, question: str) -> str:
    prompt = f"Context:\n{context}\n\nAnswer: {question}"
    response = claude.complete(prompt)
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=response.input_tokens,
        output_tokens=response.output_tokens,
        prompt=prompt,
        completion=response.text,
    )
    return response.text

@handoff(target_agent="ResearchAgent", agent_name="OrchestratorAgent")
def research_agent(question: str) -> str:
    context = web_search(question)          # tool.call span (child of handoff)
    return synthesize(context, question)    # llm.call span (child of handoff)

@llm_call(model="gpt-4o", agent_name="OrchestratorAgent")
def orchestrate(task: str) -> str:
    # Plan first, then delegate
    plan = plan_task(task)
    return research_agent(task)             # handoff span (child of this llm.call)
```

In the AgentPulse topology view, this renders as:

```
OrchestratorAgent (llm.call)
  └── OrchestratorAgent → ResearchAgent (agent.handoff)
        ├── web_search (tool.call)
        └── synthesize (llm.call)
```

---

## 5. Async agents

The decorators work with `async def` automatically:

```python
@llm_call(model="gpt-4o", agent_name="AsyncAgent")
async def async_call(prompt: str) -> str:
    response = await async_openai_client.chat.completions.create(
        model="gpt-4o",
        messages=[{"role": "user", "content": prompt}],
    )
    completion = response.choices[0].message.content
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=response.usage.prompt_tokens,
        output_tokens=response.usage.completion_tokens,
        prompt=prompt,
        completion=completion,
    )
    return completion
```

---

## 6. Run ID management

AgentPulse groups all spans with the same `agentpulse.run_id` into a single
"run" in the UI. By default, a new UUID is generated the first time a span is
created and reused for all subsequent spans in the same async context.

To start a fresh run (e.g. one run per user request):

```python
from agentpulse import reset_run, set_run_id

# Auto-generate a new run_id for this request
reset_run()
result = handle_request(user_input)

# Or pin a specific ID (e.g. tie to your own request/session ID)
set_run_id(request.id)
result = handle_request(user_input)
```

---

## 7. LangChain integration

```python
from agentpulse import init_tracer
from agentpulse.integrations.langchain import AgentPulseCallbackHandler

init_tracer()
handler = AgentPulseCallbackHandler()

# Pass to any LangChain chain, agent, or LLM
result = chain.invoke(
    {"input": "What is AgentPulse?"},
    config={"callbacks": [handler]},
)
```

The handler automatically maps:
- `on_llm_start` / `on_llm_end` → `llm.call` spans with token counts
- `on_tool_start` / `on_tool_end` → `tool.call` spans
- `on_chain_start` / `on_chain_end` → `agent.handoff` spans (for AgentExecutor chains)

---

## 8. What you'll see in the UI

After running your instrumented agent:

**Project overview** (`/projects/<id>`):
- Cost per run chart — total USD spent per execution
- Quality score chart — avg eval score per run over time (appears after evals process)
- Latency chart — execution time per run
- Recent runs table with status, cost, token count

**Run detail** (`/projects/<id>/runs/<run-id>`):
- Span list with timing, kind badges, and quality score pills
- Click any span → side drawer with full attributes, prompt/completion text,
  token breakdown, and eval reasoning
- Topology tab → DAG showing agent → tool/llm relationships

**Eval quality scores**:
Scores appear asynchronously. The eval worker polls for `llm.call` spans with
`gen_ai.prompt` set, then calls Claude Haiku to score relevance (0.0–1.0).
Scores appear in the UI within ~30 seconds of the run completing, provided
`ANTHROPIC_API_KEY` is set in the backend's environment.

---

## 9. Configuration reference

| Environment variable | Default | Description |
|---|---|---|
| `AGENTPULSE_PROJECT_ID` | (required) | Project UUID from the AgentPulse UI |
| `AGENTPULSE_ENDPOINT` | `localhost:4317` (gRPC) / `http://localhost:4318` (HTTP) | Collector address |
| `AGENTPULSE_SERVICE` | `agentpulse-agent` | OTel `service.name` attribute |
| `AGENTPULSE_PROTOCOL` | `grpc` | `grpc` or `http` |

Or pass an `AgentPulseConfig` object directly:

```python
from agentpulse import init_tracer, AgentPulseConfig

init_tracer(AgentPulseConfig(
    project_id="your-uuid",
    endpoint="collector.internal:4317",
    service_name="my-agent-service",
    protocol="grpc",
    insecure=True,
))
```

---

## 10. Running the example

```bash
cd sdk/python

# Install
pip install -e .

# Set your project ID (create one in the UI first)
export AGENTPULSE_PROJECT_ID=your-project-uuid-here

# Run the multi-agent example
python examples/basic_multi_agent.py
```

You'll see output like:

```
[AgentPulse] Starting run for task: 'What are the best practices...'
[OrchestratorAgent] Plan: Based on my research, the answer is highly...
[ResearchAgent] Result: I found three key factors...
[AgentPulse] Run complete — check the UI for the topology graph.
```

Open `http://localhost:3000`, navigate to your project, and click the run to
see the DAG with 4 nodes and the quality score after ~30 seconds.

---

## 11. Graceful shutdown

Always call `shutdown()` at application exit to flush buffered spans:

```python
import atexit
from agentpulse import shutdown

atexit.register(shutdown)
```

Or explicitly:

```python
from agentpulse import shutdown

# At the end of your script / request handler
shutdown()
```

Without shutdown, spans buffered in the `BatchSpanProcessor` may not be
exported before the process exits.
