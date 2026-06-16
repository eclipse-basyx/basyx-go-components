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
-- Patch Version  : 1.1.5
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Adds reference-level positions for submodel descriptor supplemental
--   semantic IDs so registry QL fragments can target indexed references.
-- ============================================================================

ALTER TABLE IF EXISTS submodel_descriptor_supplemental_semantic_id_reference
    ADD COLUMN IF NOT EXISTS position INTEGER;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (PARTITION BY descriptor_id ORDER BY id ASC) - 1 AS computed_position
    FROM submodel_descriptor_supplemental_semantic_id_reference
)
UPDATE submodel_descriptor_supplemental_semantic_id_reference AS ref
SET position = ranked.computed_position
FROM ranked
WHERE ref.id = ranked.id
  AND ref.position IS NULL;

ALTER TABLE IF EXISTS submodel_descriptor_supplemental_semantic_id_reference
    ALTER COLUMN position SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_smdesc_supp_sem_descriptor_position
    ON submodel_descriptor_supplemental_semantic_id_reference(descriptor_id, position);

UPDATE basyxsystem
SET schema_version = 'v1.1.5',
    state = 'clean'
WHERE identifier = (
    SELECT identifier
    FROM basyxsystem
    ORDER BY identifier ASC
    LIMIT 1
);
