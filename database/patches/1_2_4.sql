-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.4
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
-- A role or database default must not abort the backfill partway.
SET LOCAL statement_timeout = 0;
-- PostgreSQL 17 and later can also end the transaction through a role or database default.
DO $timeouts$
BEGIN
  IF current_setting('server_version_num')::integer >= 170000 THEN
    PERFORM set_config('transaction_timeout', '0', true);
  END IF;
END;
$timeouts$;

LOCK TABLE property_element, range_element IN ACCESS EXCLUSIVE MODE;

-- Each legacy value is rendered like the previous API output of a UTC session, independent of
-- TimeZone and DateStyle:
--   COALESCE(time, date, UTC timestamp with "+00:00"), where BC timestamps start with "-"
--   and infinite timestamps keep the text "infinity" or "-infinity".
-- The expression is written inline so the patch needs no TEMPORARY privilege.

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
          COALESCE(to_json(value_time) #>> '{}', to_json(value_date) #>> '{}', regexp_replace(to_json(timezone('UTC', value_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(value_datetime) THEN '+00:00' ELSE '' END))
    )
  ORDER BY id
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
          COALESCE(to_json(min_time) #>> '{}', to_json(min_date) #>> '{}', regexp_replace(to_json(timezone('UTC', min_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(min_datetime) THEN '+00:00' ELSE '' END))
    )
  ) OR (
    num_nonnulls(max_time, max_date, max_datetime) > 0
    AND (
      num_nonnulls(max_time, max_date, max_datetime) > 1
      OR max_num IS NOT NULL
      OR (max_text IS NOT NULL AND max_text COLLATE "C" <>
          COALESCE(to_json(max_time) #>> '{}', to_json(max_date) #>> '{}', regexp_replace(to_json(timezone('UTC', max_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(max_datetime) THEN '+00:00' ELSE '' END))
    )
  )
  ORDER BY id
  LIMIT 1;
  IF FOUND THEN
    RAISE EXCEPTION 'BASYXCFG-TEMPORAL-CONFLICT: range_element.id % has conflicting temporal storage for a bound', conflict_id;
  END IF;
END;
$preflight$;

UPDATE property_element
SET value_text = COALESCE(to_json(value_time) #>> '{}', to_json(value_date) #>> '{}', regexp_replace(to_json(timezone('UTC', value_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(value_datetime) THEN '+00:00' ELSE '' END)
WHERE value_text IS NULL
  AND num_nonnulls(value_time, value_date, value_datetime) > 0;

UPDATE range_element
SET min_text = COALESCE(min_text, COALESCE(to_json(min_time) #>> '{}', to_json(min_date) #>> '{}', regexp_replace(to_json(timezone('UTC', min_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(min_datetime) THEN '+00:00' ELSE '' END)),
    max_text = COALESCE(max_text, COALESCE(to_json(max_time) #>> '{}', to_json(max_date) #>> '{}', regexp_replace(to_json(timezone('UTC', max_datetime)) #>> '{}', '^(.+) BC$', E'-\\1') || CASE WHEN isfinite(max_datetime) THEN '+00:00' ELSE '' END))
WHERE (min_text IS NULL AND num_nonnulls(min_time, min_date, min_datetime) > 0)
   OR (max_text IS NULL AND num_nonnulls(max_time, max_date, max_datetime) > 0);

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
SET schema_version = 'v1.2.4',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
