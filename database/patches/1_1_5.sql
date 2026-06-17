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

ALTER TABLE IF EXISTS specific_asset_id_supplemental_semantic_id_reference
  ADD COLUMN IF NOT EXISTS position INTEGER;

WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (PARTITION BY specific_asset_id_id ORDER BY id) - 1 AS position
  FROM specific_asset_id_supplemental_semantic_id_reference
)
UPDATE specific_asset_id_supplemental_semantic_id_reference AS reference
SET position = ranked.position
FROM ranked
WHERE reference.id = ranked.id
  AND reference.position IS NULL;

ALTER TABLE IF EXISTS specific_asset_id_supplemental_semantic_id_reference
  ALTER COLUMN position SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_specasset_supp_semantic_owner_position
  ON specific_asset_id_supplemental_semantic_id_reference(specific_asset_id_id, position);

ALTER TABLE IF EXISTS submodel_descriptor_supplemental_semantic_id_reference
  ADD COLUMN IF NOT EXISTS position INTEGER;

WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (PARTITION BY descriptor_id ORDER BY id) - 1 AS position
  FROM submodel_descriptor_supplemental_semantic_id_reference
)
UPDATE submodel_descriptor_supplemental_semantic_id_reference AS reference
SET position = ranked.position
FROM ranked
WHERE reference.id = ranked.id
  AND reference.position IS NULL;

ALTER TABLE IF EXISTS submodel_descriptor_supplemental_semantic_id_reference
  ALTER COLUMN position SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ux_smdesc_supp_semantic_owner_position
  ON submodel_descriptor_supplemental_semantic_id_reference(descriptor_id, position);

UPDATE basyxsystem
SET schema_version = 'v1.1.5',
    state = 'clean';
