import type { Span } from '@opentelemetry/api'

/**
 * Record the moment the first streaming token arrives.
 *
 * Call exactly once per streaming LLM span as soon as the first token
 * is yielded. Stamps agentpulse.ttft_ms directly on the span as a
 * numeric attribute (for serverless environments where event timestamps
 * may be unreliable), AND adds a stream.first_token SpanEvent for the
 * collector's event-based computation path.
 *
 * Uses a guard attribute to silently no-op if called more than once.
 *
 * @param span - The active LLM span (from llmCall or a manual tracer.startSpan).
 * @param spanStartTimeMs - The wall-clock time (Date.now()) when the span started.
 *   Required to compute TTFT in serverless environments where OTel event timestamps
 *   may be unreliable due to clock skew.
 */
export function recordStreamFirstToken(span: Span, spanStartTimeMs: number): void {
  if (span.isRecording && !span.isRecording()) return
  // Guard: no-op on second call
  try {
    const attrs = (span as unknown as { attributes?: Record<string, unknown> }).attributes
    if (attrs && attrs['agentpulse._ttft_recorded']) return
  } catch {
    // attribute access failed — proceed
  }

  const now = Date.now()
  const ttftMs = Math.max(0, now - spanStartTimeMs)

  span.setAttribute('agentpulse._ttft_recorded', true)
  span.setAttribute('agentpulse.ttft_ms', ttftMs)
  span.addEvent('stream.first_token')
}
