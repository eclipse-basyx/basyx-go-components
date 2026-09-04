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
--   Cursor/order key is BIGSERIAL seq assigned in the writer transaction.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE TABLE IF NOT EXISTS feed_events (
    seq                 BIGSERIAL    NOT NULL,
    id                  VARCHAR(64)  PRIMARY KEY,
    event_type          TEXT         NOT NULL,
    subject             TEXT         NOT NULL,
    source              TEXT         NOT NULL,
    time                TIMESTAMPTZ  NOT NULL DEFAULT clock_timestamp(),
    dataschema_full     TEXT         NOT NULL,
    dataschema_compact  TEXT         NOT NULL,
    data_full           JSONB        NOT NULL,
    data_compact        JSONB        NOT NULL,
    CONSTRAINT ux_feed_events_seq UNIQUE (seq)
);

CREATE INDEX IF NOT EXISTS ix_feed_events_seq
    ON feed_events (seq ASC);

CREATE INDEX IF NOT EXISTS ix_feed_events_event_type_seq
    ON feed_events (event_type ASC, seq ASC);

CREATE INDEX IF NOT EXISTS ix_feed_events_subject_seq
    ON feed_events (subject ASC, seq ASC);

CREATE INDEX IF NOT EXISTS ix_feed_events_source_seq
    ON feed_events (source ASC, seq ASC);

CREATE INDEX IF NOT EXISTS ix_feed_events_dataschema_full_seq
    ON feed_events (dataschema_full ASC, seq ASC);

CREATE INDEX IF NOT EXISTS ix_feed_events_dataschema_compact_seq
    ON feed_events (dataschema_compact ASC, seq ASC);

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
