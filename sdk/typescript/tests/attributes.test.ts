import { describe, it, expect } from 'vitest'
import * as path from 'node:path'
import * as fs from 'node:fs'
import * as yaml from 'js-yaml'
import * as attrs from '../src/generated/attributes.js'

interface AgentAttributesYaml {
  field_extraction: Record<string, string[]>
  cost?: { explicit_attribute?: string }
}

const YAML_PATH = path.resolve(__dirname, '../../../config/agent_attributes.yaml')

describe('attributes (generated from YAML)', () => {
  const doc = yaml.load(fs.readFileSync(YAML_PATH, 'utf8')) as AgentAttributesYaml
  const fe = doc.field_extraction

  it('exports SPAN_KIND matching Python SDK', () => {
    expect(attrs.SPAN_KIND).toBe('agentpulse.span_kind')
  })

  it('exports all 5 span kind constants', () => {
    expect(attrs.LLM_CALL).toBe('llm.call')
    expect(attrs.TOOL_CALL).toBe('tool.call')
    expect(attrs.AGENT_HANDOFF).toBe('agent.handoff')
    expect(attrs.MEMORY_READ).toBe('memory.read')
    expect(attrs.MEMORY_WRITE).toBe('memory.write')
  })

  it('PROJECT_ID matches YAML field_extraction.project_id[0]', () => {
    expect(attrs.PROJECT_ID).toBe(fe.project_id[0])
  })

  it('RUN_ID matches YAML field_extraction.run_id[0]', () => {
    expect(attrs.RUN_ID).toBe(fe.run_id[0])
  })

  it('MODEL_ID matches YAML field_extraction.model_id[0]', () => {
    expect(attrs.MODEL_ID).toBe(fe.model_id[0])
  })

  it('INPUT_TOKENS matches YAML field_extraction.input_tokens[0]', () => {
    expect(attrs.INPUT_TOKENS).toBe(fe.input_tokens[0])
  })

  it('OUTPUT_TOKENS matches YAML field_extraction.output_tokens[0]', () => {
    expect(attrs.OUTPUT_TOKENS).toBe(fe.output_tokens[0])
  })

  it('TOOL_NAME matches YAML field_extraction.tool_name[1]', () => {
    expect(attrs.TOOL_NAME).toBe(fe.tool_name[1])
  })

  it('HANDOFF_TARGET matches YAML field_extraction.handoff_target[0]', () => {
    expect(attrs.HANDOFF_TARGET).toBe(fe.handoff_target[0])
  })

  it('MEMORY_KEY matches YAML field_extraction.memory_key[0]', () => {
    expect(attrs.MEMORY_KEY).toBe(fe.memory_key[0])
  })

  it('COST_USD matches YAML cost.explicit_attribute', () => {
    expect(attrs.COST_USD).toBe(doc.cost?.explicit_attribute ?? 'agentpulse.cost_usd')
  })

  it('generated file has @generated header', () => {
    const content = fs.readFileSync(
      path.resolve(__dirname, '../src/generated/attributes.ts'),
      'utf8'
    )
    expect(content).toContain('@generated')
  })
})
