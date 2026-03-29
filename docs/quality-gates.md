# Quality Gates

`agentpulse-cli eval check` gives your CI pipeline a binary pass/fail signal based on your project's eval scores. Point it at a project, set a score threshold, and it exits 0 (pass) or 1 (fail) — making it a drop-in step in any CI workflow.

---

## Installation

```bash
# Option A: install to /usr/local/bin via make
make cli-install

# Option B: install with go install
go install github.com/agentpulse/agentpulse/backend/cmd/agentpulse-cli@latest

# Option C: build a local binary
make cli-build          # outputs to tools/agentpulse-cli
```

---

## Basic usage

```bash
agentpulse-cli eval check \
  --project <project-id> \
  --threshold 0.7 \
  --runs 10
```

This fetches the avg eval score across the last 10 runs for the project and exits 0 if the overall score is ≥ 0.70, or exits 1 if it is below.

To gate on a specific eval type:

```bash
agentpulse-cli eval check \
  --project <project-id> \
  --threshold 0.65 \
  --eval-type hallucination \
  --runs 10
```

---

## Flags

| Flag | Default | Description |
|---|---|---|
| `--project` | (required) | Project UUID from the AgentPulse UI |
| `--threshold` | (required) | Minimum passing score, 0.0–1.0 |
| `--api-key` | `$AGENTPULSE_API_KEY` | Bearer token for the project |
| `--eval-type` | (all types) | Gate on one specific eval type (`relevance`, `hallucination`, `faithfulness`, `toxicity`, `tool_correctness`) |
| `--runs` | `10` | Number of recent runs to average (1–100) |
| `--min-runs` | `1` | Minimum runs required; exits 2 (error/skip) if fewer runs have eval data |
| `--fail-open` | `false` | Exit 0 when the API is unreachable instead of blocking the build |
| `--json` | `false` | Emit JSON to stdout instead of human-readable text |
| `--endpoint` | `$AGENTPULSE_ENDPOINT` or `https://api.agentpulse.io` | AgentPulse API base URL |

---

## Environment variables

| Variable | Description |
|---|---|
| `AGENTPULSE_API_KEY` | API key — use when `--api-key` flag is not set |
| `AGENTPULSE_ENDPOINT` | API base URL — use when `--endpoint` flag is not set (self-hosted deployments) |

---

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Pass — score meets or exceeds threshold |
| `1` | Fail — score is below threshold |
| `2` | Error or skip — API unreachable, authentication failed, insufficient data, or invalid flags |

Code 2 is distinct from 1 so that CI can differentiate a genuine quality regression from a configuration problem or a project with no eval data yet.

---

## Human-readable output

```
AgentPulse Eval Check — project: abc123
Threshold: 0.700  |  Runs considered: 10

eval type                   score      runs     spans
--------------------------------------------------------
✓ faithfulness              0.752         9        18
✗ hallucination             0.612        10        20
✓ relevance                 0.731        10        20
✓ tool_correctness          0.783         8        16

Result: FAIL ✗  —  overall score 0.720 across 4 eval type(s), 10 run(s)
```

When `--eval-type` is set, only that row is marked with a pass/fail indicator and the exit code reflects that type's score alone.

---

## JSON output

Pass `--json` to get a machine-readable result on stdout:

```json
{
  "pass": false,
  "exit_code": 1,
  "threshold": 0.7,
  "eval_type": "",
  "baseline": {
    "project_id": "abc123",
    "runs_considered": 10,
    "types": [
      { "eval_name": "faithfulness",     "avg_score": 0.752, "span_count": 18, "run_count": 9 },
      { "eval_name": "hallucination",    "avg_score": 0.612, "span_count": 20, "run_count": 10 },
      { "eval_name": "relevance",        "avg_score": 0.731, "span_count": 20, "run_count": 10 },
      { "eval_name": "tool_correctness", "avg_score": 0.783, "span_count": 16, "run_count": 8 }
    ],
    "overall_score": 0.720
  },
  "message": "overall score 0.720 across 4 eval type(s), 10 run(s)"
}
```

---

## Self-hosted deployments

Point the CLI at your own instance using the `--endpoint` flag or the environment variable:

```bash
export AGENTPULSE_ENDPOINT=http://localhost:8080
agentpulse-cli eval check --project <id> --threshold 0.7
```

---

## Handling projects with no eval data yet

Use `--min-runs` to make the gate advisory until you have enough history:

```bash
agentpulse-cli eval check \
  --project <project-id> \
  --threshold 0.7 \
  --min-runs 5
```

If fewer than 5 runs have eval data, the command exits 2 (skip) instead of 1 (fail). This prevents false failures during initial project setup.

Combined with `--fail-open`, the gate is completely non-blocking if either the API is unreachable or there is insufficient data:

```bash
agentpulse-cli eval check \
  --project <project-id> \
  --threshold 0.7 \
  --min-runs 5 \
  --fail-open
```
