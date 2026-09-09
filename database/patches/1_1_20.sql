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

CREATE OR REPLACE FUNCTION basyx_validated_cast_input(input_value text, type_name text)
RETURNS text
LANGUAGE plpgsql
STABLE
PARALLEL SAFE
STRICT
AS $validated_cast$
BEGIN
  -- Keep each validator type constant across cached PL/pgSQL executions.
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

UPDATE basyxsystem
SET schema_version = 'v1.1.20',
    state = 'clean'
WHERE identifier = (
  SELECT identifier
  FROM basyxsystem
  ORDER BY identifier ASC
  LIMIT 1
);
