-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.8
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch for deduplicated binary storage and history-independent
--   WORM evidence metadata.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- Canonical binary payloads shared by File attachments and thumbnails.
CREATE TABLE IF NOT EXISTS binary_content (
  id BIGSERIAL PRIMARY KEY,
  sha256 CHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  file_oid OID NOT NULL,
  reference_count BIGINT NOT NULL DEFAULT 0 CHECK (reference_count >= 0),
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (sha256, size_bytes),
  UNIQUE (file_oid)
);

CREATE TABLE IF NOT EXISTS file_binary_reference (
  file_element_id BIGINT PRIMARY KEY REFERENCES file_element(id) ON DELETE CASCADE,
  binary_content_id BIGINT NOT NULL REFERENCES binary_content(id),
  path_token VARCHAR(64) NOT NULL,
  safe_file_name TEXT NOT NULL CHECK (OCTET_LENGTH(safe_file_name) BETWEEN 1 AND 255),
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (path_token)
);

CREATE INDEX IF NOT EXISTS ix_file_binary_reference_content
  ON file_binary_reference(binary_content_id);

CREATE TABLE IF NOT EXISTS thumbnail_binary_reference (
  thumbnail_element_id BIGINT PRIMARY KEY REFERENCES thumbnail_file_element(id) ON DELETE CASCADE,
  binary_content_id BIGINT NOT NULL REFERENCES binary_content(id),
  path_token VARCHAR(64) NOT NULL,
  safe_file_name TEXT NOT NULL CHECK (OCTET_LENGTH(safe_file_name) BETWEEN 1 AND 255),
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (path_token)
);

CREATE INDEX IF NOT EXISTS ix_thumbnail_binary_reference_content
  ON thumbnail_binary_reference(binary_content_id);

-- One immutable WORM object receipt per canonical payload. Reference artifacts
-- remain per upload and are catalogued with their mutation evidence event.
CREATE TABLE IF NOT EXISTS binary_evidence_receipt (
  binary_content_id BIGINT UNIQUE REFERENCES binary_content(id) ON DELETE SET NULL,
  provider TEXT NOT NULL,
  bucket TEXT,
  object_key TEXT NOT NULL,
  object_version_id TEXT,
  sha256 CHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  content_type TEXT NOT NULL,
  retention_mode TEXT,
  retain_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (sha256, size_bytes)
);

-- Bounded chain head used when evidence is enabled independently of PostgreSQL
-- history. Only the current checkpoint is cached; prior states remain in WORM.
CREATE TABLE IF NOT EXISTS mutation_evidence_state (
  entity_type TEXT NOT NULL,
  identifier TEXT NOT NULL,
  last_sequence BIGINT NOT NULL CHECK (last_sequence >= 0),
  last_event_hash CHAR(64),
  last_content_hash CHAR(64),
  events_since_snapshot INTEGER NOT NULL DEFAULT 0 CHECK (events_since_snapshot >= 0),
  current_snapshot JSONB,
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (entity_type, identifier)
);

CREATE TABLE IF NOT EXISTS mutation_evidence_artifacts (
  artifact_id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  identifier TEXT NOT NULL,
  event_sequence BIGINT NOT NULL CHECK (event_sequence > 0),
  event_hash CHAR(64) NOT NULL,
  previous_event_hash CHAR(64),
  content_hash CHAR(64) NOT NULL,
  payload_hash CHAR(64) NOT NULL,
  payload_type TEXT NOT NULL CHECK (payload_type IN ('snapshot', 'diff')),
  history_table TEXT,
  history_id BIGINT,
  history_row_hash CHAR(64),
  provider TEXT NOT NULL,
  bucket TEXT,
  object_key TEXT NOT NULL,
  object_version_id TEXT,
  sha256 CHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  content_type TEXT NOT NULL,
  retention_mode TEXT,
  retain_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (entity_type, identifier, event_sequence),
  UNIQUE (entity_type, identifier, event_hash)
);

CREATE INDEX IF NOT EXISTS ix_mutation_evidence_artifacts_history
  ON mutation_evidence_artifacts(history_table, history_id);

CREATE TABLE IF NOT EXISTS binary_reference_evidence_artifacts (
  artifact_id BIGSERIAL PRIMARY KEY,
  mutation_artifact_id BIGINT NOT NULL REFERENCES mutation_evidence_artifacts(artifact_id) ON DELETE CASCADE,
  model_path TEXT NOT NULL,
  provider TEXT NOT NULL,
  bucket TEXT,
  object_key TEXT NOT NULL,
  object_version_id TEXT,
  sha256 CHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  content_type TEXT NOT NULL,
  retention_mode TEXT,
  retain_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (mutation_artifact_id, model_path)
);

CREATE OR REPLACE FUNCTION unlink_binary_content_when_unreferenced(content_id BIGINT)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
  orphaned_oid OID;
  remaining_references BIGINT;
BEGIN
  UPDATE binary_content
  SET reference_count = reference_count - 1
  WHERE id = content_id
  RETURNING reference_count INTO remaining_references;

  IF remaining_references = 0 THEN
    DELETE FROM binary_content
    WHERE id = content_id
    RETURNING file_oid INTO orphaned_oid;
  END IF;

  IF orphaned_oid IS NOT NULL THEN
    PERFORM lo_unlink(orphaned_oid);
  END IF;
  RETURN;
END;
$$;

CREATE OR REPLACE FUNCTION maintain_binary_content_reference_count()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    PERFORM id
    FROM binary_content
    WHERE id = NEW.binary_content_id
    FOR NO KEY UPDATE;

    UPDATE binary_content
    SET reference_count = reference_count + 1
    WHERE id = NEW.binary_content_id;
    RETURN NEW;
  END IF;

  IF TG_OP = 'UPDATE' AND OLD.binary_content_id IS DISTINCT FROM NEW.binary_content_id THEN
    PERFORM id
    FROM binary_content
    WHERE id IN (OLD.binary_content_id, NEW.binary_content_id)
    ORDER BY id
    FOR NO KEY UPDATE;

    UPDATE binary_content
    SET reference_count = reference_count + 1
    WHERE id = NEW.binary_content_id;
    PERFORM unlink_binary_content_when_unreferenced(OLD.binary_content_id);
    RETURN NEW;
  END IF;

  IF TG_OP = 'DELETE' THEN
    PERFORM id
    FROM binary_content
    WHERE id = OLD.binary_content_id
    FOR NO KEY UPDATE;

    PERFORM unlink_binary_content_when_unreferenced(OLD.binary_content_id);
    RETURN OLD;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS cleanup_file_binary_content ON file_binary_reference;
CREATE TRIGGER cleanup_file_binary_content
AFTER INSERT OR UPDATE OF binary_content_id OR DELETE ON file_binary_reference
FOR EACH ROW EXECUTE FUNCTION maintain_binary_content_reference_count();

DROP TRIGGER IF EXISTS cleanup_thumbnail_binary_content ON thumbnail_binary_reference;
CREATE TRIGGER cleanup_thumbnail_binary_content
AFTER INSERT OR UPDATE OF binary_content_id OR DELETE ON thumbnail_binary_reference
FOR EACH ROW EXECUTE FUNCTION maintain_binary_content_reference_count();

UPDATE basyxsystem
SET schema_version = 'v1.1.8',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
