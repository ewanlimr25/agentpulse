-- AgentPulse: add session_id to spans
-- Groups multiple runs into a conversation/session.
-- Uses bloom filter index for efficient filtering (applies to new data parts only;
-- queries scope to start_time >= migration timestamp to skip pre-index parts).

ALTER TABLE spans ADD COLUMN IF NOT EXISTS session_id String DEFAULT '';
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_session_id session_id TYPE bloom_filter GRANULARITY 1;
