-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.5
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Backfills the text representation of legacy Property and Range values,
--   then removes their nine typed temporal columns.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

-- The configuration service runs this entire patch in one transaction.
SET LOCAL lock_timeout = '5s';
LOCK TABLE property_element, range_element IN ACCESS EXCLUSIVE MODE;

-- Match the legacy API and AASQL rendering, independent of TimeZone and DateStyle.
CREATE FUNCTION pg_temp.basyx_legacy_temporal_text(
  legacy_time time, legacy_date date, legacy_datetime timestamptz
)
RETURNS text
LANGUAGE sql
STABLE
AS $format$
  SELECT COALESCE(
    to_json(legacy_time) #>> '{}',
    to_json(legacy_date) #>> '{}',
    regexp_replace(to_json(timezone('UTC', legacy_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || '+00:00'
  );
$format$;

DO $preflight$
DECLARE
  conflict_id bigint;
BEGIN
  -- Matching text can coexist with a legacy value after a staged backfill.
  -- A different text value or another populated value column requires review.
  SELECT id INTO conflict_id
  FROM property_element
  WHERE num_nonnulls(value_time, value_date, value_datetime) > 0
    AND (
      num_nonnulls(value_time, value_date, value_datetime) > 1
      OR value_num IS NOT NULL
      OR value_bool IS NOT NULL
      OR (value_text IS NOT NULL AND value_text COLLATE "C" <>
          pg_temp.basyx_legacy_temporal_text(value_time, value_date, value_datetime))
    )
  LIMIT 1;
  IF FOUND THEN
    RAISE EXCEPTION 'BASYXCFG-TEMPORAL-CONFLICT: property_element.id % has conflicting temporal storage', conflict_id;
  END IF;

  SELECT id INTO conflict_id
  FROM range_element
  WHERE (
    num_nonnulls(min_time, min_date, min_datetime) > 0
    AND (
      num_nonnulls(min_time, min_date, min_datetime) > 1
      OR min_num IS NOT NULL
      OR (min_text IS NOT NULL AND min_text COLLATE "C" <>
          pg_temp.basyx_legacy_temporal_text(min_time, min_date, min_datetime))
    )
  ) OR (
    num_nonnulls(max_time, max_date, max_datetime) > 0
    AND (
      num_nonnulls(max_time, max_date, max_datetime) > 1
      OR max_num IS NOT NULL
      OR (max_text IS NOT NULL AND max_text COLLATE "C" <>
          pg_temp.basyx_legacy_temporal_text(max_time, max_date, max_datetime))
    )
  )
  LIMIT 1;
  IF FOUND THEN
    RAISE EXCEPTION 'BASYXCFG-TEMPORAL-CONFLICT: range_element.id % has conflicting temporal storage for a bound', conflict_id;
  END IF;
END;
$preflight$;

UPDATE property_element
SET value_text = pg_temp.basyx_legacy_temporal_text(value_time, value_date, value_datetime)
WHERE value_text IS NULL
  AND num_nonnulls(value_time, value_date, value_datetime) > 0;

UPDATE range_element
SET min_text = COALESCE(min_text, pg_temp.basyx_legacy_temporal_text(min_time, min_date, min_datetime)),
    max_text = COALESCE(max_text, pg_temp.basyx_legacy_temporal_text(max_time, max_date, max_datetime))
WHERE (min_text IS NULL AND num_nonnulls(min_time, min_date, min_datetime) > 0)
   OR (max_text IS NULL AND num_nonnulls(max_time, max_date, max_datetime) > 0);

DROP FUNCTION pg_temp.basyx_legacy_temporal_text(time, date, timestamptz);

ALTER TABLE property_element
  DROP COLUMN value_time RESTRICT,
  DROP COLUMN value_date RESTRICT,
  DROP COLUMN value_datetime RESTRICT;

ALTER TABLE range_element
  DROP COLUMN min_time RESTRICT,
  DROP COLUMN min_date RESTRICT,
  DROP COLUMN min_datetime RESTRICT,
  DROP COLUMN max_time RESTRICT,
  DROP COLUMN max_date RESTRICT,
  DROP COLUMN max_datetime RESTRICT;

UPDATE basyxsystem
SET schema_version = 'v1.2.5',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
