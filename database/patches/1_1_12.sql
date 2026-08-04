-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.12
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds indexes for supplemental semantic reference payload lookups.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE INDEX IF NOT EXISTS ix_specasset_supp_sem_refpayload_refid
  ON specific_asset_id_supplemental_semantic_id_reference_payload(reference_id);

CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_refpayload_refid
  ON submodel_descriptor_supplemental_semantic_id_reference_payload(reference_id);

UPDATE basyxsystem
SET schema_version = 'v1.1.12',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
