-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds relationship authorization state and shared immutable audit delivery.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE rebac_scope (
  scope TEXT PRIMARY KEY,
  configuration_hash TEXT NOT NULL,
  integration_state TEXT NOT NULL DEFAULT '',
  recovery_pending BOOLEAN NOT NULL DEFAULT FALSE,
  changes_cursor TEXT NOT NULL DEFAULT '',
  desired_revision BIGINT NOT NULL DEFAULT 0,
  applied_revision BIGINT NOT NULL DEFAULT 0,
  CHECK (applied_revision >= 0 AND desired_revision >= applied_revision)
);

CREATE TABLE rebac_resource (
  resource_uuid UUID PRIMARY KEY,
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  kind TEXT NOT NULL,
  identifier TEXT NOT NULL,
  object_key TEXT NOT NULL,
  parent_uuid UUID REFERENCES rebac_resource(resource_uuid),
  generation BIGINT NOT NULL DEFAULT 1,
  deleted_at TIMESTAMPTZ,
  UNIQUE (scope, resource_uuid)
);
CREATE UNIQUE INDEX ix_rebac_resource_live ON rebac_resource(scope, kind, identifier)
WHERE deleted_at IS NULL;
CREATE INDEX ix_rebac_resource_parent ON rebac_resource(scope, parent_uuid)
WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX ix_rebac_resource_object_key ON rebac_resource(scope, kind, object_key)
WHERE deleted_at IS NULL;

CREATE TABLE rebac_relationship (
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  subject TEXT NOT NULL,
  relation TEXT NOT NULL,
  object TEXT NOT NULL,
  provenance TEXT NOT NULL DEFAULT 'direct',
  PRIMARY KEY (scope, subject, relation, object)
);
CREATE INDEX ix_rebac_relationship_object ON rebac_relationship(scope, object, relation);
CREATE INDEX ix_rebac_relationship_subject ON rebac_relationship(scope, subject);

CREATE TABLE rebac_generated (
 scope TEXT NOT NULL REFERENCES rebac_scope(scope),
 target_uuid UUID NOT NULL REFERENCES rebac_resource(resource_uuid),
 source_uuid UUID NOT NULL REFERENCES rebac_resource(resource_uuid),
 integration TEXT NOT NULL,
 PRIMARY KEY(scope, target_uuid, source_uuid, integration)
);
CREATE INDEX ix_rebac_generated_source ON rebac_generated(scope, source_uuid);

CREATE TABLE rebac_principal (
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  subject TEXT NOT NULL,
  kind TEXT NOT NULL,
  issuer TEXT NOT NULL,
  identifier TEXT NOT NULL,
  PRIMARY KEY (scope, subject)
);

CREATE TABLE rebac_outbox (
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  revision BIGINT NOT NULL,
  transaction_id BIGINT NOT NULL DEFAULT txid_current(),
  writes JSONB NOT NULL,
  deletes JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  PRIMARY KEY (scope, revision)
);

CREATE TABLE audit_stream (
  stream TEXT PRIMARY KEY,
  last_sequence BIGINT NOT NULL DEFAULT 0,
  last_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE audit_record (
  event_id UUID PRIMARY KEY,
  stream TEXT NOT NULL REFERENCES audit_stream(stream),
  sequence BIGINT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  actor TEXT NOT NULL,
  resource TEXT NOT NULL,
  outcome TEXT NOT NULL,
  correlation_id TEXT NOT NULL,
  event JSONB NOT NULL,
  previous_hash TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  UNIQUE (stream, sequence)
);
CREATE INDEX ix_audit_record_time ON audit_record(stream, occurred_at, sequence);
CREATE INDEX ix_audit_record_actor ON audit_record(stream, actor, sequence);
CREATE INDEX ix_audit_record_resource ON audit_record(stream, resource, sequence);
CREATE INDEX ix_audit_record_correlation ON audit_record(stream, correlation_id);
CREATE TABLE audit_delivery (
  event_id UUID PRIMARY KEY REFERENCES audit_record(event_id),
  attempts INTEGER NOT NULL DEFAULT 0,
  receipt JSONB,
  archived_at TIMESTAMPTZ
);
CREATE INDEX ix_audit_delivery_pending ON audit_delivery(event_id) WHERE archived_at IS NULL;

CREATE TABLE aasx_package_manifest (
  package_db_id BIGINT PRIMARY KEY REFERENCES aasx_package(id) ON DELETE CASCADE,
  content_sha256 CHAR(64) NOT NULL,
  manifest_version SMALLINT NOT NULL DEFAULT 1,
  db_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  db_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE aasx_package_manifest_resource (
  package_db_id BIGINT NOT NULL REFERENCES aasx_package(id) ON DELETE CASCADE,
  resource_uuid UUID NOT NULL REFERENCES rebac_resource(resource_uuid),
  resource_kind TEXT NOT NULL,
  resource_identifier TEXT NOT NULL,
  position INTEGER NOT NULL,
  PRIMARY KEY (package_db_id, position),
  UNIQUE (package_db_id, resource_uuid)
);

UPDATE basyxsystem
SET schema_version = 'v1.2.2', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
