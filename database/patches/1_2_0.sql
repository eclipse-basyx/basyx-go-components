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
-- Patch Version  : 1.2.0
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds feed_events table for the CloudEvents Event Feed REST API.
--
--   seq (BIGSERIAL) is an internal write-order id, assigned before commit -
--   it is never used for client-facing ordering or cursors, because two
--   concurrent writer transactions can commit in the opposite order to the
--   one in which they were allocated a seq value.
--
--   publish_seq is the client-facing cursor/order key. It starts NULL and is
--   assigned by a periodic background job (Service.RunPublishAssignment)
--   that only ever selects already-committed (i.e. visible) rows. Because
--   assignment can only happen after a row is visible, a client resuming
--   from any already-issued publish_seq can never have an earlier-committed
--   row appear after it - unlike seq, publish_seq reflects true commit
--   (visibility) order.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS feed_events (
    seq                 BIGSERIAL    NOT NULL,
    publish_seq         BIGINT,
    id                  VARCHAR(64)  PRIMARY KEY,
    event_type          TEXT         NOT NULL,
    subject             TEXT         NOT NULL,
    source              TEXT         NOT NULL,
    time                TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp(),
    dataschema_full     TEXT         NOT NULL,
    dataschema_compact  TEXT         NOT NULL,
    data_full           JSONB        NOT NULL,
    data_compact        JSONB        NOT NULL,
    authorization_aas_ids JSONB,
    CONSTRAINT ux_feed_events_seq UNIQUE (seq)
);

CREATE SEQUENCE IF NOT EXISTS feed_events_publish_seq_seq;

CREATE INDEX IF NOT EXISTS ix_feed_events_seq
    ON feed_events (seq ASC);

-- Lets the publish-assignment background job find newly committed,
-- not-yet-assigned rows without scanning the whole table.
CREATE INDEX IF NOT EXISTS ix_feed_events_unpublished_seq
    ON feed_events (seq ASC) WHERE publish_seq IS NULL;

-- Client-facing ordering index and uniqueness guard; only assigned rows are
-- ever queried by clients.
CREATE UNIQUE INDEX IF NOT EXISTS ix_feed_events_publish_seq
    ON feed_events (publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_event_type_publish_seq
    ON feed_events (event_type ASC, publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_subject_publish_seq
    ON feed_events (subject ASC, publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_source_publish_seq
    ON feed_events (source ASC, publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_dataschema_full_publish_seq
    ON feed_events (dataschema_full ASC, publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_dataschema_compact_publish_seq
    ON feed_events (dataschema_compact ASC, publish_seq ASC) WHERE publish_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_feed_events_time_seq
    ON feed_events (time ASC, seq ASC);

UPDATE basyxsystem
SET schema_version = 'v1.2.0',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
