-- Initial relay database schema
-- PostgreSQL 14+
-- Per cascade-data-architecture.md §2.2

BEGIN;

-- Device push tokens table
-- Stores fingerprint → push_token mapping
CREATE TABLE device_push_tokens (
  fingerprint TEXT PRIMARY KEY,
  push_token TEXT NOT NULL,
  platform TEXT NOT NULL CHECK (platform IN ('apns', 'fcm')),
  registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_delivered_at TIMESTAMPTZ,
  delivery_failures INT NOT NULL DEFAULT 0,
  CONSTRAINT fingerprint_hex CHECK (fingerprint ~ '^[a-f0-9]{64}$')
);

CREATE INDEX idx_push_tokens_updated ON device_push_tokens(updated_at);
CREATE INDEX idx_push_tokens_failures ON device_push_tokens(delivery_failures) WHERE delivery_failures > 0;

-- Server registry for signature verification
-- Stores known Sigil server public keys
CREATE TABLE server_registry (
  server_id TEXT PRIMARY KEY,
  server_public_key BYTEA NOT NULL,
  server_name TEXT,
  registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_push_at TIMESTAMPTZ,
  CONSTRAINT server_public_key_length CHECK (octet_length(server_public_key) = 33)
);

CREATE INDEX idx_server_last_push ON server_registry(last_push_at);

COMMIT;
