-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.1
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds durable event delivery queues with per-entity ordering and retries.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS event_outbox (
  seq BIGSERIAL PRIMARY KEY,
  sink_id TEXT NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  ordering_key TEXT NOT NULL,
  envelope TEXT NOT NULL,
  routing JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (sink_id, event_id)
);

CREATE INDEX IF NOT EXISTS ix_event_outbox_ordering
ON event_outbox (sink_id, ordering_key, seq);
CREATE INDEX IF NOT EXISTS ix_event_outbox_ready
ON event_outbox (sink_id, next_attempt_at, seq);
CREATE INDEX IF NOT EXISTS ix_event_outbox_age
ON event_outbox (sink_id, created_at);

UPDATE basyxsystem
SET schema_version = 'v1.2.1',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
