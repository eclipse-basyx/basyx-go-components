-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Integration Test Fixture
-- ----------------------------------------------------------------------------
-- Description:
--   Simulates an explicit TOAST vacuum threshold on an affected table.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element_payload SET (toast.autovacuum_vacuum_threshold = 5000);
