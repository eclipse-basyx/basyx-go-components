-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.16
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds durable PostgreSQL Large Object staging for asynchronous AASX uploads.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS aasx_async_upload (
  handle_id TEXT PRIMARY KEY REFERENCES async_job(handle_id) ON DELETE CASCADE,
  file_oid OID NOT NULL UNIQUE,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  promoted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION cleanup_aasx_async_upload_large_object()
RETURNS TRIGGER AS $$
BEGIN
  IF NOT OLD.promoted THEN
    PERFORM lo_unlink(OLD.file_oid);
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cleanup_aasx_async_upload_large_object ON aasx_async_upload;
CREATE TRIGGER trg_cleanup_aasx_async_upload_large_object
BEFORE DELETE ON aasx_async_upload
FOR EACH ROW
EXECUTE FUNCTION cleanup_aasx_async_upload_large_object();

CREATE OR REPLACE FUNCTION cleanup_terminal_aasx_async_upload()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.execution_state = 'Running' AND NEW.execution_state <> 'Running' THEN
    DELETE FROM aasx_async_upload WHERE handle_id = NEW.handle_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cleanup_terminal_aasx_async_upload ON async_job;
CREATE TRIGGER trg_cleanup_terminal_aasx_async_upload
AFTER UPDATE OF execution_state ON async_job
FOR EACH ROW
EXECUTE FUNCTION cleanup_terminal_aasx_async_upload();

UPDATE basyxsystem
SET schema_version = 'v1.1.16',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
