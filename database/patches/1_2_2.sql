-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Indexes exact property text lookups used to resolve DPP metadata IDs.
--   A hash index supports arbitrary property text lengths without B-tree
--   index-entry size restrictions.
--   GIN indexes cover existing AAS and Submodel history snapshots and diffs
--   for historical DPP ownership searches. jsonb_ops supports the recursive
--   JSONPath accessor used to find candidate versions.
--   A live inbound-reference inventory tracks ModelReferences to Submodels
--   across normalized reference keys and JSON payloads. Source triggers keep
--   the inventory current and lock newly referenced Submodels so concurrent
--   reference creation cannot race with reference-aware deletion.
--   Owner lookup is indexed for cascading deletion; unchanged references
--   retain their inventory rows without replacement writes.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE INDEX IF NOT EXISTS ix_property_element_value_text_hash
  ON property_element USING HASH (value_text)
  WHERE value_text IS NOT NULL;

CREATE INDEX IF NOT EXISTS ix_aas_history_payload_snapshot_identifiers
  ON aas_history_payload USING GIN (snapshot jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_aas_history_payload_diff_identifiers
  ON aas_history_payload USING GIN (diff jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_submodel_history_payload_snapshot_identifiers
  ON submodel_history_payload USING GIN (snapshot jsonb_ops);

CREATE INDEX IF NOT EXISTS ix_submodel_history_payload_diff_identifiers
  ON submodel_history_payload USING GIN (diff jsonb_ops);

ALTER TABLE concept_description
  ADD COLUMN IF NOT EXISTS inbound_reference_source_id BIGINT GENERATED ALWAYS AS IDENTITY;

CREATE TABLE IF NOT EXISTS submodel_inbound_reference (
  source_table NAME NOT NULL,
  source_id BIGINT NOT NULL,
  owner_submodel_id BIGINT REFERENCES submodel(id) ON DELETE CASCADE,
  target_id TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_submodel_inbound_reference_source
  ON submodel_inbound_reference(source_table, source_id);

CREATE INDEX IF NOT EXISTS ix_submodel_inbound_reference_owner
  ON submodel_inbound_reference(owner_submodel_id);

CREATE INDEX IF NOT EXISTS ix_submodel_inbound_reference_target_hash
  ON submodel_inbound_reference USING HASH (target_id);

CREATE OR REPLACE FUNCTION basyx_submodel_targets_from_json(payload JSONB)
RETURNS TABLE(target_id TEXT)
LANGUAGE SQL
IMMUTABLE
PARALLEL SAFE
AS $$
  WITH RECURSIVE nodes(value) AS (
    SELECT payload
    WHERE payload IS NOT NULL
    UNION ALL
    SELECT child.value
    FROM nodes
    CROSS JOIN LATERAL (
      SELECT object_value AS value
      FROM jsonb_each(
        CASE jsonb_typeof(nodes.value)
          WHEN 'object' THEN nodes.value
          ELSE '{}'::jsonb
        END
      ) AS object_child(object_key, object_value)
      UNION ALL
      SELECT array_value AS value
      FROM jsonb_array_elements(
        CASE jsonb_typeof(nodes.value)
          WHEN 'array' THEN nodes.value
          ELSE '[]'::jsonb
        END
      ) AS array_child(array_value)
    ) AS child
  )
  SELECT DISTINCT key.value ->> 'value'
  FROM nodes
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(nodes.value) = 'object'
       AND nodes.value ->> 'type' = 'ModelReference'
       AND jsonb_typeof(nodes.value -> 'keys') = 'array'
        THEN nodes.value -> 'keys'
      ELSE '[]'::jsonb
    END
  ) AS key(value)
  WHERE key.value ->> 'type' = 'Submodel'
    AND NULLIF(key.value ->> 'value', '') IS NOT NULL;
$$;

CREATE OR REPLACE FUNCTION basyx_submodel_inbound_owner(
  owner_kind TEXT,
  owner_value TEXT
)
RETURNS BIGINT
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
  owner_id BIGINT;
BEGIN
  IF owner_kind = 'none' OR owner_value IS NULL THEN
    RETURN NULL;
  END IF;
  IF owner_kind = 'submodel' THEN
    RETURN owner_value::bigint;
  END IF;
  IF owner_kind = 'sme' THEN
    SELECT submodel_id
    INTO owner_id
    FROM submodel_element
    WHERE id = owner_value::bigint;
    RETURN owner_id;
  END IF;
  IF owner_kind = 'submodel_supplemental_reference' THEN
    SELECT submodel_id
    INTO owner_id
    FROM submodel_supplemental_semantic_id_reference
    WHERE id = owner_value::bigint;
    RETURN owner_id;
  END IF;
  IF owner_kind = 'sme_supplemental_reference' THEN
    SELECT element.submodel_id
    INTO owner_id
    FROM submodel_element_supplemental_semantic_id_reference AS reference
    JOIN submodel_element AS element
      ON element.id = reference.submodel_element_id
    WHERE reference.id = owner_value::bigint;
    RETURN owner_id;
  END IF;
  RAISE EXCEPTION 'DATABASE-PATCH-1_2_2-BADOWNER unsupported inbound-reference owner kind %', owner_kind;
END;
$$;

CREATE OR REPLACE FUNCTION basyx_replace_submodel_inbound_references(
  inbound_source_table NAME,
  inbound_source_id BIGINT,
  inbound_owner_submodel_id BIGINT,
  inbound_target_ids TEXT[]
)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
  old_target_ids TEXT[];
  new_target_ids TEXT[];
  same_owner BOOLEAN;
  target TEXT;
  observed_target_id BIGINT;
  locked_target_id BIGINT;
BEGIN
  SELECT COALESCE(array_agg(DISTINCT target_id ORDER BY target_id), ARRAY[]::text[]),
         COALESCE(bool_and(owner_submodel_id IS NOT DISTINCT FROM inbound_owner_submodel_id), TRUE)
  INTO old_target_ids, same_owner
  FROM submodel_inbound_reference
  WHERE source_table = inbound_source_table
    AND source_id = inbound_source_id;

  SELECT COALESCE(array_agg(DISTINCT target_id ORDER BY target_id), ARRAY[]::text[])
  INTO new_target_ids
  FROM unnest(COALESCE(inbound_target_ids, ARRAY[]::text[])) AS targets(target_id)
  WHERE NULLIF(target_id, '') IS NOT NULL;

  IF old_target_ids = new_target_ids AND same_owner THEN
    RETURN;
  END IF;

  DELETE FROM submodel_inbound_reference
  WHERE source_table = inbound_source_table
    AND source_id = inbound_source_id;

  INSERT INTO submodel_inbound_reference(source_table, source_id, owner_submodel_id, target_id)
  SELECT inbound_source_table, inbound_source_id, inbound_owner_submodel_id, target_id
  FROM unnest(new_target_ids) AS targets(target_id);

  FOR target IN
    SELECT DISTINCT reference.target_id
    FROM submodel_inbound_reference AS reference
    WHERE reference.source_table = inbound_source_table
      AND reference.source_id = inbound_source_id
      AND NOT reference.target_id = ANY(old_target_ids)
    ORDER BY reference.target_id
  LOOP
    observed_target_id := NULL;
    SELECT id
    INTO observed_target_id
    FROM submodel
    WHERE submodel_identifier = target;

    IF observed_target_id IS NULL THEN
      CONTINUE;
    END IF;

    locked_target_id := NULL;
    SELECT id
    INTO locked_target_id
    FROM submodel
    WHERE id = observed_target_id
    FOR KEY SHARE;

    IF locked_target_id IS NULL THEN
      RAISE EXCEPTION USING
        ERRCODE = '40001',
        MESSAGE = 'DATABASE-PATCH-1_2_2-INBOUNDREFRACE target Submodel disappeared while adding an inbound reference';
    END IF;
  END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION basyx_sync_submodel_inbound_json_references()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  source_row JSONB;
  source_id BIGINT;
  owner_id BIGINT;
  target_ids TEXT[];
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW IS NOT DISTINCT FROM OLD THEN
      RETURN NEW;
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN
    source_row := to_jsonb(OLD);
  ELSE
    source_row := to_jsonb(NEW);
  END IF;
  source_id := (source_row ->> TG_ARGV[0])::bigint;

  IF TG_OP = 'DELETE' THEN
    target_ids := ARRAY[]::text[];
    owner_id := NULL;
  ELSE
    owner_id := basyx_submodel_inbound_owner(TG_ARGV[1], source_row ->> TG_ARGV[2]);
    SELECT COALESCE(array_agg(target_id ORDER BY target_id), ARRAY[]::text[])
    INTO target_ids
    FROM basyx_submodel_targets_from_json(source_row);
  END IF;

  PERFORM basyx_replace_submodel_inbound_references(TG_TABLE_NAME, source_id, owner_id, target_ids);
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION basyx_sync_submodel_inbound_key_reference()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  source_row JSONB;
  source_id BIGINT;
  reference_id BIGINT;
  reference_owner_value TEXT;
  owner_id BIGINT;
  target_ids TEXT[] := ARRAY[]::text[];
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW IS NOT DISTINCT FROM OLD THEN
      RETURN NEW;
    END IF;
  END IF;
  IF TG_OP = 'DELETE' THEN
    source_row := to_jsonb(OLD);
  ELSE
    source_row := to_jsonb(NEW);
  END IF;
  source_id := (source_row ->> 'id')::bigint;

  IF TG_OP <> 'DELETE' THEN
    IF TG_ARGV[3] = 'conservative' THEN
      target_ids := ARRAY[source_row ->> 'value'];
    ELSIF (source_row ->> 'type')::integer = 20 THEN
      reference_id := (source_row ->> 'reference_id')::bigint;
      EXECUTE format(
        'SELECT %I::text FROM %I WHERE id = $1',
        TG_ARGV[2],
        TG_ARGV[1]
      )
      INTO reference_owner_value
      USING reference_id;
      IF reference_owner_value IS NOT NULL THEN
        target_ids := ARRAY[source_row ->> 'value'];
        owner_id := basyx_submodel_inbound_owner(TG_ARGV[0], reference_owner_value);
      END IF;
    END IF;
  END IF;

  PERFORM basyx_replace_submodel_inbound_references(TG_TABLE_NAME, source_id, owner_id, target_ids);
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$;

DELETE FROM submodel_inbound_reference;

DO $$
DECLARE
  source RECORD;
BEGIN
  FOR source IN
    SELECT *
    FROM (VALUES
      ('aas_payload', 'aas_id', 'none', 'aas_id'),
      ('aas_submodel_reference_payload', 'id', 'none', 'id'),
      ('submodel_payload', 'submodel_id', 'submodel', 'submodel_id'),
      ('submodel_element_payload', 'submodel_element_id', 'sme', 'submodel_element_id'),
      ('property_element_payload', 'property_element_id', 'sme', 'property_element_id'),
      ('multilanguage_property_payload', 'submodel_element_id', 'sme', 'submodel_element_id'),
      ('reference_element', 'id', 'sme', 'id'),
      ('relationship_element', 'id', 'sme', 'id'),
      ('annotated_relationship_element', 'id', 'sme', 'id'),
      ('submodel_element_list', 'id', 'sme', 'id'),
      ('entity_element', 'id', 'sme', 'id'),
      ('operation_element', 'id', 'sme', 'id'),
      ('basic_event_element', 'id', 'sme', 'id'),
      ('qualifier_payload', 'qualifier_id', 'none', 'qualifier_id'),
      ('concept_description', 'inbound_reference_source_id', 'none', 'inbound_reference_source_id'),
      ('specific_asset_id_payload', 'specific_asset_id', 'none', 'specific_asset_id'),
      ('descriptor_payload', 'descriptor_id', 'none', 'descriptor_id'),
      ('submodel_semantic_id_reference_payload', 'id', 'submodel', 'reference_id'),
      ('submodel_supplemental_semantic_id_reference_payload', 'id', 'submodel_supplemental_reference', 'reference_id'),
      ('submodel_element_semantic_id_reference_payload', 'id', 'sme', 'reference_id'),
      ('submodel_element_supplemental_semantic_id_reference_payload', 'id', 'sme_supplemental_reference', 'reference_id'),
      ('submodel_descriptor_semantic_id_reference_payload', 'id', 'none', 'id'),
      ('submodel_descriptor_supplemental_semantic_id_reference_payload', 'id', 'none', 'id'),
      ('specific_asset_id_external_subject_id_reference_payload', 'id', 'none', 'id'),
      ('specific_asset_id_supplemental_semantic_id_reference_payload', 'id', 'none', 'id')
    ) AS sources(source_table, id_column, owner_kind, owner_column)
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', 'sync_submodel_inbound_reference', source.source_table);
    EXECUTE format(
      'CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION basyx_sync_submodel_inbound_json_references(%L, %L, %L)',
      'sync_submodel_inbound_reference',
      source.source_table,
      source.id_column,
      source.owner_kind,
      source.owner_column
    );
    EXECUTE format(
      'SELECT basyx_replace_submodel_inbound_references(%L, row.%I::bigint, basyx_submodel_inbound_owner(%L, to_jsonb(row) ->> %L), ARRAY(SELECT target_id FROM basyx_submodel_targets_from_json(to_jsonb(row)))) FROM %I AS row',
      source.source_table,
      source.id_column,
      source.owner_kind,
      source.owner_column,
      source.source_table
    );
  END LOOP;
END;
$$;

DO $$
DECLARE
  source RECORD;
BEGIN
  FOR source IN
    SELECT *
    FROM (VALUES
      ('aas_submodel_reference_key', 'none', 'aas_submodel_reference', 'id', 'conservative'),
      ('submodel_semantic_id_reference_key', 'submodel', 'submodel_semantic_id_reference', 'id', 'typed'),
      ('submodel_supplemental_semantic_id_reference_key', 'submodel', 'submodel_supplemental_semantic_id_reference', 'submodel_id', 'typed'),
      ('submodel_element_semantic_id_reference_key', 'sme', 'submodel_element_semantic_id_reference', 'id', 'typed'),
      ('submodel_element_supplemental_semantic_id_reference_key', 'sme', 'submodel_element_supplemental_semantic_id_reference', 'submodel_element_id', 'typed'),
      ('submodel_descriptor_semantic_id_reference_key', 'none', 'submodel_descriptor_semantic_id_reference', 'id', 'typed'),
      ('submodel_descriptor_supplemental_semantic_id_reference_key', 'none', 'submodel_descriptor_supplemental_semantic_id_reference', 'descriptor_id', 'typed'),
      ('specific_asset_id_external_subject_id_reference_key', 'none', 'specific_asset_id_external_subject_id_reference', 'id', 'typed'),
      ('specific_asset_id_supplemental_semantic_id_reference_key', 'none', 'specific_asset_id_supplemental_semantic_id_reference', 'specific_asset_id_id', 'typed')
    ) AS sources(source_table, owner_kind, reference_table, owner_column, matching_mode)
  LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', 'sync_submodel_inbound_reference', source.source_table);
    EXECUTE format(
      'CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION basyx_sync_submodel_inbound_key_reference(%L, %L, %L, %L)',
      'sync_submodel_inbound_reference',
      source.source_table,
      source.owner_kind,
      source.reference_table,
      source.owner_column,
      source.matching_mode
    );
    EXECUTE format(
      'SELECT basyx_replace_submodel_inbound_references(%L, key.id, CASE WHEN %L = ''none'' THEN NULL ELSE basyx_submodel_inbound_owner(%L, reference.%I::text) END, ARRAY[key.value]) FROM %I AS key JOIN %I AS reference ON reference.id = key.reference_id WHERE %L = ''conservative'' OR key.type = 20',
      source.source_table,
      source.owner_kind,
      source.owner_kind,
      source.owner_column,
      source.source_table,
      source.reference_table,
      source.matching_mode
    );
  END LOOP;
END;
$$;

UPDATE basyxsystem
SET schema_version = 'v1.2.2',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
