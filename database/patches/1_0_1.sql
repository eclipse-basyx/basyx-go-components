-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.0.1
-- Metamodel Ver. : 3.1
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for Eclipse BaSyx components and schema updates.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- Schema compatibility upgrades
-- Author: Stemmer
-- ------------------------------------------

ALTER TABLE IF EXISTS aas_descriptor
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE IF EXISTS aas_identifier
  ADD COLUMN IF NOT EXISTS db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE IF EXISTS aas_identifier
  ADD COLUMN IF NOT EXISTS db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

UPDATE basyxsystem
SET schema_version = 'v1.0.1',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
