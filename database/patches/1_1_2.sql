-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for new history snapshot checkpoint indexes.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- Add indexes for history snapshot checkpoints
-- ------------------------------------------

CREATE INDEX IF NOT EXISTS ix_aas_history_snapshot_checkpoint
  ON aas_history(identifier, history_id DESC)
  WHERE payload_type = 'snapshot';

CREATE INDEX IF NOT EXISTS ix_submodel_history_snapshot_checkpoint
  ON submodel_history(identifier, history_id DESC)
  WHERE payload_type = 'snapshot';

CREATE INDEX IF NOT EXISTS ix_cd_history_snapshot_checkpoint
  ON concept_description_history(identifier, history_id DESC)
  WHERE payload_type = 'snapshot';

CREATE INDEX IF NOT EXISTS ix_descriptor_history_snapshot_checkpoint
  ON descriptor_history(identifier, history_id DESC)
  WHERE payload_type = 'snapshot';

-- Mark the schema as upgraded only after all schema objects completed
-- successfully.
UPDATE basyxsystem
SET schema_version = 'v1.1.2',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
