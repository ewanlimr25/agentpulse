ALTER TABLE alert_rules
  ADD COLUMN slack_webhook_url     TEXT,
  ADD COLUMN discord_webhook_url   TEXT,
  ADD COLUMN last_channel_error    TEXT,
  ADD COLUMN last_channel_error_at TIMESTAMPTZ;
