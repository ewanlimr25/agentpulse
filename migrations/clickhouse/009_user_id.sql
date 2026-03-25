-- AgentPulse: add user_id to spans
-- Groups spans by the end-user/customer who triggered the run.
-- Uses bloom filter index for efficient filtering (applies to new data parts only;
-- queries scope to start_time >= migration timestamp to skip pre-index parts).

ALTER TABLE spans ADD COLUMN IF NOT EXISTS user_id String DEFAULT '';
ALTER TABLE spans ADD INDEX IF NOT EXISTS idx_user_id user_id TYPE bloom_filter GRANULARITY 1;
