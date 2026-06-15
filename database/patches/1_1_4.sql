/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.1.4
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Database patch script for PostgreSQL-backed ABAC policy versions, rules,
--   policy events, and ABAC policy WORM evidence artifact receipts.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- ------------------------------------------
-- ABAC policy versions, rules, and events
-- ------------------------------------------

CREATE TABLE IF NOT EXISTS abac_policy_versions (
    version_id BIGSERIAL PRIMARY KEY,
    service_scope VARCHAR(255) NOT NULL,
    policy_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status IN ('staged', 'active', 'superseded', 'rejected')),
    source_type VARCHAR(64) NOT NULL,
    source_ref TEXT,
    configured_policy_json JSONB NOT NULL,
    configured_policy_hash VARCHAR(64) NOT NULL,
    raw_policy_hash VARCHAR(64),
    materialized_policy_json JSONB NOT NULL,
    materialized_policy_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_subject TEXT,
    created_by_issuer TEXT,
    created_by_client_id TEXT,
    updated_at TIMESTAMPTZ,
    updated_by_subject TEXT,
    updated_by_issuer TEXT,
    updated_by_client_id TEXT,
    activated_at TIMESTAMPTZ,
    activated_by_subject TEXT,
    activated_by_issuer TEXT,
    activated_by_client_id TEXT,
    superseded_at TIMESTAMPTZ,
    artifact_ref JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS abac_policy_versions_one_active_per_scope
    ON abac_policy_versions (service_scope)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS abac_policy_versions_scope_status_idx
    ON abac_policy_versions (service_scope, status, created_at DESC);

CREATE INDEX IF NOT EXISTS abac_policy_versions_policy_id_idx
    ON abac_policy_versions (service_scope, policy_id);

CREATE INDEX IF NOT EXISTS abac_policy_versions_raw_hash_idx
    ON abac_policy_versions (service_scope, raw_policy_hash)
    WHERE raw_policy_hash IS NOT NULL;

CREATE TABLE IF NOT EXISTS abac_policy_rules (
    rule_id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES abac_policy_versions(version_id) ON DELETE CASCADE,
    policy_id VARCHAR(64) NOT NULL,
    service_scope VARCHAR(255) NOT NULL,
    rule_index INTEGER NOT NULL CHECK (rule_index > 0),
    matched_rule_id TEXT NOT NULL,
    configured_rule_json JSONB NOT NULL,
    materialized_rule_json JSONB NOT NULL,
    acl_json JSONB,
    attributes_json JSONB,
    objects_json JSONB,
    formula_json JSONB,
    filters_json JSONB,
    access VARCHAR(64) NOT NULL,
    rights TEXT[] NOT NULL DEFAULT '{}',
    rule_hash VARCHAR(64) NOT NULL,
    materialized_rule_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_subject TEXT,
    created_by_issuer TEXT,
    created_by_client_id TEXT,
    UNIQUE (version_id, rule_index),
    UNIQUE (version_id, matched_rule_id)
);

CREATE INDEX IF NOT EXISTS abac_policy_rules_lookup_idx
    ON abac_policy_rules (service_scope, policy_id, matched_rule_id);

CREATE INDEX IF NOT EXISTS abac_policy_rules_policy_order_idx
    ON abac_policy_rules (version_id, rule_index);

CREATE TABLE IF NOT EXISTS abac_policy_events (
    event_id BIGSERIAL PRIMARY KEY,
    version_id BIGINT REFERENCES abac_policy_versions(version_id) ON DELETE CASCADE,
    service_scope VARCHAR(255) NOT NULL,
    policy_id VARCHAR(64),
    operation VARCHAR(128) NOT NULL,
    endpoint TEXT,
    actor_subject TEXT,
    actor_issuer TEXT,
    actor_client_id TEXT,
    request_id TEXT,
    correlation_id TEXT,
    source_type VARCHAR(64),
    source_ref TEXT,
    before_policy_hash VARCHAR(64),
    after_policy_hash VARCHAR(64),
    before_materialized_policy_hash VARCHAR(64),
    after_materialized_policy_hash VARCHAR(64),
    details_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS abac_policy_events_version_idx
    ON abac_policy_events (version_id, created_at DESC);

CREATE INDEX IF NOT EXISTS abac_policy_events_scope_idx
    ON abac_policy_events (service_scope, created_at DESC);

DO $$
BEGIN
    ALTER TABLE history_evidence_artifacts
        DROP CONSTRAINT IF EXISTS history_evidence_artifacts_artifact_type_check;
    ALTER TABLE history_evidence_artifacts
        ADD CONSTRAINT history_evidence_artifacts_artifact_type_check
        CHECK (artifact_type IN ('manifest', 'snapshot', 'history_event', 'abac_policy_version'));
END $$;

UPDATE basyxsystem
SET schema_version = 'v1.1.4',
    state = 'clean'
WHERE identifier = (
    SELECT identifier
    FROM basyxsystem
    ORDER BY identifier ASC
    LIMIT 1
);
