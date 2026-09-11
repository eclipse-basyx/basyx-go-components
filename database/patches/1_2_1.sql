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

-- Durable event delivery; pending rows are independent of feed retention.

CREATE TABLE IF NOT EXISTS event_outbox (
    seq BIGSERIAL PRIMARY KEY,
    sink_id TEXT NOT NULL,
    event_id VARCHAR(64) NOT NULL,
    ordering_key TEXT NOT NULL,
    envelope TEXT NOT NULL,
    routing JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    worker_id TEXT,
    last_error_code TEXT,
    UNIQUE (sink_id, event_id)
);
CREATE INDEX IF NOT EXISTS ix_event_outbox_ordering ON event_outbox (sink_id, ordering_key, seq);
CREATE INDEX IF NOT EXISTS ix_event_outbox_ready ON event_outbox (sink_id, next_attempt_at, seq);
CREATE INDEX IF NOT EXISTS ix_event_outbox_age ON event_outbox (sink_id, created_at);

UPDATE basyxsystem SET schema_version = 'v1.2.1', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
