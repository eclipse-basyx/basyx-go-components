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

-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.3
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for WORM history evidence manifest catalogs.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- History evidence manifests and artifacts
-- ------------------------------------------

CREATE TABLE IF NOT EXISTS history_evidence_manifests (
  manifest_id BIGSERIAL PRIMARY KEY,
  manifest_version TEXT NOT NULL,
  history_table TEXT NOT NULL,
  identifier TEXT,
  first_history_id BIGINT NOT NULL,
  last_history_id BIGINT NOT NULL,
  first_row_hash TEXT NOT NULL,
  last_row_hash TEXT NOT NULL,
  row_count BIGINT NOT NULL CHECK (row_count > 0),
  range_digest TEXT NOT NULL,
  generated_at TIMESTAMPTZ NOT NULL,
  signature_state TEXT NOT NULL CHECK (signature_state IN ('signed', 'unsigned')),
  signer_key_id TEXT,
  signer_algorithm TEXT,
  snapshot_reference_count INTEGER NOT NULL DEFAULT 0 CHECK (snapshot_reference_count >= 0),
  provider TEXT NOT NULL,
  bucket TEXT,
  manifest_object_key TEXT NOT NULL,
  manifest_object_version_id TEXT,
  manifest_sha256 TEXT NOT NULL,
  retention_mode TEXT,
  retain_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CHECK (last_history_id >= first_history_id)
);

CREATE TABLE IF NOT EXISTS history_evidence_artifacts (
  artifact_id BIGSERIAL PRIMARY KEY,
  manifest_id BIGINT REFERENCES history_evidence_manifests(manifest_id),
  artifact_type TEXT NOT NULL CHECK (artifact_type IN ('manifest', 'snapshot', 'history_event')),
  history_table TEXT NOT NULL,
  identifier TEXT,
  history_id BIGINT,
  row_hash TEXT,
  content_hash TEXT,
  provider TEXT NOT NULL,
  bucket TEXT,
  object_key TEXT NOT NULL,
  object_version_id TEXT,
  sha256 TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  content_type TEXT NOT NULL,
  retention_mode TEXT,
  retain_until TIMESTAMPTZ,
  legal_hold BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_history_evidence_manifest_range
  ON history_evidence_manifests(
    history_table,
    COALESCE(identifier, ''),
    first_history_id,
    last_history_id,
    range_digest
  );

CREATE UNIQUE INDEX IF NOT EXISTS ux_history_evidence_artifact_object_manifest
  ON history_evidence_artifacts(
    manifest_id,
    provider,
    COALESCE(bucket, ''),
    object_key,
    COALESCE(object_version_id, '')
  )
  WHERE manifest_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_history_evidence_artifact_object_unlinked
  ON history_evidence_artifacts(
    provider,
    COALESCE(bucket, ''),
    object_key,
    COALESCE(object_version_id, '')
  )
  WHERE manifest_id IS NULL;

CREATE INDEX IF NOT EXISTS ix_history_evidence_manifest_table_identifier
  ON history_evidence_manifests(history_table, identifier, last_history_id DESC);

CREATE INDEX IF NOT EXISTS ix_history_evidence_artifact_manifest
  ON history_evidence_artifacts(manifest_id, artifact_type);

CREATE INDEX IF NOT EXISTS ix_history_evidence_artifact_history_row
  ON history_evidence_artifacts(history_table, identifier, history_id);

CREATE UNIQUE INDEX IF NOT EXISTS ux_history_evidence_artifact_history_event_manifest
  ON history_evidence_artifacts(
    manifest_id,
    history_table,
    COALESCE(identifier, ''),
    history_id,
    COALESCE(row_hash, '')
  )
  WHERE artifact_type = 'history_event'
    AND manifest_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_history_evidence_artifact_history_event_unlinked
  ON history_evidence_artifacts(
    history_table,
    COALESCE(identifier, ''),
    history_id,
    COALESCE(row_hash, '')
  )
  WHERE artifact_type = 'history_event'
    AND manifest_id IS NULL;

-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.3',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
