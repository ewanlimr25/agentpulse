ALTER TABLE alert_rules
  DROP COLUMN IF EXISTS slack_webhook_url,
  DROP COLUMN IF EXISTS discord_webhook_url,
  DROP COLUMN IF EXISTS last_channel_error,
  DROP COLUMN IF EXISTS last_channel_error_at;
