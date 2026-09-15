-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Indexes exact property text lookups used to resolve DPP metadata IDs.
--   A hash index supports arbitrary property text lengths without B-tree
--   index-entry size restrictions.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE INDEX IF NOT EXISTS ix_property_element_value_text_hash
  ON property_element USING HASH (value_text)
  WHERE value_text IS NOT NULL;

UPDATE basyxsystem
SET schema_version = 'v1.2.2',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
