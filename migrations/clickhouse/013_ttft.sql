-- AgentPulse: add ttft_ms column for time-to-first-token (milliseconds).
-- Non-streaming spans have ttft_ms = 0.0 (default).
-- Computed by agentsemanticproc from stream.first_token SpanEvent.
-- Derivation: (stream.first_token event timestamp - span start timestamp) / 1e6
-- To backfill if formula changes: ALTER TABLE spans UPDATE ttft_ms = <formula> WHERE ttft_ms > 0
ALTER TABLE spans ADD COLUMN IF NOT EXISTS ttft_ms Float64 DEFAULT 0.0;
