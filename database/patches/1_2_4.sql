-- ============================================================================
-- Project        : Eclipse BaSyx
-- Organization   : Fraunhofer IESE
-- File Type      : SQL Patch Script
-- Patch Version  : 1.2.4
-- Metamodel Ver. : 3.2
-- ----------------------------------------------------------------------------
-- Description:
--   Parse temporal query operands in UTC and preserve their calendar date
--   when extracting a date part from sub-microsecond XSD values.
--
-- Copyright (c) Eclipse BaSyx Authors and Fraunhofer IESE
-- SPDX-License-Identifier: MIT
-- ============================================================================

CREATE OR REPLACE FUNCTION basyx_parse_datetime(input_value text, for_date_part boolean DEFAULT false)
RETURNS timestamptz
LANGUAGE plpgsql
STABLE
PARALLEL SAFE
STRICT
SET TimeZone = 'UTC'
AS $parse_datetime$
DECLARE
  normalized text := btrim(input_value);
BEGIN
  -- PostgreSQL needs an explicit midnight to disambiguate negative offsets on dates.
  normalized := regexp_replace(normalized,
    '^([0-9]{4,}-[0-9]{2}-[0-9]{2})([+-][0-9]{2}:[0-9]{2}|Z)$', '\1T00:00:00\2');
  IF for_date_part THEN
    -- Rounding the last fraction of a day can otherwise change its year, month or day.
    normalized := regexp_replace(normalized, '(:[0-9]{2}\.[0-9]{6})[0-9]+', '\1');
  END IF;
  RETURN basyx_validated_cast_input(normalized, 'timestamp with time zone')::timestamptz;
END
$parse_datetime$;

UPDATE basyxsystem
SET schema_version = 'v1.2.4',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
