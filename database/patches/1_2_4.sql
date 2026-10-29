-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.4
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Persists immutable AOL plans, single-use approvals and revision receipts
--   for confirmed MCP AAS creation and conditional rollback. Receipts commit
--   with their resource mutation and remain available without history enabled.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS aol_plan (
  id UUID PRIMARY KEY,
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  status TEXT NOT NULL,
  version BIGINT NOT NULL CHECK (version > 0),
  updated_at TIMESTAMPTZ NOT NULL,
  document JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS aol_plan_owner ON aol_plan (issuer, subject, updated_at);

CREATE TABLE IF NOT EXISTS aol_receipt (
  sequence BIGSERIAL PRIMARY KEY,
  plan_id UUID NOT NULL REFERENCES aol_plan (id),
  kind TEXT NOT NULL CHECK (kind IN ('shells', 'submodels')),
  identifier TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  audit JSONB NOT NULL,
  UNIQUE (plan_id, kind, identifier)
);

CREATE TABLE IF NOT EXISTS aol_event (
  sequence BIGSERIAL PRIMARY KEY,
  plan_id UUID NOT NULL REFERENCES aol_plan (id),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  audit JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS aol_event_plan ON aol_event (plan_id, sequence);

UPDATE basyxsystem
SET schema_version = 'v1.2.4',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
