-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds persistent authorization identities and the relationship, link,
--   derivation and invitation tables of the experimental relationship-based
--   access control.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- Persistent authorization identities. The volatile default rewrites existing
-- rows once and assigns a distinct UUID to each of them.
ALTER TABLE IF EXISTS aas
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE IF EXISTS submodel
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE IF EXISTS concept_description
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE IF EXISTS descriptor
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE IF EXISTS aas_identifier
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE IF EXISTS aasx_package
  ADD COLUMN IF NOT EXISTS auth_uuid UUID NOT NULL DEFAULT gen_random_uuid();

CREATE UNIQUE INDEX IF NOT EXISTS ux_aas_auth_uuid ON aas (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_submodel_auth_uuid ON submodel (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_concept_description_auth_uuid ON concept_description (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_descriptor_auth_uuid ON descriptor (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_aas_identifier_auth_uuid ON aas_identifier (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_aasx_package_auth_uuid ON aasx_package (auth_uuid);

-- Direct relationships. Permissions are evaluated from these rows in SQL.
CREATE TABLE IF NOT EXISTS rebac_grant (
  id BIGSERIAL PRIMARY KEY,
  object_key TEXT NOT NULL,
  object_type TEXT NOT NULL CHECK (object_type IN (
    'aas', 'submodel', 'concept_description', 'element',
    'aas_descriptor', 'submodel_descriptor', 'asset_links', 'aasx_package', 'repository'
  )),
  object_uuid UUID,
  element_path TEXT,
  relation TEXT NOT NULL CHECK (relation IN ('owner', 'editor', 'viewer', 'executor', 'creator', 'admin')),
  subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'group')),
  subject_key TEXT NOT NULL,
  subject_issuer TEXT NOT NULL,
  subject_name TEXT NOT NULL,
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (object_key, relation, subject_key),
  CHECK ((object_type = 'element') = (element_path IS NOT NULL)),
  CHECK ((object_type = 'repository') = (object_uuid IS NULL))
);

CREATE INDEX IF NOT EXISTS ix_rebac_grant_object_uuid ON rebac_grant (object_uuid, object_type);
CREATE INDEX IF NOT EXISTS ix_rebac_grant_permission
ON rebac_grant (subject_key, object_type, relation) INCLUDE (object_uuid, element_path);

CREATE TABLE IF NOT EXISTS rebac_object_revision (
  object_key TEXT PRIMARY KEY,
  revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS rebac_submodel_link (
  submodel_uuid UUID NOT NULL,
  aas_uuid UUID NOT NULL,
  approved_by TEXT NOT NULL,
  approved_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (submodel_uuid, aas_uuid)
);

CREATE INDEX IF NOT EXISTS ix_rebac_submodel_link_aas ON rebac_submodel_link (aas_uuid);

-- Registry descriptors synchronized from repository resources and discovery
-- entries generated from descriptors inherit the access of their source.
CREATE TABLE IF NOT EXISTS rebac_derivation (
  object_uuid UUID PRIMARY KEY,
  object_type TEXT NOT NULL CHECK (object_type IN ('aas_descriptor', 'submodel_descriptor', 'asset_links')),
  source_uuid UUID NOT NULL,
  source_type TEXT NOT NULL CHECK (source_type IN ('aas', 'submodel', 'aas_descriptor')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS ix_rebac_derivation_source ON rebac_derivation (source_uuid, object_type);

CREATE TABLE IF NOT EXISTS rebac_invitation (
  id UUID PRIMARY KEY,
  token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  object_key TEXT NOT NULL,
  object_type TEXT NOT NULL CHECK (object_type IN (
    'aas', 'submodel', 'concept_description', 'element',
    'aas_descriptor', 'submodel_descriptor', 'asset_links', 'aasx_package'
  )),
  object_uuid UUID NOT NULL,
  element_path TEXT,
  relation TEXT NOT NULL CHECK (relation IN ('viewer', 'editor', 'executor')),
  expires_at TIMESTAMPTZ NOT NULL,
  max_uses INTEGER NOT NULL CHECK (max_uses > 0),
  used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0 AND used_count <= max_uses),
  expected_subject_key TEXT,
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_rebac_invitation_object ON rebac_invitation (object_key, created_at);
CREATE INDEX IF NOT EXISTS ix_rebac_invitation_object_uuid ON rebac_invitation (object_uuid);

UPDATE basyxsystem
SET schema_version = 'v1.2.2',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
