# AgentPulse Roadmap

## Full Usability Review: AgentPulse for Individual Developers

### What We Cover Well

The core observability loop is solid for a solo developer running agents:

- **Trace & span inspection** — Full prompt/completion viewing, token counts, cost per LLM call, TTFT, throughput. The span detail drawer is comprehensive.
- **Cost tracking** — Per-run, per-agent, per-session, per-user attribution. Budget rules with halt/notify. This is genuinely differentiated.
- **Topology visualization** — Interactive DAG showing agent→tool→LLM flow. Run comparison with overlay. This directly answers "what did my agent actually do?"
- **Loop detection** — Two-tier (repeated tool calls + topology cycles). Critical for catching runaway agents.
- **Evals** — Multi-model consensus judging, custom eval configs, baseline tracking. The human feedback (thumbs up/down + correction) is a nice touch.
- **Session tracking** — Groups multi-turn conversations. Useful for someone running an agent across multiple interactions.
- **SDK integrations** — Python covers LangChain, OpenAI Agents, CrewAI, AutoGen, LlamaIndex. TypeScript covers Vercel AI, LangChain, OpenAI. Good breadth.
- **Replay** — Deterministic re-execution from recorded responses. Powerful for debugging.
- **Search** — Full-text across prompts/completions/tool I/O. Essential for debugging.
- **Real-time alerts** — Polling-based toast notifications for budget and signal alerts.

### What's Missing for the Individual Developer Persona

Gaps prioritized by how much they'd matter to someone running OpenClaw or similar:

#### 1. No Onboarding / Empty State Experience
There's no quickstart wizard, setup guide in the UI, or empty-state guidance. A solo developer hitting the dashboard for the first time sees "No runs yet." with no next steps. They need:
- A "Getting Started" panel showing: install SDK → set env vars → run first agent → see data appear
- Copy-pasteable snippets for their specific framework
- A health check indicator showing whether the collector is receiving data

#### 2. No Live/Streaming Run View
A developer watching an agent execute in real-time has no way to see spans arriving as they happen. The current UI is entirely retrospective — you wait for a run to finish, then inspect it. For the OpenClaw scenario, they'd want:
- A live tail showing spans as they stream in during an active run
- A "currently running" indicator on the runs list
- Real-time cost counter ticking up during execution

#### 3. No Prompt Playground / Experiment Mode
When a developer sees a bad LLM output in the trace, the natural next action is "let me tweak this prompt and retry." There's no way to:
- Take a span's prompt, edit it, and re-send it to the model
- A/B test prompt variants
- Save prompt versions and compare outputs

This is probably the #1 feature Langfuse users love. It closes the observe→improve loop.

> **Implementation plan:** [`.claude/plan/prompt-playground.md`](../.claude/plan/prompt-playground.md)

#### 4. No Model Cost/Performance Comparison Dashboard
The services page shows tool stats and agent cost breakdown, but there's no view that answers "which model should I use?" Individual developers constantly experiment with models. They need:
- A model-level breakdown: cost, latency, quality score, token efficiency — grouped by model ID
- A "what if I switched from Opus to Sonnet" cost projection

#### 5. No Data Export
There's no way to export runs, traces, or analytics as CSV/JSON. A solo developer doing analysis in a notebook, or wanting to share results, needs this. The replay bundle is close but it's for re-execution, not analysis.

#### 6. No CLI Beyond `eval check`
The `agentpulse eval check` CI gate exists, but a solo developer would benefit from:
- `agentpulse runs list` — quick terminal check on recent runs
- `agentpulse runs tail` — live tail of incoming spans (solves gap #2 from terminal)
- `agentpulse status` — "is the collector healthy, are spans flowing?"

#### 7. No Tagging / Annotations on Runs
When iterating on agents, developers need to mark runs: "this was the version with the new system prompt," "this used tool X." There's no way to:
- Tag runs with arbitrary labels (e.g., "experiment-v3", "baseline")
- Filter/group runs by tags
- Annotate a run with free-text notes

This is critical for the observe→improve cycle. Without it, the runs list is just a chronological log with no semantic meaning.

#### 8. No Diff Between Prompt Versions Across Runs
The run comparison shows topology and metric deltas, but doesn't highlight what actually changed in the prompts or system instructions between runs. A developer iterating on their agent's prompts needs to see "run A used system prompt X, run B used system prompt Y, here's the diff."

#### 9. No Notification Channels Beyond Webhooks
Webhooks require a server to receive them. A solo developer would much rather get a:
- Desktop notification (browser push)
- Slack DM
- Discord webhook (simpler than generic webhook)
- Email digest

#### 10. No Dark/Light Mode Toggle
The UI appears to be dark-mode only (CSS vars suggest a single dark theme). Some developers prefer light mode, especially during daytime. Minor, but it's a polish issue for an always-on side tool.

#### 11. Missing Claude Code / OpenClaw-Specific Integration
If the primary target is "someone running OpenClaw," there's no integration that directly hooks into Claude Code's tool use. The OpenAI Agents SDK integration exists, but Claude Code uses the Anthropic API directly. You'd want:
- A Claude Code hook that auto-sends spans to AgentPulse
- Or at minimum, a guide for wiring OTEL_EXPORTER into Claude Code's environment

#### 12. No Retention / Cleanup Policy in the UI
The MinIO lifecycle is 35 days in docker-compose, but there's no UI for the developer to:
- See how much storage they're using
- Configure data retention
- Manually purge old runs

For someone running this on their laptop long-term, disk space matters.

---

### What to Build Next

**High priority** (close the observe→improve loop):
1. ~~Prompt playground — edit & re-send from span detail~~ ✓
2. Run tagging & annotations
3. ~~Live run streaming view~~ ✓
4. Model comparison dashboard

**Medium priority** (developer experience):
5. Onboarding empty state / getting started wizard
6. Data export (CSV/JSON)
7. CLI for runs/status/tail
8. Browser push notifications

**Lower priority** (polish):
9. Dark/light mode toggle
10. Storage/retention settings UI
11. Prompt diff across runs
12. Claude Code / OpenClaw-specific integration guide
