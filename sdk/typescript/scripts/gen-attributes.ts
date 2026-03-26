#!/usr/bin/env tsx
/**
 * Codegen script: reads config/agent_attributes.yaml and emits
 * src/generated/attributes.ts with typed constants.
 *
 * Run: tsx scripts/gen-attributes.ts
 * Auto-run: npm run prebuild
 */

import * as fs from 'node:fs'
import * as path from 'node:path'
import * as yaml from 'js-yaml'

const REPO_ROOT = path.resolve(__dirname, '../../..')
const YAML_PATH = path.join(REPO_ROOT, 'config', 'agent_attributes.yaml')
const OUT_PATH = path.resolve(__dirname, '../src/generated/attributes.ts')

interface AgentAttributesYaml {
  field_extraction: Record<string, string[]>
  cost?: { explicit_attribute?: string }
}

function firstEntry(list: string[]): string {
  return list[0]
}

async function main() {
  const raw = fs.readFileSync(YAML_PATH, 'utf8')
  const doc = yaml.load(raw) as AgentAttributesYaml
  const fe = doc.field_extraction

  const lines: string[] = [
    '// @generated -- DO NOT EDIT. Source: config/agent_attributes.yaml',
    '// Run `npm run codegen` or `npm run build` to regenerate.',
    '',
    '// ── Span kind ──────────────────────────────────────────────────────────────',
    '',
    '/** Attribute key used to classify span kind. Set this on all AgentPulse spans. */',
    'export const SPAN_KIND = "agentpulse.span_kind" as const',
    '',
    'export type AgentSpanKind =',
    '  | "llm.call"',
    '  | "tool.call"',
    '  | "agent.handoff"',
    '  | "memory.read"',
    '  | "memory.write"',
    '  | "mcp.tool_call"',
    '  | "mcp.list_tools"',
    '',
    'export const LLM_CALL: AgentSpanKind = "llm.call"',
    'export const TOOL_CALL: AgentSpanKind = "tool.call"',
    'export const AGENT_HANDOFF: AgentSpanKind = "agent.handoff"',
    'export const MEMORY_READ: AgentSpanKind = "memory.read"',
    'export const MEMORY_WRITE: AgentSpanKind = "memory.write"',
    'export const MCP_TOOL_CALL: AgentSpanKind = "mcp.tool_call"',
    'export const MCP_LIST_TOOLS: AgentSpanKind = "mcp.list_tools"',
    '',
    '// ── Identity ────────────────────────────────────────────────────────────────',
    '',
    `export const PROJECT_ID = ${JSON.stringify(firstEntry(fe.project_id))} as const`,
    `export const RUN_ID = ${JSON.stringify(firstEntry(fe.run_id))} as const`,
    `export const SESSION_ID = ${JSON.stringify(firstEntry(fe.session_id))} as const`,
    `export const USER_ID = ${JSON.stringify(firstEntry(fe.user_id))} as const`,
    `export const AGENT_NAME = ${JSON.stringify(fe.agent_name[2])} as const`,
    '',
    '// ── LLM ─────────────────────────────────────────────────────────────────────',
    '',
    `export const MODEL_ID = ${JSON.stringify(firstEntry(fe.model_id))} as const`,
    `export const INPUT_TOKENS = ${JSON.stringify(firstEntry(fe.input_tokens))} as const`,
    `export const OUTPUT_TOKENS = ${JSON.stringify(firstEntry(fe.output_tokens))} as const`,
    `export const PROMPT = "gen_ai.prompt" as const`,
    `export const COMPLETION = "gen_ai.completion" as const`,
    `export const COST_USD = ${JSON.stringify(doc.cost?.explicit_attribute ?? 'agentpulse.cost_usd')} as const`,
    '',
    '// ── Tool ────────────────────────────────────────────────────────────────────',
    '',
    `export const TOOL_NAME = ${JSON.stringify(fe.tool_name[1])} as const`,
    '',
    '// ── Handoff ─────────────────────────────────────────────────────────────────',
    '',
    `export const HANDOFF_TARGET = ${JSON.stringify(firstEntry(fe.handoff_target))} as const`,
    '',
    '// ── Memory ──────────────────────────────────────────────────────────────────',
    '',
    `export const MEMORY_KEY = ${JSON.stringify(firstEntry(fe.memory_key))} as const`,
    '',
    '// ── Streaming ────────────────────────────────────────────────────────────────',
    '',
    `export const TTFT_MS = "agentpulse.ttft_ms" as const`,
    '',
    '// ── MCP (Model Context Protocol) ─────────────────────────────────────────────',
    '',
    `export const MCP_SERVER_NAME = ${JSON.stringify(firstEntry(fe.mcp_server_name))} as const`,
    `export const MCP_TOOL_NAME = ${JSON.stringify(firstEntry(fe.mcp_tool_name))} as const`,
    `export const MCP_INPUT_SCHEMA = ${JSON.stringify(firstEntry(fe.mcp_input_schema))} as const`,
    `export const MCP_OUTPUT_SCHEMA = ${JSON.stringify(firstEntry(fe.mcp_output_schema))} as const`,
    `export const MCP_TOOL_COUNT = ${JSON.stringify(firstEntry(fe.mcp_tool_count))} as const`,
    `export const MCP_DISCOVERED_TOOLS = ${JSON.stringify(firstEntry(fe.mcp_discovered_tools))} as const`,
  ]

  const outDir = path.dirname(OUT_PATH)
  if (!fs.existsSync(outDir)) {
    fs.mkdirSync(outDir, { recursive: true })
  }

  fs.writeFileSync(OUT_PATH, lines.join('\n') + '\n', 'utf8')
  console.log(`[gen-attributes] Written to ${OUT_PATH}`)
}

main().catch(err => {
  console.error('[gen-attributes] Error:', err)
  process.exit(1)
})
