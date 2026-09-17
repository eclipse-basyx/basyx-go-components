-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Integration Test Fixture
-- ----------------------------------------------------------------------------
-- Description:
--   Simulates a pre-existing table vacuum scale factor override.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element SET (autovacuum_vacuum_scale_factor = 0.07);
