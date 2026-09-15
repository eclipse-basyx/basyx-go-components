-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.3
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Indexes existing AAS and Submodel history snapshots and diffs for exact
--   identifier searches when resolving historical DPP ownership. jsonb_ops
--   supports the recursive JSONPath accessor used to find candidate versions.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE INDEX IF NOT EXISTS ix_aas_history_payload_snapshot_identifiers
  ON aas_history_payload USING GIN (snapshot jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_aas_history_payload_diff_identifiers
  ON aas_history_payload USING GIN (diff jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_submodel_history_payload_snapshot_identifiers
  ON submodel_history_payload USING GIN (snapshot jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_submodel_history_payload_diff_identifiers
  ON submodel_history_payload USING GIN (diff jsonb_ops);

UPDATE basyxsystem
SET schema_version = 'v1.2.3',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
