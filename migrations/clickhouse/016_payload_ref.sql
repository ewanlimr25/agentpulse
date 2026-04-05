-- 016_payload_ref: dedicated column for S3 offloaded span payloads.
-- When gen_ai.prompt / gen_ai.completion / tool.input / tool.output exceed
-- the configured threshold, the collector writes those fields to S3 and stores
-- the S3 object key here instead of in the attributes map.
-- An empty string means the span was not offloaded (the common case).

-- Primary storage column: written by the collector exporter.
ALTER TABLE spans ADD COLUMN IF NOT EXISTS payload_s3_key String DEFAULT '';

-- Observability column: MATERIALIZED from the attributes map flag.
-- The collector stamps attributes['agentpulse.payload_offloaded'] = 'true' on offloaded spans.
-- Using MATERIALIZED (not DEFAULT) so the value is frozen at insert time and
-- cannot be manually overridden.
ALTER TABLE spans ADD COLUMN IF NOT EXISTS payload_offloaded Bool
    MATERIALIZED attributes['agentpulse.payload_offloaded'] = 'true';

-- Bloom filter index to enable efficient WHERE payload_s3_key = '' filtering.
-- Used by: search_store (exclude offloaded from full-text search),
--          loopdetect queries (exclude offloaded from hash-based loop detection).
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_payload_s3_key payload_s3_key TYPE bloom_filter GRANULARITY 1;
