/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

-- Canonical binary payloads shared by File attachments and thumbnails.
CREATE TABLE IF NOT EXISTS binary_content (
  id BIGSERIAL PRIMARY KEY,
  sha256 CHAR(64) NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  file_oid OID NOT NULL,
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (sha256, size_bytes),
  UNIQUE (file_oid)
);

CREATE TABLE IF NOT EXISTS file_binary_reference (
  file_element_id BIGINT PRIMARY KEY REFERENCES file_element(id) ON DELETE CASCADE,
  binary_content_id BIGINT NOT NULL REFERENCES binary_content(id),
  path_token VARCHAR(64) NOT NULL,
  safe_file_name TEXT NOT NULL,
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
  safe_file_name TEXT NOT NULL,
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

-- Small chain head used when evidence is enabled independently of PostgreSQL
-- history. Model payloads remain exclusively in WORM/history payload storage.
CREATE TABLE IF NOT EXISTS mutation_evidence_state (
  entity_type TEXT NOT NULL,
  identifier TEXT NOT NULL,
  last_sequence BIGINT NOT NULL CHECK (last_sequence >= 0),
  last_event_hash CHAR(64),
  events_since_snapshot INTEGER NOT NULL DEFAULT 0 CHECK (events_since_snapshot >= 0),
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
  binary_content_id BIGINT REFERENCES binary_content(id) ON DELETE SET NULL,
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

CREATE OR REPLACE FUNCTION cleanup_unreferenced_binary_content()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  orphaned_oid OID;
BEGIN
  IF EXISTS (
    SELECT 1 FROM file_binary_reference WHERE binary_content_id = OLD.binary_content_id
    UNION ALL
    SELECT 1 FROM thumbnail_binary_reference WHERE binary_content_id = OLD.binary_content_id
  ) THEN
    RETURN OLD;
  END IF;

  DELETE FROM binary_content
  WHERE id = OLD.binary_content_id
  RETURNING file_oid INTO orphaned_oid;

  IF orphaned_oid IS NOT NULL THEN
    PERFORM lo_unlink(orphaned_oid);
  END IF;
  RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS cleanup_file_binary_content ON file_binary_reference;
CREATE TRIGGER cleanup_file_binary_content
AFTER DELETE ON file_binary_reference
FOR EACH ROW EXECUTE FUNCTION cleanup_unreferenced_binary_content();

DROP TRIGGER IF EXISTS cleanup_thumbnail_binary_content ON thumbnail_binary_reference;
CREATE TRIGGER cleanup_thumbnail_binary_content
AFTER DELETE ON thumbnail_binary_reference
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
