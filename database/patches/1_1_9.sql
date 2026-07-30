-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.9
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds the index required for efficient AAS-to-Submodel reference lookups.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE INDEX IF NOT EXISTS ix_aas_submodel_reference_aas_id
  ON aas_submodel_reference(aas_id);

UPDATE basyxsystem
SET schema_version = 'v1.1.9',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
