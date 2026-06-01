-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.1
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for version history
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- V3.2 version history / recent changes
-- ------------------------------------------

CREATE TABLE IF NOT EXISTS aas_history (
  history_id BIGSERIAL PRIMARY KEY,
  identifier TEXT NOT NULL,
  change_type TEXT NOT NULL,
  snapshot JSONB NOT NULL,
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
  previous_hash TEXT,
  content_hash TEXT,
  row_hash TEXT,
  signature TEXT,
  key_id TEXT,
  anchor_id TEXT,
  anchor_time TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS submodel_history (
  history_id BIGSERIAL PRIMARY KEY,
  identifier TEXT NOT NULL,
  change_type TEXT NOT NULL,
  snapshot JSONB NOT NULL,
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
  previous_hash TEXT,
  content_hash TEXT,
  row_hash TEXT,
  signature TEXT,
  key_id TEXT,
  anchor_id TEXT,
  anchor_time TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS concept_description_history (
  history_id BIGSERIAL PRIMARY KEY,
  identifier TEXT NOT NULL,
  change_type TEXT NOT NULL,
  snapshot JSONB NOT NULL,
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
  previous_hash TEXT,
  content_hash TEXT,
  row_hash TEXT,
  signature TEXT,
  key_id TEXT,
  anchor_id TEXT,
  anchor_time TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS descriptor_history (
  history_id BIGSERIAL PRIMARY KEY,
  identifier TEXT NOT NULL,
  change_type TEXT NOT NULL,
  snapshot JSONB NOT NULL,
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
  previous_hash TEXT,
  content_hash TEXT,
  row_hash TEXT,
  signature TEXT,
  key_id TEXT,
  anchor_id TEXT,
  anchor_time TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_aas_history_identifier_validity ON aas_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_aas_history_identifier_latest ON aas_history(identifier, history_id DESC);
CREATE INDEX IF NOT EXISTS ix_aas_history_recent ON aas_history(operation_time, history_id);
CREATE INDEX IF NOT EXISTS ix_aas_history_administration_created ON aas_history(administration_created_at, history_id);
CREATE INDEX IF NOT EXISTS ix_aas_history_administration_updated ON aas_history(administration_updated_at, history_id);
CREATE INDEX IF NOT EXISTS ix_aas_history_row_hash ON aas_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_submodel_history_identifier_validity ON submodel_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_submodel_history_identifier_latest ON submodel_history(identifier, history_id DESC);
CREATE INDEX IF NOT EXISTS ix_submodel_history_recent ON submodel_history(operation_time, history_id);
CREATE INDEX IF NOT EXISTS ix_submodel_history_administration_created ON submodel_history(administration_created_at, history_id);
CREATE INDEX IF NOT EXISTS ix_submodel_history_administration_updated ON submodel_history(administration_updated_at, history_id);
CREATE INDEX IF NOT EXISTS ix_submodel_history_row_hash ON submodel_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_cd_history_identifier_validity ON concept_description_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_cd_history_identifier_latest ON concept_description_history(identifier, history_id DESC);
CREATE INDEX IF NOT EXISTS ix_cd_history_recent ON concept_description_history(operation_time, history_id);
CREATE INDEX IF NOT EXISTS ix_cd_history_administration_created ON concept_description_history(administration_created_at, history_id);
CREATE INDEX IF NOT EXISTS ix_cd_history_administration_updated ON concept_description_history(administration_updated_at, history_id);
CREATE INDEX IF NOT EXISTS ix_cd_history_row_hash ON concept_description_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_identifier_validity ON descriptor_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_identifier_latest ON descriptor_history(identifier, history_id DESC);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_recent ON descriptor_history(operation_time, history_id);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_administration_created ON descriptor_history(administration_created_at, history_id);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_administration_updated ON descriptor_history(administration_updated_at, history_id);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_row_hash ON descriptor_history(row_hash);


-- PostgreSQL mutation guards are installed by the schema patch but disabled by
-- default. Services enable them at startup when history.immutability is
-- postgres_guarded. Normal service startup never disables an enabled guard.
CREATE TABLE IF NOT EXISTS history_guard_config (
  id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO history_guard_config (id, enabled)
VALUES (TRUE, FALSE)
ON CONFLICT (id) DO NOTHING;

CREATE OR REPLACE FUNCTION basyx_prevent_history_table_mutation()
RETURNS TRIGGER AS $$
DECLARE
  guard_enabled BOOLEAN;
BEGIN
  SELECT enabled INTO guard_enabled
  FROM history_guard_config
  WHERE id = TRUE;

  IF COALESCE(guard_enabled, FALSE) THEN
    RAISE EXCEPTION 'history tables are append-only'
      USING ERRCODE = '55000';
  END IF;

  IF TG_OP = 'UPDATE' THEN
    RETURN NEW;
  ELSIF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS aas_history_prevent_update_delete ON aas_history;
CREATE TRIGGER aas_history_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON aas_history
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS aas_history_prevent_truncate ON aas_history;
CREATE TRIGGER aas_history_prevent_truncate
  BEFORE TRUNCATE
  ON aas_history
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS submodel_history_prevent_update_delete ON submodel_history;
CREATE TRIGGER submodel_history_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON submodel_history
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS submodel_history_prevent_truncate ON submodel_history;
CREATE TRIGGER submodel_history_prevent_truncate
  BEFORE TRUNCATE
  ON submodel_history
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS concept_description_history_prevent_update_delete ON concept_description_history;
CREATE TRIGGER concept_description_history_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON concept_description_history
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS concept_description_history_prevent_truncate ON concept_description_history;
CREATE TRIGGER concept_description_history_prevent_truncate
  BEFORE TRUNCATE
  ON concept_description_history
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS descriptor_history_prevent_update_delete ON descriptor_history;
CREATE TRIGGER descriptor_history_prevent_update_delete
  BEFORE UPDATE OR DELETE
  ON descriptor_history
  FOR EACH ROW
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();

DROP TRIGGER IF EXISTS descriptor_history_prevent_truncate ON descriptor_history;
CREATE TRIGGER descriptor_history_prevent_truncate
  BEFORE TRUNCATE
  ON descriptor_history
  FOR EACH STATEMENT
  EXECUTE FUNCTION basyx_prevent_history_table_mutation();


-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.1',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
