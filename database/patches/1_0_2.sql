-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.0.2
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

CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_refpayload_refid
  ON submodel_descriptor_semantic_id_reference_payload(reference_id);
