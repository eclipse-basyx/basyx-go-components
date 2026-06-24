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

DROP TABLE IF EXISTS pg_temp.tmp_global_asset_id_subject_keys;

CREATE TEMP TABLE tmp_global_asset_id_subject_keys ON COMMIT DROP AS
SELECT
  global_asset_id.id AS global_specific_asset_id,
  MIN(source_reference.type) AS reference_type,
  source_key.type AS key_type,
  BTRIM(source_key.value) AS key_value,
  MIN(source_asset_id.position) AS asset_position,
  MIN(source_key.position) AS key_position
FROM specific_asset_id AS global_asset_id
JOIN aas_descriptor AS descriptor
  ON descriptor.descriptor_id = global_asset_id.descriptor_id
 AND descriptor.global_asset_id = global_asset_id.value
JOIN specific_asset_id AS source_asset_id
  ON source_asset_id.descriptor_id = global_asset_id.descriptor_id
 AND source_asset_id.id <> global_asset_id.id
JOIN specific_asset_id_external_subject_id_reference AS source_reference
  ON source_reference.id = source_asset_id.id
JOIN specific_asset_id_external_subject_id_reference_key AS source_key
  ON source_key.reference_id = source_reference.id
WHERE global_asset_id.name = 'globalAssetId'
  AND global_asset_id.descriptor_id IS NOT NULL
  AND (
    BTRIM(source_key.value) = 'PUBLIC_READABLE'
    OR BTRIM(source_key.value) LIKE 'BPN%'
  )
GROUP BY global_asset_id.id, source_key.type, BTRIM(source_key.value);

CREATE UNIQUE INDEX ux_tmp_global_asset_id_subject_keys
  ON tmp_global_asset_id_subject_keys(global_specific_asset_id, key_type, key_value);

ANALYZE tmp_global_asset_id_subject_keys;

INSERT INTO specific_asset_id_external_subject_id_reference(id, type)
SELECT global_specific_asset_id, MIN(reference_type)
FROM tmp_global_asset_id_subject_keys
GROUP BY global_specific_asset_id
ON CONFLICT (id) DO NOTHING;

INSERT INTO specific_asset_id_external_subject_id_reference_payload(reference_id, parent_reference_payload)
SELECT global_specific_asset_id, '{}'::jsonb
FROM tmp_global_asset_id_subject_keys
WHERE NOT EXISTS (
  SELECT 1
  FROM specific_asset_id_external_subject_id_reference_payload AS payload
  WHERE payload.reference_id = tmp_global_asset_id_subject_keys.global_specific_asset_id
)
GROUP BY global_specific_asset_id;

WITH missing_keys AS (
  SELECT tmp_global_asset_id_subject_keys.*
  FROM tmp_global_asset_id_subject_keys
  WHERE NOT EXISTS (
    SELECT 1
    FROM specific_asset_id_external_subject_id_reference_key AS existing_key
    WHERE existing_key.reference_id = tmp_global_asset_id_subject_keys.global_specific_asset_id
      AND existing_key.type = tmp_global_asset_id_subject_keys.key_type
      AND existing_key.value = tmp_global_asset_id_subject_keys.key_value
  )
),
positioned_keys AS (
  SELECT
    missing_keys.global_specific_asset_id AS reference_id,
    COALESCE(existing_positions.max_position, -1)
      + ROW_NUMBER() OVER (
          PARTITION BY missing_keys.global_specific_asset_id
          ORDER BY missing_keys.asset_position, missing_keys.key_position, missing_keys.key_type, missing_keys.key_value
        ) AS position,
    missing_keys.key_type AS type,
    missing_keys.key_value AS value
  FROM missing_keys
  LEFT JOIN (
    SELECT existing_key.reference_id, MAX(existing_key.position) AS max_position
    FROM specific_asset_id_external_subject_id_reference_key AS existing_key
    JOIN (
      SELECT global_specific_asset_id
      FROM tmp_global_asset_id_subject_keys
      GROUP BY global_specific_asset_id
    ) AS candidate_references
      ON candidate_references.global_specific_asset_id = existing_key.reference_id
    GROUP BY existing_key.reference_id
  ) AS existing_positions
    ON existing_positions.reference_id = missing_keys.global_specific_asset_id
)
INSERT INTO specific_asset_id_external_subject_id_reference_key(reference_id, position, type, value)
SELECT reference_id, position, type, value
FROM positioned_keys;

DROP TABLE IF EXISTS pg_temp.tmp_global_asset_id_subject_keys;
