-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.0
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for AAS metamodel V3.2 compatibility.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- AdministrativeInformation V3.2 timestamps
-- ------------------------------------------

-- V3.2 adds AdministrativeInformation/createdAt and /updatedAt.
-- The existing schema keeps AdministrativeInformation as JSONB payloads, so
-- nullable scalar columns are added beside those payloads for efficient
-- filtering and version-date lookups while preserving the original JSON.
-- SubmodelElement is not Identifiable and has no AdministrativeInformation;
-- remove the obsolete payload column from older schema versions.
ALTER TABLE IF EXISTS submodel_element_payload
  DROP COLUMN IF EXISTS administrative_information_payload;

ALTER TABLE IF EXISTS aas_payload
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS submodel_payload
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS descriptor_payload
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS concept_description
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

-- Extract an optional JSON string timestamp defensively. Invalid or missing
-- future payload values stay NULL instead of aborting writes.
CREATE OR REPLACE FUNCTION basyx_jsonb_timestamptz(payload JSONB, field_name TEXT)
RETURNS TIMESTAMPTZ AS $$
DECLARE
  raw_value TEXT;
BEGIN
  IF payload IS NULL OR jsonb_typeof(payload) <> 'object' THEN
    RETURN NULL;
  END IF;

  raw_value := payload ->> field_name;
  IF raw_value IS NULL OR btrim(raw_value) = '' THEN
    RETURN NULL;
  END IF;

  BEGIN
    RETURN raw_value::TIMESTAMPTZ;
  EXCEPTION WHEN OTHERS THEN
    RETURN NULL;
  END;
END;
$$ LANGUAGE plpgsql;

-- Common trigger function for tables whose administrative information is stored
-- directly in administrative_information_payload.
CREATE OR REPLACE FUNCTION sync_administrative_information_timestamps()
RETURNS TRIGGER AS $$
BEGIN
  NEW.administration_created_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'createdAt');
  NEW.administration_updated_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'updatedAt');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Concept descriptions store the complete metamodel object in data, so their
-- AdministrativeInformation timestamps live below data->'administration'.
CREATE OR REPLACE FUNCTION sync_concept_description_administration_timestamps()
RETURNS TRIGGER AS $$
BEGIN
  NEW.administration_created_at = basyx_jsonb_timestamptz(NEW.data -> 'administration', 'createdAt');
  NEW.administration_updated_at = basyx_jsonb_timestamptz(NEW.data -> 'administration', 'updatedAt');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Keep the scalar timestamp columns in sync for new and updated payloads.
-- Triggers are dropped first to make the patch idempotent and allow function
-- replacement on repeated deployments.
DROP TRIGGER IF EXISTS aas_payload_sync_administration_timestamps ON aas_payload;
CREATE TRIGGER aas_payload_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF administrative_information_payload
  ON aas_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_administrative_information_timestamps();

DROP TRIGGER IF EXISTS submodel_payload_sync_administration_timestamps ON submodel_payload;
CREATE TRIGGER submodel_payload_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF administrative_information_payload
  ON submodel_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_administrative_information_timestamps();

DROP TRIGGER IF EXISTS descriptor_payload_sync_administration_timestamps ON descriptor_payload;
CREATE TRIGGER descriptor_payload_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF administrative_information_payload
  ON descriptor_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_administrative_information_timestamps();

DROP TRIGGER IF EXISTS concept_description_sync_administration_timestamps ON concept_description;
CREATE TRIGGER concept_description_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF data
  ON concept_description
  FOR EACH ROW
  EXECUTE FUNCTION sync_concept_description_administration_timestamps();

-- Index the extracted timestamps for V3.2 recent-change and version-date
-- access patterns without querying deep JSONB expressions.
CREATE INDEX IF NOT EXISTS ix_aas_payload_admin_created_at ON aas_payload(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_aas_payload_admin_updated_at ON aas_payload(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_submodel_payload_admin_created_at ON submodel_payload(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_submodel_payload_admin_updated_at ON submodel_payload(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_descriptor_payload_admin_created_at ON descriptor_payload(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_descriptor_payload_admin_updated_at ON descriptor_payload(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_cd_admin_created_at ON concept_description(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_cd_admin_updated_at ON concept_description(administration_updated_at);

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

ALTER TABLE IF EXISTS aas_history
  ADD COLUMN IF NOT EXISTS actor_subject TEXT,
  ADD COLUMN IF NOT EXISTS actor_issuer TEXT,
  ADD COLUMN IF NOT EXISTS client_id TEXT,
  ADD COLUMN IF NOT EXISTS authorization_result TEXT,
  ADD COLUMN IF NOT EXISTS policy_id TEXT,
  ADD COLUMN IF NOT EXISTS matched_rule_id TEXT,
  ADD COLUMN IF NOT EXISTS request_id TEXT,
  ADD COLUMN IF NOT EXISTS correlation_id TEXT,
  ADD COLUMN IF NOT EXISTS source_ip INET,
  ADD COLUMN IF NOT EXISTS user_agent TEXT,
  ADD COLUMN IF NOT EXISTS operation TEXT,
  ADD COLUMN IF NOT EXISTS endpoint TEXT,
  ADD COLUMN IF NOT EXISTS http_method TEXT,
  ADD COLUMN IF NOT EXISTS previous_hash TEXT,
  ADD COLUMN IF NOT EXISTS content_hash TEXT,
  ADD COLUMN IF NOT EXISTS row_hash TEXT,
  ADD COLUMN IF NOT EXISTS signature TEXT,
  ADD COLUMN IF NOT EXISTS key_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_time TIMESTAMPTZ;

ALTER TABLE IF EXISTS submodel_history
  ADD COLUMN IF NOT EXISTS actor_subject TEXT,
  ADD COLUMN IF NOT EXISTS actor_issuer TEXT,
  ADD COLUMN IF NOT EXISTS client_id TEXT,
  ADD COLUMN IF NOT EXISTS authorization_result TEXT,
  ADD COLUMN IF NOT EXISTS policy_id TEXT,
  ADD COLUMN IF NOT EXISTS matched_rule_id TEXT,
  ADD COLUMN IF NOT EXISTS request_id TEXT,
  ADD COLUMN IF NOT EXISTS correlation_id TEXT,
  ADD COLUMN IF NOT EXISTS source_ip INET,
  ADD COLUMN IF NOT EXISTS user_agent TEXT,
  ADD COLUMN IF NOT EXISTS operation TEXT,
  ADD COLUMN IF NOT EXISTS endpoint TEXT,
  ADD COLUMN IF NOT EXISTS http_method TEXT,
  ADD COLUMN IF NOT EXISTS previous_hash TEXT,
  ADD COLUMN IF NOT EXISTS content_hash TEXT,
  ADD COLUMN IF NOT EXISTS row_hash TEXT,
  ADD COLUMN IF NOT EXISTS signature TEXT,
  ADD COLUMN IF NOT EXISTS key_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_time TIMESTAMPTZ;

ALTER TABLE IF EXISTS concept_description_history
  ADD COLUMN IF NOT EXISTS actor_subject TEXT,
  ADD COLUMN IF NOT EXISTS actor_issuer TEXT,
  ADD COLUMN IF NOT EXISTS client_id TEXT,
  ADD COLUMN IF NOT EXISTS authorization_result TEXT,
  ADD COLUMN IF NOT EXISTS policy_id TEXT,
  ADD COLUMN IF NOT EXISTS matched_rule_id TEXT,
  ADD COLUMN IF NOT EXISTS request_id TEXT,
  ADD COLUMN IF NOT EXISTS correlation_id TEXT,
  ADD COLUMN IF NOT EXISTS source_ip INET,
  ADD COLUMN IF NOT EXISTS user_agent TEXT,
  ADD COLUMN IF NOT EXISTS operation TEXT,
  ADD COLUMN IF NOT EXISTS endpoint TEXT,
  ADD COLUMN IF NOT EXISTS http_method TEXT,
  ADD COLUMN IF NOT EXISTS previous_hash TEXT,
  ADD COLUMN IF NOT EXISTS content_hash TEXT,
  ADD COLUMN IF NOT EXISTS row_hash TEXT,
  ADD COLUMN IF NOT EXISTS signature TEXT,
  ADD COLUMN IF NOT EXISTS key_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_time TIMESTAMPTZ;

ALTER TABLE IF EXISTS descriptor_history
  ADD COLUMN IF NOT EXISTS actor_subject TEXT,
  ADD COLUMN IF NOT EXISTS actor_issuer TEXT,
  ADD COLUMN IF NOT EXISTS client_id TEXT,
  ADD COLUMN IF NOT EXISTS authorization_result TEXT,
  ADD COLUMN IF NOT EXISTS policy_id TEXT,
  ADD COLUMN IF NOT EXISTS matched_rule_id TEXT,
  ADD COLUMN IF NOT EXISTS request_id TEXT,
  ADD COLUMN IF NOT EXISTS correlation_id TEXT,
  ADD COLUMN IF NOT EXISTS source_ip INET,
  ADD COLUMN IF NOT EXISTS user_agent TEXT,
  ADD COLUMN IF NOT EXISTS operation TEXT,
  ADD COLUMN IF NOT EXISTS endpoint TEXT,
  ADD COLUMN IF NOT EXISTS http_method TEXT,
  ADD COLUMN IF NOT EXISTS previous_hash TEXT,
  ADD COLUMN IF NOT EXISTS content_hash TEXT,
  ADD COLUMN IF NOT EXISTS row_hash TEXT,
  ADD COLUMN IF NOT EXISTS signature TEXT,
  ADD COLUMN IF NOT EXISTS key_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_id TEXT,
  ADD COLUMN IF NOT EXISTS anchor_time TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS ix_aas_history_identifier_validity ON aas_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_aas_history_recent ON aas_history(history_id, operation_time);
CREATE INDEX IF NOT EXISTS ix_aas_history_row_hash ON aas_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_submodel_history_identifier_validity ON submodel_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_submodel_history_recent ON submodel_history(history_id, operation_time);
CREATE INDEX IF NOT EXISTS ix_submodel_history_row_hash ON submodel_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_cd_history_identifier_validity ON concept_description_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_cd_history_recent ON concept_description_history(history_id, operation_time);
CREATE INDEX IF NOT EXISTS ix_cd_history_row_hash ON concept_description_history(row_hash);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_identifier_validity ON descriptor_history(identifier, valid_from DESC, valid_to);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_recent ON descriptor_history(history_id, operation_time);
CREATE INDEX IF NOT EXISTS ix_descriptor_history_row_hash ON descriptor_history(row_hash);

-- PostgreSQL mutation guards are installed by the schema patch but disabled by
-- default. Services enable them at startup when history.immutability is
-- postgres_guarded or external_anchor.
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

-- V3.2 inserts "Batch" at enum index 2 for asset kind. Existing persisted numeric enum values
-- from V3.1.1 with index >= 2 must be shifted by +1 to preserve semantic value.
UPDATE asset_information
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;

UPDATE aas_descriptor
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;

-- Backfill one open history version for already existing current records.
-- Runtime writes keep these tables append-only after the upgrade.
INSERT INTO aas_history (
  identifier,
  change_type,
  snapshot,
  deleted,
  valid_from,
  administration_created_at_text,
  administration_updated_at_text
)
SELECT
  a.aas_id,
  'Created',
  jsonb_strip_nulls(jsonb_build_object(
    'id', a.aas_id,
    'idShort', a.id_short,
    'category', a.category,
    'modelType', 'AssetAdministrationShell',
    'administration', ap.administrative_information_payload,
    'assetInformation', jsonb_strip_nulls(jsonb_build_object(
      'assetKind', CASE ai.asset_kind
        WHEN 0 THEN 'Type'
        WHEN 1 THEN 'Instance'
        WHEN 2 THEN 'Batch'
        WHEN 3 THEN 'Role'
        ELSE 'Instance'
      END,
      'globalAssetId', ai.global_asset_id,
      'assetType', ai.asset_type
    ))
  )),
  FALSE,
  COALESCE(a.db_created_at, NOW()),
  ap.administrative_information_payload ->> 'createdAt',
  ap.administrative_information_payload ->> 'updatedAt'
FROM aas a
LEFT JOIN aas_payload ap ON ap.aas_id = a.id
LEFT JOIN asset_information ai ON ai.asset_information_id = a.id
WHERE NOT EXISTS (
  SELECT 1 FROM aas_history ah WHERE ah.identifier = a.aas_id
);

INSERT INTO submodel_history (
  identifier,
  change_type,
  snapshot,
  deleted,
  valid_from,
  administration_created_at_text,
  administration_updated_at_text
)
SELECT
  s.submodel_identifier,
  'Created',
  jsonb_strip_nulls(jsonb_build_object(
    'id', s.submodel_identifier,
    'idShort', s.id_short,
    'category', s.category,
    'kind', CASE s.kind
      WHEN 0 THEN 'Instance'
      WHEN 1 THEN 'Template'
      ELSE NULL
    END,
    'modelType', 'Submodel',
    'administration', sp.administrative_information_payload
  )),
  FALSE,
  COALESCE(s.db_created_at, NOW()),
  sp.administrative_information_payload ->> 'createdAt',
  sp.administrative_information_payload ->> 'updatedAt'
FROM submodel s
LEFT JOIN submodel_payload sp ON sp.submodel_id = s.id
WHERE NOT EXISTS (
  SELECT 1 FROM submodel_history sh WHERE sh.identifier = s.submodel_identifier
);

INSERT INTO concept_description_history (
  identifier,
  change_type,
  snapshot,
  deleted,
  valid_from,
  administration_created_at_text,
  administration_updated_at_text
)
SELECT
  cd.id,
  'Created',
  CASE
    WHEN cd.data IS NOT NULL AND jsonb_typeof(cd.data) = 'object' THEN cd.data
    ELSE jsonb_strip_nulls(jsonb_build_object(
      'id', cd.id,
      'idShort', cd.id_short,
      'modelType', 'ConceptDescription'
    ))
  END,
  FALSE,
  COALESCE(cd.db_created_at, NOW()),
  cd.data -> 'administration' ->> 'createdAt',
  cd.data -> 'administration' ->> 'updatedAt'
FROM concept_description cd
WHERE NOT EXISTS (
  SELECT 1 FROM concept_description_history cdh WHERE cdh.identifier = cd.id
);

INSERT INTO descriptor_history (
  identifier,
  change_type,
  snapshot,
  deleted,
  valid_from,
  administration_created_at_text,
  administration_updated_at_text
)
SELECT
  ad.id,
  'Created',
  jsonb_strip_nulls(jsonb_build_object(
    'id', ad.id,
    'idShort', ad.id_short,
    'assetKind', CASE ad.asset_kind
      WHEN 0 THEN 'Type'
      WHEN 1 THEN 'Instance'
      WHEN 2 THEN 'Batch'
      WHEN 3 THEN 'Role'
      ELSE NULL
    END,
    'assetType', ad.asset_type,
    'globalAssetId', ad.global_asset_id,
    'endpoints', '[]'::jsonb,
    'administration', dp.administrative_information_payload
  )),
  FALSE,
  COALESCE(ad.db_created_at, NOW()),
  dp.administrative_information_payload ->> 'createdAt',
  dp.administrative_information_payload ->> 'updatedAt'
FROM aas_descriptor ad
LEFT JOIN descriptor_payload dp ON dp.descriptor_id = ad.descriptor_id
WHERE NOT EXISTS (
  SELECT 1 FROM descriptor_history dh WHERE dh.identifier = ad.id
);

-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.0',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
