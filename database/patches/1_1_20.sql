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

ALTER TABLE rebac_principal
  ADD COLUMN principal_type TEXT NOT NULL DEFAULT 'user'
  CHECK (principal_type IN ('user', 'group'));

ALTER TABLE rebac_principal DROP CONSTRAINT rebac_principal_pkey;
ALTER TABLE rebac_principal
  ADD PRIMARY KEY(access_id, principal_type, issuer, subject, relation);

DROP INDEX ix_rebac_principal_subject;
CREATE INDEX ix_rebac_principal_subject
  ON rebac_principal(principal_type, issuer, subject, relation, access_id);

ALTER TABLE rebac_grant
  ADD COLUMN principal_type TEXT NOT NULL DEFAULT 'user'
  CHECK (principal_type IN ('user', 'group'));

UPDATE basyxsystem SET schema_version = 'v1.1.20', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
