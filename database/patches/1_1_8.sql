/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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
  binary_content_id BIGINT PRIMARY KEY REFERENCES binary_content(id) ON DELETE CASCADE,
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
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
  binary_content_id BIGINT NOT NULL REFERENCES binary_content(id),
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

UPDATE basyxsystem
SET schema_version = 'v1.1.8',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
