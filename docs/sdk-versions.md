# SDK Version Compatibility

This document tracks the tested SDK and framework version combinations for AgentPulse. Combinations marked "tested (CI)" are verified on every pull request via automated workflows. Combinations marked "expected to work" are supported by the declared minimum requirements and standard OpenTelemetry compatibility guarantees, but are not currently included in the CI matrix. Untested combinations outside the ranges listed below may work but are not guaranteed.

---

## Python SDK Compatibility

Package name: `agentpulse` (PyPI)  
Current version: `0.1.0`  
Minimum Python requirement: `>=3.10`

| Python version | agentpulse version | opentelemetry-sdk version | Status |
|---|---|---|---|
| 3.10 | 0.1.0 | >=1.25.0 | expected to work |
| 3.11 | 0.1.0 | >=1.25.0 | expected to work |
| 3.12 | 0.1.0 | >=1.25.0 | tested (CI) |

> Python 3.12.3 is the active development version. The type checker (`mypy`) targets `python_version = "3.10"` to ensure broad compatibility.

---

## TypeScript SDK Compatibility

Package name: `@agentpulse/sdk` (npm)  
Current version: `0.1.0`  
Minimum Node.js requirement: `>=18` (declared in `package.json` `engines` field)

| Node.js version | @agentpulse/sdk version | @opentelemetry/sdk-node version | Status |
|---|---|---|---|
| 18 | 0.1.0 | ^1.25.0 | tested (CI) |
| 20 | 0.1.0 | ^1.25.0 | tested (CI) |
| 22 | 0.1.0 | ^1.25.0 | tested (CI) |

> All three Node.js versions are included in the `build-and-test` CI matrix defined in `.github/workflows/sdk-typescript.yml`. The Vercel AI and OpenAI compat jobs pin to Node 20.

---

## Framework Auto-Instrumentation Support

The table below covers framework integrations available via optional extras in the Python SDK and named exports in the TypeScript SDK. "Min version" is the minimum declared in `pyproject.toml` / `package.json`; "Max tested version" reflects what is pinned in the dev dependencies or CI.

| Framework | Language | Min version | Max tested version | Notes |
|---|---|---|---|---|
| LangChain (langchain-core) | Python | 0.2.0 | latest | Install with `pip install agentpulse[langchain]` |
| LangChain (@langchain/core) | TypeScript | 0.1.0 | 0.3.x | Declared as optional peer dep; dev dep pins `^0.3.0` |
| CrewAI | Python | 0.55.0 | latest | Install with `pip install agentpulse[crewai]` |
| AutoGen (autogen-agentchat) | Python | 0.4.0 | latest | Install with `pip install agentpulse[autogen]`; requires `packaging>=23.0` |
| LlamaIndex (llama-index-core) | Python | 0.10.68 | latest | Install with `pip install agentpulse[llamaindex]` |

> Integration modules are excluded from the default coverage check (see `[tool.coverage.run].omit` in `pyproject.toml`) because they require the optional framework to be installed. Each integration has its own test file under `sdk/python/tests/`.

---

## OTel Collector Compatibility

The AgentPulse collector speaks standard OTLP and is compatible with any OpenTelemetry-instrumented application regardless of language. It exposes two endpoints:

- **gRPC**: `:4317`
- **HTTP**: `:4318`

Any SDK that emits OTLP — whether the official `opentelemetry-sdk` for Python, `@opentelemetry/sdk-node` for TypeScript/JavaScript, the Java agent, the .NET SDK, or any other OTel-compatible library — can send spans to these endpoints without modification. No AgentPulse-specific SDK is required for basic trace ingestion; the AgentPulse SDKs add convenience helpers and agent-semantic attribute conventions on top of raw OTLP.

---

## Reporting a Compatibility Issue

If you encounter a version combination that does not work as expected, please open an issue in the AgentPulse GitHub repository. Include the SDK version, language runtime version, framework version (if applicable), the error message or unexpected behavior, and a minimal reproduction. This helps us expand the tested matrix and update this document.
