-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.13
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Makes SubmodelElement sibling uniqueness constraints transaction-deferrable
--   for set-wise Submodel replacement reconciliation.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element
  DROP CONSTRAINT IF EXISTS uq_sibling_idshort;

ALTER TABLE submodel_element
  ADD CONSTRAINT uq_sibling_idshort
  UNIQUE (submodel_id, parent_sme_id, idshort_path)
  DEFERRABLE INITIALLY IMMEDIATE;

ALTER TABLE submodel_element
  DROP CONSTRAINT IF EXISTS uq_sibling_pos;

ALTER TABLE submodel_element
  ADD CONSTRAINT uq_sibling_pos
  UNIQUE (submodel_id, parent_sme_id, position)
  DEFERRABLE INITIALLY IMMEDIATE;

UPDATE basyxsystem
SET schema_version = 'v1.1.13',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
