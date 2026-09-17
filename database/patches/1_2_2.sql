-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.2
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Sets earlier automatic vacuum and index cleanup on high-churn Submodel
--   Element tables while preserving explicit operator settings.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

DO $$
DECLARE
  target_table TEXT;
  target_oid OID;
BEGIN
  IF (SELECT source FROM pg_settings WHERE name = 'autovacuum_vacuum_scale_factor') = 'default' THEN
    FOREACH target_table IN ARRAY ARRAY[
      'submodel_element',
      'submodel_element_payload',
      'submodel_element_semantic_id_reference_payload'
    ] LOOP
      SELECT c.oid INTO target_oid
      FROM pg_class AS c
      JOIN pg_namespace AS n ON n.oid = c.relnamespace
      WHERE n.nspname = current_schema()
        AND c.relname = target_table
        AND c.relkind = 'r';

      IF target_oid IS NULL THEN
        RAISE EXCEPTION 'BASYXCFG-PATCH-AUTOVACUUMTABLE: required table % is missing', target_table;
      END IF;

      EXECUTE format('LOCK TABLE %I.%I IN SHARE UPDATE EXCLUSIVE MODE', current_schema(), target_table);

      IF NOT EXISTS (
        SELECT 1
        FROM pg_class AS c
        LEFT JOIN pg_class AS toast ON toast.oid = c.reltoastrelid,
             unnest(
               COALESCE(c.reloptions, ARRAY[]::TEXT[])
               || COALESCE(toast.reloptions, ARRAY[]::TEXT[])
             ) AS option
        WHERE c.oid = target_oid
          AND (
            starts_with(split_part(option, '=', 1), 'autovacuum_')
            OR starts_with(split_part(option, '=', 1), 'vacuum_')
          )
      ) THEN
        EXECUTE format(
          'ALTER TABLE %I.%I SET (autovacuum_vacuum_scale_factor = 0.002, vacuum_index_cleanup = on)',
          current_schema(),
          target_table
        );
      END IF;
    END LOOP;
  END IF;
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
