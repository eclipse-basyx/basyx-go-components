-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.5
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for Submodel Registry descriptor history.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- Submodel Registry descriptor history
-- ------------------------------------------

CREATE TABLE IF NOT EXISTS submodel_descriptor_history (
  history_id BIGSERIAL PRIMARY KEY,
  identifier TEXT NOT NULL,
  change_type TEXT NOT NULL,
  deleted BOOLEAN NOT NULL DEFAULT FALSE,
  valid_from TIMESTAMPTZ NOT NULL,
  valid_to TIMESTAMPTZ,
  operation_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  administration_created_at_text TEXT,
  administration_updated_at_text TEXT,
  administration_created_at TIMESTAMPTZ,
  administration_updated_at TIMESTAMPTZ,
  actor_subject TEXT,
  actor_issuer TEXT,
  client_id TEXT,
  authorization_result TEXT,
  policy_id TEXT,
  matched_rule_id TEXT,
  request_id TEXT,
  correlation_id TEXT,
  source_ip INET,
  user_agent TEXT,
  operation TEXT,
  endpoint TEXT,
  http_method TEXT,
  payload_type TEXT NOT NULL DEFAULT 'snapshot' CHECK (payload_type IN ('snapshot', 'diff')),
  previous_hash TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  row_hash TEXT NOT NULL,
  signature TEXT,
  key_id TEXT,
  anchor_id TEXT,
  anchor_time TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_history_payload (
  history_id BIGINT PRIMARY KEY REFERENCES submodel_descriptor_history(history_id) ON DELETE CASCADE,
  snapshot JSONB,
  diff JSONB,
  CHECK ((snapshot IS NOT NULL AND diff IS NULL) OR (snapshot IS NULL AND diff IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS ix_smdesc_history_identifier_validity ON submodel_descriptor_history(identifier, valid_from DESC, history_id DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_identifier_latest ON submodel_descriptor_history(identifier, history_id DESC);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_recent ON submodel_descriptor_history(operation_time, history_id);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_administration_created ON submodel_descriptor_history(administration_created_at, history_id);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_administration_updated ON submodel_descriptor_history(administration_updated_at, history_id);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_row_hash ON submodel_descriptor_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_smdesc_history_snapshot_checkpoint
  ON submodel_descriptor_history(identifier, history_id DESC)
  WHERE payload_type = 'snapshot';

DROP TRIGGER IF EXISTS submodel_descriptor_history_prevent_update_delete ON submodel_descriptor_history;
CREATE TRIGGER submodel_descriptor_history_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON submodel_descriptor_history
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS submodel_descriptor_history_prevent_truncate ON submodel_descriptor_history;
CREATE TRIGGER submodel_descriptor_history_prevent_truncate
  BEFORE TRUNCATE
  ON submodel_descriptor_history
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS submodel_descriptor_history_payload_prevent_update_delete ON submodel_descriptor_history_payload;
CREATE TRIGGER submodel_descriptor_history_payload_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON submodel_descriptor_history_payload
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS submodel_descriptor_history_payload_prevent_truncate ON submodel_descriptor_history_payload;
CREATE TRIGGER submodel_descriptor_history_payload_prevent_truncate
  BEFORE TRUNCATE
  ON submodel_descriptor_history_payload
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.5',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
