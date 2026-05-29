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
-- nullable scalar columns are added to the owning parent tables for efficient
-- filtering and version-date lookups while preserving the original JSON.
-- SubmodelElement is not Identifiable and has no AdministrativeInformation;
-- remove the obsolete payload column from older schema versions.
ALTER TABLE IF EXISTS submodel_element_payload
  DROP COLUMN IF EXISTS administrative_information_payload;

ALTER TABLE IF EXISTS aas
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS submodel
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS aas_descriptor
  ADD COLUMN IF NOT EXISTS administration_created_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS administration_updated_at TIMESTAMPTZ;

ALTER TABLE IF EXISTS submodel_descriptor
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

-- Keep parent-table scalar columns synced from payload JSON.
CREATE OR REPLACE FUNCTION sync_aas_administration_timestamps()
RETURNS TRIGGER AS $$
BEGIN
  UPDATE aas
  SET administration_created_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'createdAt'),
      administration_updated_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'updatedAt')
  WHERE id = NEW.aas_id;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_submodel_administration_timestamps()
RETURNS TRIGGER AS $$
BEGIN
  UPDATE submodel
  SET administration_created_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'createdAt'),
      administration_updated_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'updatedAt')
  WHERE id = NEW.submodel_id;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_descriptor_administration_timestamps()
RETURNS TRIGGER AS $$
BEGIN
  UPDATE aas_descriptor
  SET administration_created_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'createdAt'),
      administration_updated_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'updatedAt')
  WHERE descriptor_id = NEW.descriptor_id;

  UPDATE submodel_descriptor
  SET administration_created_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'createdAt'),
      administration_updated_at = basyx_jsonb_timestamptz(NEW.administrative_information_payload, 'updatedAt')
  WHERE descriptor_id = NEW.descriptor_id;

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
  AFTER INSERT OR UPDATE OF administrative_information_payload
  ON aas_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_aas_administration_timestamps();

DROP TRIGGER IF EXISTS submodel_payload_sync_administration_timestamps ON submodel_payload;
CREATE TRIGGER submodel_payload_sync_administration_timestamps
  AFTER INSERT OR UPDATE OF administrative_information_payload
  ON submodel_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_submodel_administration_timestamps();

DROP TRIGGER IF EXISTS descriptor_payload_sync_administration_timestamps ON descriptor_payload;
CREATE TRIGGER descriptor_payload_sync_administration_timestamps
  AFTER INSERT OR UPDATE OF administrative_information_payload
  ON descriptor_payload
  FOR EACH ROW
  EXECUTE FUNCTION sync_descriptor_administration_timestamps();

DROP TRIGGER IF EXISTS concept_description_sync_administration_timestamps ON concept_description;
CREATE TRIGGER concept_description_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF data
  ON concept_description
  FOR EACH ROW
  EXECUTE FUNCTION sync_concept_description_administration_timestamps();

-- Index the extracted timestamps for V3.2 recent-change and version-date
-- access patterns without querying deep JSONB expressions.
CREATE INDEX IF NOT EXISTS ix_aas_admin_created_at ON aas(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_aas_admin_updated_at ON aas(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_submodel_admin_created_at ON submodel(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_submodel_admin_updated_at ON submodel(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_aas_descriptor_admin_created_at ON aas_descriptor(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_aas_descriptor_admin_updated_at ON aas_descriptor(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_admin_created_at ON submodel_descriptor(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_admin_updated_at ON submodel_descriptor(administration_updated_at);
CREATE INDEX IF NOT EXISTS ix_cd_admin_created_at ON concept_description(administration_created_at);
CREATE INDEX IF NOT EXISTS ix_cd_admin_updated_at ON concept_description(administration_updated_at);

-- V3.2 inserts "Batch" at enum index 2 for asset kind. Existing persisted numeric enum values
-- from V3.1.1 with index >= 2 must be shifted by +1 to preserve semantic value.
UPDATE asset_information
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;

UPDATE aas_descriptor
SET asset_kind = asset_kind + 1
WHERE asset_kind >= 2;

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
