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

-- The ReBAC feature branch used the prerelease versions v1.1.18-v1.1.20
-- before main assigned v1.1.18 and v1.1.19. Reapply main's additions
-- idempotently so databases created from either lineage converge here.

DO $minimum_postgres_version$
BEGIN
  IF current_setting('server_version_num')::integer < 160000 THEN
    RAISE EXCEPTION 'DATABASE-PATCH-1_1_21-POSTGRESVERSION PostgreSQL 16 or newer is required';
  END IF;
END
$minimum_postgres_version$;

CREATE TABLE IF NOT EXISTS aasx_async_upload (
  handle_id TEXT PRIMARY KEY REFERENCES async_job(handle_id) ON DELETE CASCADE,
  file_oid OID NOT NULL UNIQUE,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  promoted BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION cleanup_aasx_async_upload_large_object()
RETURNS TRIGGER AS $$
BEGIN
  IF NOT OLD.promoted THEN
    PERFORM lo_unlink(OLD.file_oid);
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cleanup_aasx_async_upload_large_object ON aasx_async_upload;
CREATE TRIGGER trg_cleanup_aasx_async_upload_large_object
BEFORE DELETE ON aasx_async_upload
FOR EACH ROW
EXECUTE FUNCTION cleanup_aasx_async_upload_large_object();

CREATE OR REPLACE FUNCTION cleanup_terminal_aasx_async_upload()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.execution_state = 'Running' AND NEW.execution_state <> 'Running' THEN
    DELETE FROM aasx_async_upload WHERE handle_id = NEW.handle_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cleanup_terminal_aasx_async_upload ON async_job;
CREATE TRIGGER trg_cleanup_terminal_aasx_async_upload
AFTER UPDATE OF execution_state ON async_job
FOR EACH ROW
EXECUTE FUNCTION cleanup_terminal_aasx_async_upload();

CREATE OR REPLACE FUNCTION basyx_safe_regex_pattern(pattern_value text)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
STRICT
AS $safe_regex$
BEGIN
  PERFORM '' ~ pattern_value;
  RETURN pattern_value;
EXCEPTION
  WHEN invalid_regular_expression THEN
    RETURN NULL;
END
$safe_regex$;

CREATE OR REPLACE FUNCTION basyx_validated_cast_input(input_value text, type_name text)
RETURNS text
LANGUAGE plpgsql
STABLE
PARALLEL SAFE
STRICT
AS $validated_cast$
BEGIN
  IF (CASE type_name
    WHEN 'double precision' THEN pg_catalog.pg_input_is_valid(input_value, 'double precision')
    WHEN 'boolean' THEN pg_catalog.pg_input_is_valid(input_value, 'boolean')
    WHEN 'timestamp with time zone' THEN pg_catalog.pg_input_is_valid(input_value, 'timestamp with time zone')
    WHEN 'time without time zone' THEN pg_catalog.pg_input_is_valid(input_value, 'time without time zone')
    ELSE false
  END) THEN
    RETURN input_value;
  END IF;
  RETURN NULL;
END
$validated_cast$;

UPDATE basyxsystem SET schema_version = 'v1.1.21', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
