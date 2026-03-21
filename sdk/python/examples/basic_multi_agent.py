"""
Basic multi-agent example for AgentPulse.

This shows a two-agent pipeline:
  OrchestratorAgent → (handoff) → ResearchAgent → llm.call + tool.call

Prerequisites:
  1. AgentPulse running locally (make dev-up && make migrate-up && make backend-run)
  2. AGENTPULSE_PROJECT_ID set to a project UUID from the AgentPulse UI
  3. pip install -e ".[dev]" (from sdk/python/)

Run:
  AGENTPULSE_PROJECT_ID=<your-project-id> python examples/basic_multi_agent.py

Then open the AgentPulse UI and look for a new run with a 3-node topology graph.
"""

import random
import time
from opentelemetry import trace

from agentpulse import (
    init_tracer,
    handoff,
    llm_call,
    tool_call,
    memory_read,
    memory_write,
    record_llm_usage,
    reset_run,
)

# ── Init ──────────────────────────────────────────────────────────────────────

init_tracer()  # Reads AGENTPULSE_PROJECT_ID from env; exports to localhost:4317

# ── Simulated LLM / tool clients ──────────────────────────────────────────────

SAMPLE_RESPONSES = [
    "Based on my research, the answer is highly relevant to your query.",
    "I found three key factors that directly address your question.",
    "The analysis shows a clear correlation between the inputs and the expected output.",
]


def fake_llm(prompt: str, model: str) -> tuple[str, int, int]:
    """Simulate an LLM API call. Returns (completion, input_tokens, output_tokens)."""
    time.sleep(0.05)  # Simulate latency
    completion = random.choice(SAMPLE_RESPONSES)
    input_tokens = len(prompt.split()) * 2
    output_tokens = len(completion.split()) * 2
    return completion, input_tokens, output_tokens


def fake_web_search(query: str) -> str:
    """Simulate a web search tool."""
    time.sleep(0.03)
    return f"Top results for '{query}': [result1, result2, result3]"


# ── Agent implementations ──────────────────────────────────────────────────────

@tool_call(tool_name="web_search", agent_name="ResearchAgent")
def web_search(query: str) -> str:
    return fake_web_search(query)


@memory_read(key="prior_context", agent_name="ResearchAgent")
def load_context() -> str:
    return "Previously researched: AI observability tools"


@memory_write(key="research_results", agent_name="ResearchAgent")
def save_results(results: str) -> None:
    pass  # In a real agent this would write to a memory store


@llm_call(model="claude-sonnet-4-6", agent_name="ResearchAgent")
def research_with_llm(question: str, context: str) -> str:
    prompt = f"Context: {context}\n\nQuestion: {question}\n\nProvide a concise answer."
    completion, input_tokens, output_tokens = fake_llm(prompt, "claude-sonnet-4-6")
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=input_tokens,
        output_tokens=output_tokens,
        prompt=prompt,
        completion=completion,
    )
    return completion


@handoff(target_agent="ResearchAgent", agent_name="OrchestratorAgent")
def research_agent(question: str) -> str:
    """ResearchAgent: loads context, searches the web, calls LLM, saves results."""
    context = load_context()
    search_results = web_search(question)
    answer = research_with_llm(question, f"{context}\n\nSearch results: {search_results}")
    save_results(answer)
    return answer


@llm_call(model="gpt-4o", agent_name="OrchestratorAgent")
def plan_task(task: str) -> str:
    """OrchestratorAgent plans which sub-agent to invoke."""
    prompt = f"Break this task into steps: {task}"
    completion, input_tokens, output_tokens = fake_llm(prompt, "gpt-4o")
    record_llm_usage(
        trace.get_current_span(),
        input_tokens=input_tokens,
        output_tokens=output_tokens,
        prompt=prompt,
        completion=completion,
    )
    return completion


def run_pipeline(task: str) -> str:
    """Top-level pipeline: OrchestratorAgent plans, then delegates to ResearchAgent."""
    reset_run()  # Start a fresh run_id for each pipeline invocation

    print(f"\n[AgentPulse] Starting run for task: {task!r}")

    # Step 1: Orchestrator plans
    plan = plan_task(task)
    print(f"[OrchestratorAgent] Plan: {plan[:80]}...")

    # Step 2: Handoff to ResearchAgent
    result = research_agent(task)
    print(f"[ResearchAgent] Result: {result[:80]}...")

    print("[AgentPulse] Run complete — check the UI for the topology graph.")
    return result


# ── Main ──────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    tasks = [
        "What are the best practices for multi-agent AI observability?",
        "How do LLM token costs compare across major providers in 2025?",
        "What is the difference between tracing and logging for AI agents?",
    ]

    for task in tasks:
        run_pipeline(task)
        time.sleep(0.5)  # Brief pause between runs

    # Flush remaining spans before exit
    from agentpulse import shutdown
    shutdown()
    print("\nDone. Open AgentPulse and navigate to your project to see the runs.")
