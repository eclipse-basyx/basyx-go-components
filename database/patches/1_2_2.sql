-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds persistent authorization identities and the desired-state, outbox and
--   invitation tables of the experimental relationship-based access control.
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

CREATE UNIQUE INDEX IF NOT EXISTS ux_aas_auth_uuid ON aas (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_submodel_auth_uuid ON submodel (auth_uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_concept_description_auth_uuid ON concept_description (auth_uuid);

-- Desired authorization state. OpenFGA is a projection of these rows.
CREATE TABLE IF NOT EXISTS rebac_grant (
  id BIGSERIAL PRIMARY KEY,
  object_key TEXT NOT NULL,
  object_type TEXT NOT NULL CHECK (object_type IN ('aas', 'submodel', 'concept_description', 'element', 'repository')),
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

CREATE INDEX IF NOT EXISTS ix_rebac_grant_object_uuid ON rebac_grant (object_uuid);
CREATE INDEX IF NOT EXISTS ix_rebac_grant_subject ON rebac_grant (subject_key, object_type);

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

CREATE TABLE IF NOT EXISTS rebac_invitation (
  id UUID PRIMARY KEY,
  token_hash BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
  object_key TEXT NOT NULL,
  object_type TEXT NOT NULL CHECK (object_type IN ('aas', 'submodel', 'concept_description', 'element')),
  object_uuid UUID NOT NULL,
  element_path TEXT,
  relation TEXT NOT NULL CHECK (relation IN ('viewer', 'editor', 'executor')),
  expires_at TIMESTAMPTZ NOT NULL,
  max_uses INTEGER NOT NULL CHECK (max_uses > 0),
  used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0 AND used_count <= max_uses),
  created_by TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_rebac_invitation_object ON rebac_invitation (object_key, created_at);
CREATE INDEX IF NOT EXISTS ix_rebac_invitation_object_uuid ON rebac_invitation (object_uuid);

-- Ordered projection queue towards OpenFGA. Operations of one object are
-- serialized by rebac_object_revision row locks, so their seq order equals
-- their commit order.
CREATE TABLE IF NOT EXISTS rebac_outbox (
  seq BIGSERIAL PRIMARY KEY,
  scope TEXT NOT NULL,
  operation_id UUID NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('write', 'delete')),
  tuple_object TEXT NOT NULL,
  tuple_relation TEXT NOT NULL,
  tuple_user TEXT NOT NULL,
  revokes BOOLEAN NOT NULL DEFAULT FALSE,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  applied_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_rebac_outbox_pending
ON rebac_outbox (scope, seq) WHERE applied_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_rebac_outbox_pending_object
ON rebac_outbox (scope, tuple_object) WHERE applied_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_rebac_outbox_pending_revocation
ON rebac_outbox (scope) WHERE applied_at IS NULL AND revokes;
CREATE INDEX IF NOT EXISTS ix_rebac_outbox_operation
ON rebac_outbox (operation_id);
CREATE INDEX IF NOT EXISTS ix_rebac_outbox_applied
ON rebac_outbox (applied_at) WHERE applied_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS rebac_scope_state (
  scope TEXT PRIMARY KEY,
  last_enabled_at TIMESTAMPTZ,
  reconciled_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

-- One database is bound to exactly one scope, store and model.
CREATE TABLE IF NOT EXISTS rebac_model_activation (
  scope TEXT PRIMARY KEY,
  store_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  model_hash TEXT NOT NULL,
  activated_by TEXT NOT NULL,
  activated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_rebac_model_activation_singleton
ON rebac_model_activation ((TRUE));

UPDATE basyxsystem
SET schema_version = 'v1.2.2',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
