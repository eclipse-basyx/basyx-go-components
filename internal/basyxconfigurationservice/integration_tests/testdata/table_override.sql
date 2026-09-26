-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Integration Test Fixture
-- ----------------------------------------------------------------------------
-- Description:
--   Simulates a pre-existing operator override on an affected table.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element SET (autovacuum_vacuum_threshold = 5000);
ALTER TABLE submodel_element_payload SET (autovacuum_enabled = false);
