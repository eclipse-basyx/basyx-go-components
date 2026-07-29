-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.10
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Persists asynchronous jobs, their handles, and terminal results.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS async_job (
  handle_id TEXT PRIMARY KEY,
  manager_key TEXT NOT NULL,
  job_kind TEXT NOT NULL,
  execution_state TEXT NOT NULL CHECK (
    execution_state IN ('Initiated', 'Running', 'Completed', 'Canceled', 'Failed', 'Timeout')
  ),
  owner_key TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  bulk_result JSONB,
  result_payload JSONB,
  error_status INTEGER NOT NULL DEFAULT 0,
  error_payload JSONB,
  worker_id TEXT NOT NULL,
  lease_expires_at TIMESTAMPTZ,
  execution_deadline TIMESTAMPTZ,
  terminal_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS ix_async_job_owner
  ON async_job(handle_id, owner_key);
CREATE INDEX IF NOT EXISTS ix_async_job_recovery
  ON async_job(manager_key, execution_state, lease_expires_at, execution_deadline);
CREATE INDEX IF NOT EXISTS ix_async_job_cleanup
  ON async_job(manager_key, expires_at)
  WHERE execution_state <> 'Running';

UPDATE basyxsystem
SET schema_version = 'v1.1.10',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
