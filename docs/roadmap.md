# AgentPulse Roadmap

---

## Tier 3 — Strategic Differentiators

### H. Agent Replay / Sandbox Debugging

**Why it's a moat:** no competitor does this. A developer can take a failed production trace and replay it locally with mocked tool responses (sourced from the original `tool.input`/`tool.output` span attributes), reproducing a prod failure deterministically.

**Architecture:**
- Replay engine: reads a run's span tree from the API, reconstructs the execution graph
- SDK "replay mode": intercepts real tool/LLM calls and substitutes recorded responses
- Configurable overrides: swap one tool's response to test a hypothesis
- UI: "Replay this run" button on run detail page, diff view showing original vs replay spans

**Effort:** ~2–3 weeks.

---

### I. Hardcoded Defaults Warning

Log a `WARN` at startup if `DATABASE_URL` contains `localhost` or the default `agentpulse:agentpulse` credentials. One-line change; do it opportunistically while touching config files for another item.

---

## Summary

| # | Item | Effort | Priority |
|---|------|--------|----------|
| H | Agent Replay / Sandbox Debugging | 2–3w | 🟡 Tier 3 |
| I | Hardcoded defaults warning | 30m | 🟢 Opportunistic |
