BEGIN;

DROP TABLE IF EXISTS project_pii_configs;

ALTER TABLE projects DROP COLUMN IF EXISTS admin_key_hash;

COMMIT;
