-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.3
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds server-managed resource revisions for HTTP conditional requests
--   (ETag, If-Match, If-None-Match). Revisions are keyed by resource kind and
--   API identifier without foreign keys, so deletes and replacements can
--   validate the previous revision before the resource disappears. All
--   revisions come from one sequence and never repeat.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE SEQUENCE IF NOT EXISTS basyx_resource_revision_seq AS BIGINT START WITH 1 MINVALUE 1;

CREATE TABLE IF NOT EXISTS resource_revision (
  kind TEXT NOT NULL,
  identifier TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision >= 0),
  PRIMARY KEY (kind, identifier)
);

UPDATE basyxsystem
SET schema_version = 'v1.2.3',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
