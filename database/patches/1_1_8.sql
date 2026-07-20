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
-- history. Model snapshots remain in the live model and immutable evidence.
CREATE TABLE IF NOT EXISTS mutation_evidence_state (
  entity_type TEXT NOT NULL,
  identifier TEXT NOT NULL,
  identifier_digest CHAR(64) NOT NULL,
  last_sequence BIGINT NOT NULL CHECK (last_sequence >= 0),
  last_event_hash CHAR(64),
  last_content_hash CHAR(64),
  events_since_snapshot INTEGER NOT NULL DEFAULT 0 CHECK (events_since_snapshot >= 0),
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (entity_type, identifier_digest)
);

CREATE TABLE IF NOT EXISTS mutation_evidence_artifacts (
  artifact_id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  identifier TEXT NOT NULL,
  identifier_digest CHAR(64) NOT NULL,
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
  UNIQUE (entity_type, identifier_digest, event_sequence),
  UNIQUE (entity_type, identifier_digest, event_hash)
);

CREATE INDEX IF NOT EXISTS ix_mutation_evidence_artifacts_history
  ON mutation_evidence_artifacts(history_table, history_id)
  WHERE history_id IS NOT NULL;

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

CREATE OR REPLACE FUNCTION add_binary_content_references()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  reference_row RECORD;
BEGIN
  PERFORM id
  FROM binary_content
  WHERE id IN (SELECT DISTINCT binary_content_id FROM inserted_binary_references)
  ORDER BY id
  FOR NO KEY UPDATE;

  FOR reference_row IN
    SELECT binary_content_id, COUNT(*) AS reference_delta
    FROM inserted_binary_references
    GROUP BY binary_content_id
    ORDER BY binary_content_id
  LOOP
    UPDATE binary_content
    SET reference_count = reference_count + reference_row.reference_delta
    WHERE id = reference_row.binary_content_id;
  END LOOP;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION remove_binary_content_references()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  reference_row RECORD;
BEGIN
  PERFORM id
  FROM binary_content
  WHERE id IN (SELECT DISTINCT binary_content_id FROM deleted_binary_references)
  ORDER BY id
  FOR NO KEY UPDATE;

  FOR reference_row IN
    SELECT binary_content_id, COUNT(*) AS reference_delta
    FROM deleted_binary_references
    GROUP BY binary_content_id
    ORDER BY binary_content_id
  LOOP
    UPDATE binary_content
    SET reference_count = reference_count - reference_row.reference_delta
    WHERE id = reference_row.binary_content_id;
  END LOOP;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION replace_binary_content_references()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  reference_row RECORD;
BEGIN
  PERFORM id
  FROM binary_content
  WHERE id IN (
    SELECT binary_content_id FROM inserted_binary_references
    UNION
    SELECT binary_content_id FROM deleted_binary_references
  )
  ORDER BY id
  FOR NO KEY UPDATE;

  FOR reference_row IN
    WITH reference_deltas AS (
      SELECT binary_content_id, COUNT(*)::BIGINT AS reference_delta
      FROM inserted_binary_references
      GROUP BY binary_content_id
      UNION ALL
      SELECT binary_content_id, -COUNT(*)::BIGINT AS reference_delta
      FROM deleted_binary_references
      GROUP BY binary_content_id
    )
    SELECT binary_content_id, SUM(reference_delta) AS reference_delta
    FROM reference_deltas
    GROUP BY binary_content_id
    HAVING SUM(reference_delta) <> 0
    ORDER BY binary_content_id
  LOOP
    UPDATE binary_content
    SET reference_count = reference_count + reference_row.reference_delta
    WHERE id = reference_row.binary_content_id;
  END LOOP;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION cleanup_unreferenced_binary_content()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  content_id BIGINT;
  orphaned_oid OID;
BEGIN
  FOR content_id IN
    SELECT DISTINCT candidate_id
    FROM unnest(ARRAY[
      CASE WHEN TG_OP <> 'INSERT' THEN OLD.binary_content_id END,
      CASE WHEN TG_OP <> 'DELETE' THEN NEW.binary_content_id END
    ]) AS candidate_id
    WHERE candidate_id IS NOT NULL
    ORDER BY candidate_id
  LOOP
    orphaned_oid := NULL;
    DELETE FROM binary_content
    WHERE id = content_id
      AND reference_count = 0
    RETURNING file_oid INTO orphaned_oid;
    IF orphaned_oid IS NOT NULL THEN
      PERFORM lo_unlink(orphaned_oid);
    END IF;
  END LOOP;
  RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS add_file_binary_content_reference ON file_binary_reference;
DROP TRIGGER IF EXISTS replace_file_binary_content_reference ON file_binary_reference;
DROP TRIGGER IF EXISTS remove_file_binary_content_reference ON file_binary_reference;
DROP TRIGGER IF EXISTS cleanup_unreferenced_file_binary_content ON file_binary_reference;
CREATE TRIGGER add_file_binary_content_reference
AFTER INSERT ON file_binary_reference
REFERENCING NEW TABLE AS inserted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION add_binary_content_references();
CREATE TRIGGER replace_file_binary_content_reference
AFTER UPDATE ON file_binary_reference
REFERENCING OLD TABLE AS deleted_binary_references NEW TABLE AS inserted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION replace_binary_content_references();
CREATE TRIGGER remove_file_binary_content_reference
AFTER DELETE ON file_binary_reference
REFERENCING OLD TABLE AS deleted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION remove_binary_content_references();
CREATE CONSTRAINT TRIGGER cleanup_unreferenced_file_binary_content
AFTER INSERT OR UPDATE OR DELETE ON file_binary_reference
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION cleanup_unreferenced_binary_content();

DROP TRIGGER IF EXISTS add_thumbnail_binary_content_reference ON thumbnail_binary_reference;
DROP TRIGGER IF EXISTS replace_thumbnail_binary_content_reference ON thumbnail_binary_reference;
DROP TRIGGER IF EXISTS remove_thumbnail_binary_content_reference ON thumbnail_binary_reference;
DROP TRIGGER IF EXISTS cleanup_unreferenced_thumbnail_binary_content ON thumbnail_binary_reference;
CREATE TRIGGER add_thumbnail_binary_content_reference
AFTER INSERT ON thumbnail_binary_reference
REFERENCING NEW TABLE AS inserted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION add_binary_content_references();
CREATE TRIGGER replace_thumbnail_binary_content_reference
AFTER UPDATE ON thumbnail_binary_reference
REFERENCING OLD TABLE AS deleted_binary_references NEW TABLE AS inserted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION replace_binary_content_references();
CREATE TRIGGER remove_thumbnail_binary_content_reference
AFTER DELETE ON thumbnail_binary_reference
REFERENCING OLD TABLE AS deleted_binary_references
FOR EACH STATEMENT EXECUTE FUNCTION remove_binary_content_references();
CREATE CONSTRAINT TRIGGER cleanup_unreferenced_thumbnail_binary_content
AFTER INSERT OR UPDATE OR DELETE ON thumbnail_binary_reference
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION cleanup_unreferenced_binary_content();

UPDATE basyxsystem
SET schema_version = 'v1.1.8',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
