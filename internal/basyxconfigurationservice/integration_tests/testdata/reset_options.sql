-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Integration Test Fixture
-- ----------------------------------------------------------------------------
-- Description:
--   Resets table options before exercising an upgrade with operator settings.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

ALTER TABLE submodel_element RESET (autovacuum_vacuum_scale_factor, vacuum_index_cleanup);
ALTER TABLE submodel_element RESET (autovacuum_vacuum_threshold);
ALTER TABLE submodel_element_payload RESET (autovacuum_vacuum_scale_factor, vacuum_index_cleanup);
ALTER TABLE submodel_element_payload RESET (autovacuum_enabled);
ALTER TABLE submodel_element_payload RESET (toast.autovacuum_vacuum_threshold);
ALTER TABLE submodel_element_semantic_id_reference_payload RESET (autovacuum_vacuum_scale_factor, vacuum_index_cleanup);
