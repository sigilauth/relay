-- Rollback initial schema

BEGIN;

DROP TABLE IF EXISTS server_registry;
DROP TABLE IF EXISTS device_push_tokens;

COMMIT;
