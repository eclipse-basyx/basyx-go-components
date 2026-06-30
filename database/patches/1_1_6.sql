-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.6
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for future descriptor AdministrativeInformation
--   timestamp synchronization when descriptor payloads are inserted before
--   parent rows.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ----------------------------------------------------------
-- Descriptor AdministrativeInformation timestamp sync repair
-- ----------------------------------------------------------

CREATE OR REPLACE FUNCTION sync_aas_descriptor_administration_timestamps_from_payload()
RETURNS TRIGGER AS $$
BEGIN
  SELECT basyx_jsonb_timestamptz(dp.administrative_information_payload, 'createdAt'),
         basyx_jsonb_timestamptz(dp.administrative_information_payload, 'updatedAt')
    INTO NEW.administration_created_at, NEW.administration_updated_at
    FROM descriptor_payload dp
   WHERE dp.descriptor_id = NEW.descriptor_id;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_submodel_descriptor_administration_timestamps_from_payload()
RETURNS TRIGGER AS $$
BEGIN
  SELECT basyx_jsonb_timestamptz(dp.administrative_information_payload, 'createdAt'),
         basyx_jsonb_timestamptz(dp.administrative_information_payload, 'updatedAt')
    INTO NEW.administration_created_at, NEW.administration_updated_at
    FROM descriptor_payload dp
   WHERE dp.descriptor_id = NEW.descriptor_id;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS aas_descriptor_sync_administration_timestamps ON aas_descriptor;
CREATE TRIGGER aas_descriptor_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF descriptor_id
  ON aas_descriptor
  FOR EACH ROW
  EXECUTE FUNCTION sync_aas_descriptor_administration_timestamps_from_payload();

DROP TRIGGER IF EXISTS submodel_descriptor_sync_administration_timestamps ON submodel_descriptor;
CREATE TRIGGER submodel_descriptor_sync_administration_timestamps
  BEFORE INSERT OR UPDATE OF descriptor_id
  ON submodel_descriptor
  FOR EACH ROW
  EXECUTE FUNCTION sync_submodel_descriptor_administration_timestamps_from_payload();

-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.6',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
