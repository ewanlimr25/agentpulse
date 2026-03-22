/**
 * LLM usage recording helper.
 * Call inside a llmCall() wrapper after the LLM responds.
 */

import type { Span } from '@opentelemetry/api'
import * as attrs from './generated/attributes.js'

export interface LlmUsage {
  inputTokens: number
  outputTokens: number
  prompt?: string
  completion?: string
  costUsd?: number
}

export function recordLlmUsage(span: Span, usage: LlmUsage): void {
  span.setAttribute(attrs.INPUT_TOKENS, usage.inputTokens)
  span.setAttribute(attrs.OUTPUT_TOKENS, usage.outputTokens)
  if (usage.prompt !== undefined) span.setAttribute(attrs.PROMPT, usage.prompt)
  if (usage.completion !== undefined) span.setAttribute(attrs.COMPLETION, usage.completion)
  if (usage.costUsd !== undefined) span.setAttribute(attrs.COST_USD, usage.costUsd)
}
