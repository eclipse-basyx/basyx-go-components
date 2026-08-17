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

-- Separates SubmodelElement path uniqueness from sibling identity and ordering
-- while treating all top-level elements as members of the same parent scope.

ALTER TABLE submodel_element
  ADD COLUMN parent_scope_id BIGINT
  GENERATED ALWAYS AS (COALESCE(parent_sme_id, 0)) STORED;

ALTER TABLE submodel_element
  DROP CONSTRAINT IF EXISTS uq_sibling_idshort;

ALTER TABLE submodel_element
  DROP CONSTRAINT IF EXISTS uq_sibling_pos;

ALTER TABLE submodel_element
  ADD CONSTRAINT uq_sme_path
  UNIQUE (submodel_id, idshort_path)
  DEFERRABLE INITIALLY IMMEDIATE;

ALTER TABLE submodel_element
  ADD CONSTRAINT uq_sibling_idshort
  UNIQUE (submodel_id, parent_scope_id, id_short)
  DEFERRABLE INITIALLY IMMEDIATE;

ALTER TABLE submodel_element
  ADD CONSTRAINT uq_sibling_pos
  UNIQUE (submodel_id, parent_scope_id, position)
  DEFERRABLE INITIALLY IMMEDIATE;

DROP INDEX IF EXISTS ix_sme_sub_path;

UPDATE basyxsystem
SET schema_version = 'v1.1.15',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
