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

CREATE TABLE IF NOT EXISTS rebac_share_invitation (
  id UUID PRIMARY KEY,
  scope TEXT NOT NULL REFERENCES rebac_scope(scope) ON DELETE CASCADE,
  access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
  issued_access_revision BIGINT NOT NULL,
  token_hash BYTEA NOT NULL UNIQUE,
  rights JSONB NOT NULL,
  expected_issuer TEXT,
  expected_subject TEXT,
  created_by_issuer TEXT NOT NULL,
  created_by_subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMPTZ NOT NULL,
  redeemed_at TIMESTAMPTZ,
  redeemed_by_issuer TEXT,
  redeemed_by_subject TEXT,
  revoked_at TIMESTAMPTZ,
  revoked_by_issuer TEXT,
  revoked_by_subject TEXT,
  CHECK ((expected_issuer IS NULL) = (expected_subject IS NULL)),
  CHECK ((revoked_by_issuer IS NULL) = (revoked_by_subject IS NULL)),
  CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS ix_rebac_share_invitation_access
  ON rebac_share_invitation(scope, access_id, expires_at)
  WHERE redeemed_at IS NULL AND revoked_at IS NULL;

UPDATE basyxsystem SET schema_version = 'v1.1.22', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
