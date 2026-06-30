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

CREATE TABLE IF NOT EXISTS submodel_supplemental_semantic_id_reference (
  id BIGSERIAL PRIMARY KEY,
  submodel_id BIGINT NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  type INTEGER NOT NULL,
  UNIQUE(submodel_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_supplemental_semantic_id_reference_key (
  id BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  type INTEGER NOT NULL,
  value TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_supplemental_semantic_id_reference_payload (
  id BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS submodel_element_supplemental_semantic_id_reference (
  id BIGSERIAL PRIMARY KEY,
  submodel_element_id BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  type INTEGER NOT NULL,
  UNIQUE(submodel_element_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_element_supplemental_semantic_id_reference_key (
  id BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_element_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  type INTEGER NOT NULL,
  value TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_element_supplemental_semantic_id_reference_payload (
  id BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_element_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

WITH references_to_migrate AS (
  SELECT
    payload.submodel_id,
    reference.value AS reference_json,
    reference.ordinality - 1 AS position
  FROM submodel_payload AS payload
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(payload.supplemental_semantic_ids_payload) = 'array'
        THEN payload.supplemental_semantic_ids_payload
      ELSE '[]'::jsonb
    END
  ) WITH ORDINALITY AS reference(value, ordinality)
  WHERE NOT EXISTS (
    SELECT 1
    FROM submodel_supplemental_semantic_id_reference AS existing
    WHERE existing.submodel_id = payload.submodel_id
  )
),
inserted_references AS (
  INSERT INTO submodel_supplemental_semantic_id_reference(submodel_id, position, type)
  SELECT
    submodel_id,
    position,
    CASE reference_json ->> 'type'
      WHEN 'ExternalReference' THEN 0
      WHEN 'ModelReference' THEN 1
    END
  FROM references_to_migrate
  WHERE reference_json ->> 'type' IN ('ExternalReference', 'ModelReference')
  RETURNING id, submodel_id, position
)
INSERT INTO submodel_supplemental_semantic_id_reference_payload(reference_id, parent_reference_payload)
SELECT inserted.id, COALESCE(source.reference_json -> 'referredSemanticId', '{}'::jsonb)
FROM inserted_references AS inserted
JOIN references_to_migrate AS source
  ON source.submodel_id = inserted.submodel_id
 AND source.position = inserted.position;

INSERT INTO submodel_supplemental_semantic_id_reference_key(reference_id, position, type, value)
SELECT
  reference.id,
  key.ordinality - 1,
  CASE key.value ->> 'type'
    WHEN 'AnnotatedRelationshipElement' THEN 0
    WHEN 'AssetAdministrationShell' THEN 1
    WHEN 'BasicEventElement' THEN 2
    WHEN 'Blob' THEN 3
    WHEN 'Capability' THEN 4
    WHEN 'ConceptDescription' THEN 5
    WHEN 'DataElement' THEN 6
    WHEN 'Entity' THEN 7
    WHEN 'EventElement' THEN 8
    WHEN 'File' THEN 9
    WHEN 'FragmentReference' THEN 10
    WHEN 'GlobalReference' THEN 11
    WHEN 'Identifiable' THEN 12
    WHEN 'MultiLanguageProperty' THEN 13
    WHEN 'Operation' THEN 14
    WHEN 'Property' THEN 15
    WHEN 'Range' THEN 16
    WHEN 'Referable' THEN 17
    WHEN 'ReferenceElement' THEN 18
    WHEN 'RelationshipElement' THEN 19
    WHEN 'Submodel' THEN 20
    WHEN 'SubmodelElement' THEN 21
    WHEN 'SubmodelElementCollection' THEN 22
    WHEN 'SubmodelElementList' THEN 23
  END,
  key.value ->> 'value'
FROM submodel_supplemental_semantic_id_reference AS reference
JOIN submodel_payload AS payload ON payload.submodel_id = reference.submodel_id
CROSS JOIN LATERAL jsonb_array_elements(
  CASE
    WHEN jsonb_typeof(payload.supplemental_semantic_ids_payload) = 'array'
      THEN payload.supplemental_semantic_ids_payload
    ELSE '[]'::jsonb
  END
) WITH ORDINALITY AS source(value, ordinality)
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(source.value -> 'keys', '[]'::jsonb)) WITH ORDINALITY AS key(value, ordinality)
WHERE source.ordinality - 1 = reference.position
  AND key.value ? 'value'
  AND NOT EXISTS (
    SELECT 1
    FROM submodel_supplemental_semantic_id_reference_key AS existing
    WHERE existing.reference_id = reference.id
  );

WITH references_to_migrate AS (
  SELECT
    payload.submodel_element_id,
    reference.value AS reference_json,
    reference.ordinality - 1 AS position
  FROM submodel_element_payload AS payload
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(payload.supplemental_semantic_ids_payload) = 'array'
        THEN payload.supplemental_semantic_ids_payload
      ELSE '[]'::jsonb
    END
  ) WITH ORDINALITY AS reference(value, ordinality)
  WHERE NOT EXISTS (
    SELECT 1
    FROM submodel_element_supplemental_semantic_id_reference AS existing
    WHERE existing.submodel_element_id = payload.submodel_element_id
  )
),
inserted_references AS (
  INSERT INTO submodel_element_supplemental_semantic_id_reference(submodel_element_id, position, type)
  SELECT
    submodel_element_id,
    position,
    CASE reference_json ->> 'type'
      WHEN 'ExternalReference' THEN 0
      WHEN 'ModelReference' THEN 1
    END
  FROM references_to_migrate
  WHERE reference_json ->> 'type' IN ('ExternalReference', 'ModelReference')
  RETURNING id, submodel_element_id, position
)
INSERT INTO submodel_element_supplemental_semantic_id_reference_payload(reference_id, parent_reference_payload)
SELECT inserted.id, COALESCE(source.reference_json -> 'referredSemanticId', '{}'::jsonb)
FROM inserted_references AS inserted
JOIN references_to_migrate AS source
  ON source.submodel_element_id = inserted.submodel_element_id
 AND source.position = inserted.position;

INSERT INTO submodel_element_supplemental_semantic_id_reference_key(reference_id, position, type, value)
SELECT
  reference.id,
  key.ordinality - 1,
  CASE key.value ->> 'type'
    WHEN 'AnnotatedRelationshipElement' THEN 0
    WHEN 'AssetAdministrationShell' THEN 1
    WHEN 'BasicEventElement' THEN 2
    WHEN 'Blob' THEN 3
    WHEN 'Capability' THEN 4
    WHEN 'ConceptDescription' THEN 5
    WHEN 'DataElement' THEN 6
    WHEN 'Entity' THEN 7
    WHEN 'EventElement' THEN 8
    WHEN 'File' THEN 9
    WHEN 'FragmentReference' THEN 10
    WHEN 'GlobalReference' THEN 11
    WHEN 'Identifiable' THEN 12
    WHEN 'MultiLanguageProperty' THEN 13
    WHEN 'Operation' THEN 14
    WHEN 'Property' THEN 15
    WHEN 'Range' THEN 16
    WHEN 'Referable' THEN 17
    WHEN 'ReferenceElement' THEN 18
    WHEN 'RelationshipElement' THEN 19
    WHEN 'Submodel' THEN 20
    WHEN 'SubmodelElement' THEN 21
    WHEN 'SubmodelElementCollection' THEN 22
    WHEN 'SubmodelElementList' THEN 23
  END,
  key.value ->> 'value'
FROM submodel_element_supplemental_semantic_id_reference AS reference
JOIN submodel_element_payload AS payload ON payload.submodel_element_id = reference.submodel_element_id
CROSS JOIN LATERAL jsonb_array_elements(
  CASE
    WHEN jsonb_typeof(payload.supplemental_semantic_ids_payload) = 'array'
      THEN payload.supplemental_semantic_ids_payload
    ELSE '[]'::jsonb
  END
) WITH ORDINALITY AS source(value, ordinality)
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(source.value -> 'keys', '[]'::jsonb)) WITH ORDINALITY AS key(value, ordinality)
WHERE source.ordinality - 1 = reference.position
  AND key.value ? 'value'
  AND NOT EXISTS (
    SELECT 1
    FROM submodel_element_supplemental_semantic_id_reference_key AS existing
    WHERE existing.reference_id = reference.id
  );

CREATE INDEX IF NOT EXISTS ix_submodel_supp_sem_owner_id ON submodel_supplemental_semantic_id_reference(submodel_id);
CREATE INDEX IF NOT EXISTS ix_submodel_supp_sem_refkey_refid ON submodel_supplemental_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_submodel_supp_sem_refkey_refval ON submodel_supplemental_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_submodel_supp_sem_refkey_type_val ON submodel_supplemental_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_submodel_supp_sem_refkey_val_trgm ON submodel_supplemental_semantic_id_reference_key USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_sme_supp_sem_owner_id ON submodel_element_supplemental_semantic_id_reference(submodel_element_id);
CREATE INDEX IF NOT EXISTS ix_sme_supp_sem_refkey_refid ON submodel_element_supplemental_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_sme_supp_sem_refkey_refval ON submodel_element_supplemental_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_sme_supp_sem_refkey_type_val ON submodel_element_supplemental_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_sme_supp_sem_refkey_val_trgm ON submodel_element_supplemental_semantic_id_reference_key USING GIN (value gin_trgm_ops);

UPDATE basyxsystem
SET schema_version = 'v1.1.7',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
