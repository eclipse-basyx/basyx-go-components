-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Integration Test Fixture
-- ----------------------------------------------------------------------------
-- Description:
--   Simulates an explicit table index-cleanup policy.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element SET (vacuum_index_cleanup = off);
