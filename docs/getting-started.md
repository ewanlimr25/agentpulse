# Getting started

This tutorial takes a fresh clone to a working local AgentPulse stack, your first real trace, a first budget rule, and a first eval score. Plan for ~20 minutes start to finish.

If anything goes wrong along the way, jump to [Troubleshooting](#troubleshooting).

---

## 0. Prerequisites

| Tool | Version | Install |
|---|---|---|
| Docker Desktop | any recent | [docker.com](https://www.docker.com/products/docker-desktop) |
| Go | 1.22+ | `brew install go` |
| Node.js | 20+ | `brew install node` or [nvm](https://github.com/nvm-sh/nvm) |
| Python | 3.10+ | only for the Python SDK |
| `jq` | any | optional — used in curl examples |
| `psql`, `clickhouse-client` | any | optional — easier debugging |

You will need ~8 GB of free RAM and ~4 GB of free disk. None of this touches the internet after the first container pulls.

---

## 1. Clone and set environment

```bash
git clone https://github.com/agentpulse/agentpulse.git
cd agentpulse

cp .env.example .env
# The defaults in .env.example already target the local docker-compose stack.
# Nothing to edit unless you want to enable evals (see §7) or notifications.
```

---

## 2. Start infrastructure

```bash
make web-install     # one-time: installs Next.js dependencies (~2 min)
make dev-up          # starts ClickHouse, Postgres, MinIO via docker-compose
```

Expected output ends in:

```
Infrastructure ready.
```

If you want to watch logs:

```bash
make dev-logs
```

---

## 3. Apply migrations

```bash
make migrate-up
```

This applies every `.up.sql` under `migrations/postgres/` and every `.sql` under `migrations/clickhouse/` in the order the backend expects. If you're re-running against a partially-migrated database you may see "table already exists" warnings for the earliest migrations — they're safe to ignore.

---

## 4. Start the three services

Open three terminals. Each command blocks; leave it running.

**Terminal 1 — collector** (OTel receiver on `:4317` gRPC and `:4318` HTTP)

```bash
make collector-run
```

Expected: `agentpulse collector ready` with a list of pipelines.

**Terminal 2 — backend API** (REST on `:8080`)

```bash
make backend-run
```

Expected: `HTTP server listening on 0.0.0.0:8080`. If you see a `relation "run_tags" does not exist` error, revisit §3.

**Terminal 3 — frontend** (Next.js on `:3000`)

```bash
make web-dev
```

Expected: `✓ Ready in …` followed by `http://localhost:3000`.

Keep all three running. If you close a terminal, that service stops.

---

## 5. Create your first project

In a fourth terminal:

```bash
curl -s -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -d '{"name":"my-first-project"}' | jq .
```

Response:

```json
{
  "data": {
    "project": {
      "ID": "a1b2c3d4-5e6f-7890-abcd-ef1234567890",
      "Name": "my-first-project"
    },
    "api_key": "apk_01H9Z7..."
  }
}
```

**Save both values.** The `api_key` is shown exactly once — the server stores only its SHA-256 hash.

```bash
export PROJECT_ID=a1b2c3d4-5e6f-7890-abcd-ef1234567890
export AGENTPULSE_API_KEY=apk_01H9Z7...
export AGENTPULSE_API_URL=http://localhost:8080
```

---

## 6. Send synthetic traces

The fastest way to prove the stack end-to-end:

```bash
go run ./tools/tracegen/... \
  --project-id "$PROJECT_ID" \
  --scenario all \
  --runs 5
```

You should see five runs appear at [http://localhost:3000](http://localhost:3000) within a few seconds. Click into any run to see:

- The **topology DAG** (React Flow) — one node per agent, edges for handoffs
- The **span list** with cost, latency, token counts
- The **timeline** (Gantt view) if you prefer
- The **span drawer** with raw attributes

Try the **Run Comparison** (select two runs and click *Compare*) and the **Prompt Playground** (click an `llm.call` span → *Edit & re-send*).

---

## 7. Instrument a real agent

Replace synthetic runs with your actual workload. Pick your language.

### Python (quickest — auto-instrumentor)

```bash
pip install agentpulse-sdk
```

```python
from agentpulse import AgentPulse
from anthropic import Anthropic

pulse = AgentPulse(
    project_id="a1b2c3d4-...",
    endpoint="http://localhost:4317",
)
client = Anthropic()

with pulse.run(name="summarize-article") as run:
    with run.agent("summarizer"):
        with run.llm(model="claude-sonnet-4-6") as span:
            resp = client.messages.create(
                model="claude-sonnet-4-6",
                max_tokens=512,
                messages=[{"role": "user", "content": "Summarize: ..."}],
            )
            span.record_usage(
                input_tokens=resp.usage.input_tokens,
                output_tokens=resp.usage.output_tokens,
            )
            print(resp.content[0].text)
```

Framework users can skip manual spans with:

```python
from agentpulse.instrumentation.crewai import CrewAIInstrumentor
CrewAIInstrumentor().instrument()
# now every Task/Agent/Tool invocation emits an agentpulse span automatically
```

Instrumentors also ship for **LangChain**, **AutoGen / AG2**, **LlamaIndex**, and the **OpenAI Agents SDK**.

### TypeScript (Vercel AI SDK)

```bash
npm install @agentpulse/sdk
```

```ts
import { AgentPulse } from "@agentpulse/sdk";
import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

const pulse = new AgentPulse({
  projectId: "a1b2c3d4-...",
  endpoint: "http://localhost:4317",
});

await pulse.run("summarize", async (run) => {
  const { text, usage } = await generateText({
    model: openai("gpt-4o"),
    prompt: "Summarize: ...",
  });
  // usage is captured automatically by the Vercel AI instrumentor
  console.log(text);
});
```

### Claude Code (zero code)

If you run Claude Code on your machine, add the hook and every session becomes a run:

```bash
make cli-install
agentpulse hook install
```

Details: [claude-code-integration.md](./claude-code-integration.md).

---

## 8. Set a budget rule

Stop a run if it gets expensive. Notify-only first:

```bash
curl -s -X POST "http://localhost:8080/api/v1/projects/$PROJECT_ID/budget/rules" \
  -H "Authorization: Bearer $AGENTPULSE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "warn at 10 cents",
    "threshold_usd": 0.10,
    "action": "notify",
    "scope": "run",
    "enabled": true
  }' | jq .
```

Actions:
- `notify` — writes an alert, fires webhook/Slack/Discord if configured
- `halt` — stamps `agentpulse.budget.halted=true` on subsequent spans so your SDK wrapper can short-circuit the run

Scopes: `run`, `agent`, or `user`.

Rules refresh from Postgres every 30 seconds — no restart needed.

---

## 9. Turn on evals

Evals need an LLM API key for the judge. Add one to `.env`:

```dotenv
ANTHROPIC_API_KEY=sk-ant-...
# OR
OPENAI_API_KEY=sk-...
# OR
GOOGLE_AI_API_KEY=...
```

Restart the backend (Ctrl-C in Terminal 2, then `make backend-run`). The eval worker picks up new spans and scores them asynchronously.

Configure which judges to run for this project:

```bash
curl -s -X PUT "http://localhost:8080/api/v1/projects/$PROJECT_ID/evals/config" \
  -H "Authorization: Bearer $AGENTPULSE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "judges": [
      { "type": "relevance",      "model": "claude-haiku-4-5" },
      { "type": "hallucination",  "model": "claude-haiku-4-5" },
      { "type": "tool_correctness", "model": "claude-haiku-4-5" }
    ]
  }' | jq .
```

Supported judge types: `relevance`, `hallucination`, `faithfulness`, `toxicity`, `tool_correctness`, `semantic_similarity`, `custom`.

Open the **Evals** tab in the UI — the trend chart populates as spans are scored.

---

## 10. Wire a quality gate into CI

`agentpulse-cli eval check` fails your CI job if recent evals drop below a threshold:

```yaml
# .github/workflows/ai-quality.yml
jobs:
  quality:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install AgentPulse CLI
        run: curl -sSL https://agentpulse.dev/install.sh | sh
      - name: Run agent test suite
        run: ./scripts/run-agent-tests.sh
      - name: Block if quality regressed
        env:
          AGENTPULSE_API_URL: ${{ secrets.AGENTPULSE_API_URL }}
          AGENTPULSE_API_KEY: ${{ secrets.AGENTPULSE_API_KEY }}
        run: |
          agentpulse eval check \
            --project "$PROJECT_ID" \
            --window 10m \
            --min-score 0.8 \
            --judges relevance,hallucination
```

Full recipe: [quality-gates.md](./quality-gates.md).

---

## 11. Hook notifications up

Add Slack or Discord alerts from the project → Settings → Notifications panel (or via the API). For browser push:

```bash
npx web-push generate-vapid-keys
```

Put the keys in `.env`:

```dotenv
VAPID_PUBLIC_KEY=...
VAPID_PRIVATE_KEY=...
VAPID_SUBJECT=mailto:you@example.com
```

Restart the backend. In the UI, click *Enable push notifications* — your browser asks for permission, subscribes, and you'll get desktop toasts for the signal alerts you've configured.

---

## Troubleshooting

### `make dev-up` hangs / ports in use

```bash
lsof -i :5432   # Postgres — often a system install
lsof -i :9000   # ClickHouse — collides with some tools
lsof -i :9090   # MinIO
```

Kill the offender or edit `docker-compose.yml` to expose different ports.

### Backend exits with `relation "run_tags" does not exist`

You're missing the newer migrations. See [§3](#applying-migrations-manually).

### Collector shows `invalid API key` on every span

The OTel receiver does **not** require a token in this release — it accepts any OTLP payload. If you're seeing this, check you're pointing the SDK at `:4317` (collector), not `:8080` (backend).

### `go run ./tools/tracegen/... --project-id demo-project` succeeds but nothing appears in the UI

`demo-project` is the literal default; you must pass your real project UUID. Re-run with `--project-id $PROJECT_ID`.

### UI shows "No runs yet" forever

Open [http://localhost:8080/healthz](http://localhost:8080/healthz) — should be `{"status":"ok"}`. Then check the collector log for `accepted N spans`. If the collector accepted spans but the UI is empty, the project ID in the spans doesn't match the one in the URL bar.

### `make seed` wipes my data

It does — that's the point. Use it on a scratch install, never on real data.

### Can I disable MinIO?

Yes, if no span carries a payload larger than 8 KB. Edit `docker-compose.yml` and remove the `minio` + `minio-init` services. The backend will silently skip offloading.

### Can I disable ClickHouse or Postgres?

No. Every feature touches both. ClickHouse stores span-scale data; Postgres stores operational config. See [architecture.md](./architecture.md) for the split rationale.

---

## What's next

- **[architecture.md](./architecture.md)** — how data flows through the system
- **[sdk-getting-started.md](./sdk-getting-started.md)** — deeper Python patterns (sessions, users, memory spans)
- **[claude-code-integration.md](./claude-code-integration.md)** — turn every Claude Code session into an observable run
- **[feasibility-analysis.md](./feasibility-analysis.md)** — honest take on whether you should run this for your use case
- **[recommendations.md](./recommendations.md)** — where the observability landscape is going and what AgentPulse should build next
