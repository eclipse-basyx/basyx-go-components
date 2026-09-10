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

CREATE TABLE IF NOT EXISTS rebac_scope (
  scope TEXT PRIMARY KEY,
  revision BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS rebac_access (
  id BIGSERIAL PRIMARY KEY,
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  collection TEXT,
  aas_id BIGINT REFERENCES aas(id) ON DELETE CASCADE,
  submodel_id BIGINT REFERENCES submodel(id) ON DELETE CASCADE,
  sme_id BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
  aas_descriptor_id BIGINT REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
  submodel_descriptor_id BIGINT REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  concept_description_id TEXT REFERENCES concept_description(id) ON DELETE CASCADE,
  discovery_aas_id BIGINT REFERENCES aas_identifier(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL DEFAULT 1,
  policy JSONB,
  UNIQUE(scope, collection),
  UNIQUE(scope, aas_id),
  UNIQUE(scope, submodel_id),
  UNIQUE(scope, sme_id)
);

ALTER TABLE rebac_access
  ADD COLUMN IF NOT EXISTS aas_descriptor_id BIGINT REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS submodel_descriptor_id BIGINT REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS concept_description_id TEXT REFERENCES concept_description(id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS discovery_aas_id BIGINT REFERENCES aas_identifier(id) ON DELETE CASCADE;

ALTER TABLE rebac_access DROP CONSTRAINT IF EXISTS rebac_access_collection_check;
ALTER TABLE rebac_access DROP CONSTRAINT IF EXISTS rebac_access_check;
ALTER TABLE rebac_access DROP CONSTRAINT IF EXISTS rebac_access_binding_check;
ALTER TABLE rebac_access ADD CONSTRAINT rebac_access_collection_check CHECK (
  collection IN ('/shells', '/submodels', '/shell-descriptors', '/submodel-descriptors', '/concept-descriptions', '/lookup/shells')
);
ALTER TABLE rebac_access ADD CONSTRAINT rebac_access_binding_check CHECK (
  num_nonnulls(collection, aas_id, submodel_id, sme_id, aas_descriptor_id, submodel_descriptor_id, concept_description_id, discovery_aas_id) = 1
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_rebac_access_aas_descriptor
  ON rebac_access(scope, aas_descriptor_id) WHERE aas_descriptor_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_rebac_access_submodel_descriptor
  ON rebac_access(scope, submodel_descriptor_id) WHERE submodel_descriptor_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_rebac_access_concept_description
  ON rebac_access(scope, concept_description_id) WHERE concept_description_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_rebac_access_discovery_aas
  ON rebac_access(scope, discovery_aas_id) WHERE discovery_aas_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS rebac_principal (
  access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
  principal_type TEXT NOT NULL DEFAULT 'user' CHECK (principal_type IN ('user', 'group')),
  issuer TEXT NOT NULL CHECK (issuer <> ''),
  subject TEXT NOT NULL CHECK (subject <> ''),
  relation TEXT NOT NULL CHECK (relation IN ('owner', 'manager')),
  PRIMARY KEY(access_id, principal_type, issuer, subject, relation)
);

ALTER TABLE rebac_principal
  ADD COLUMN IF NOT EXISTS principal_type TEXT NOT NULL DEFAULT 'user'
  CHECK (principal_type IN ('user', 'group'));
ALTER TABLE rebac_principal DROP CONSTRAINT IF EXISTS rebac_principal_pkey;
ALTER TABLE rebac_principal
  ADD PRIMARY KEY(access_id, principal_type, issuer, subject, relation);

DROP INDEX IF EXISTS ix_rebac_principal_subject;
CREATE INDEX ix_rebac_principal_subject
  ON rebac_principal(principal_type, issuer, subject, relation, access_id);

CREATE TABLE IF NOT EXISTS rebac_grant (
  id UUID PRIMARY KEY,
  access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
  principal_type TEXT NOT NULL DEFAULT 'user' CHECK (principal_type IN ('user', 'group')),
  issuer TEXT NOT NULL,
  subject TEXT NOT NULL,
  rights JSONB NOT NULL,
  rule JSONB NOT NULL
);

ALTER TABLE rebac_grant
  ADD COLUMN IF NOT EXISTS principal_type TEXT NOT NULL DEFAULT 'user'
  CHECK (principal_type IN ('user', 'group'));
CREATE INDEX IF NOT EXISTS ix_rebac_grant_access ON rebac_grant(access_id);

CREATE TABLE IF NOT EXISTS rebac_policy_version (
  id BIGSERIAL PRIMARY KEY,
  scope TEXT NOT NULL REFERENCES rebac_scope(scope),
  access_id BIGINT,
  revision BIGINT NOT NULL,
  policy JSONB,
  actor_issuer TEXT NOT NULL,
  actor_subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS ix_rebac_version_access
  ON rebac_policy_version(scope, access_id, revision);

CREATE TABLE IF NOT EXISTS rebac_policy_import (
  scope TEXT NOT NULL REFERENCES rebac_scope(scope) ON DELETE CASCADE,
  access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
  PRIMARY KEY (scope, access_id)
);

INSERT INTO rebac_policy_import(scope, access_id)
SELECT scope, id FROM rebac_access
ON CONFLICT DO NOTHING;

CREATE OR REPLACE VIEW rebac_parent AS
SELECT child.id AS child_id, parent.id AS parent_id, NULL::BIGINT AS aas_context, FALSE AS ambiguous
FROM rebac_access child
JOIN submodel_element sme ON sme.id = child.sme_id
JOIN rebac_access parent ON parent.scope = child.scope AND
 ((sme.parent_sme_id IS NOT NULL AND parent.sme_id = sme.parent_sme_id) OR
  (sme.parent_sme_id IS NULL AND parent.submodel_id = sme.submodel_id))
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/shells'
WHERE child.aas_id IS NOT NULL
UNION ALL
SELECT child.id, parent.id, associated.aas_id, associated.parent_count > 1
FROM rebac_access child JOIN submodel sm ON sm.id = child.submodel_id
JOIN (
 SELECT value, aas_id, count(*) OVER (PARTITION BY value) AS parent_count
 FROM (SELECT DISTINCT k.value, r.aas_id FROM aas_submodel_reference r
 JOIN aas_submodel_reference_key k ON k.reference_id = r.id WHERE k.position = 0 AND k.type = 20 AND r.type = 1
 AND NOT EXISTS (SELECT 1 FROM aas_submodel_reference_key extra WHERE extra.reference_id = r.id AND extra.position <> 0)) refs
) associated ON associated.value = sm.submodel_identifier
JOIN rebac_access parent ON parent.scope = child.scope AND parent.aas_id = associated.aas_id
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN submodel sm ON sm.id = child.submodel_id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/submodels'
WHERE NOT EXISTS (SELECT 1 FROM aas_submodel_reference_key k JOIN aas_submodel_reference r ON r.id = k.reference_id
 WHERE k.position = 0 AND k.type = 20 AND r.type = 1 AND k.value = sm.submodel_identifier
 AND NOT EXISTS (SELECT 1 FROM aas_submodel_reference_key extra WHERE extra.reference_id = r.id AND extra.position <> 0))
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN aas_descriptor descriptor ON descriptor.descriptor_id = child.aas_descriptor_id
JOIN aas resource ON resource.aas_id = descriptor.id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.aas_id = resource.id
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN aas_descriptor descriptor ON descriptor.descriptor_id = child.aas_descriptor_id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/shell-descriptors'
WHERE NOT EXISTS (SELECT 1 FROM aas resource WHERE resource.aas_id = descriptor.id)
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN submodel_descriptor descriptor ON descriptor.descriptor_id = child.submodel_descriptor_id
JOIN submodel resource ON resource.submodel_identifier = descriptor.id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.submodel_id = resource.id
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN submodel_descriptor descriptor ON descriptor.descriptor_id = child.submodel_descriptor_id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/submodel-descriptors'
WHERE NOT EXISTS (SELECT 1 FROM submodel resource WHERE resource.submodel_identifier = descriptor.id)
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN aas_identifier discovery ON discovery.id = child.discovery_aas_id
JOIN aas resource ON resource.aas_id = discovery.aasid
JOIN rebac_access parent ON parent.scope = child.scope AND parent.aas_id = resource.id
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN aas_identifier discovery ON discovery.id = child.discovery_aas_id
JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/lookup/shells'
WHERE NOT EXISTS (SELECT 1 FROM aas resource WHERE resource.aas_id = discovery.aasid)
UNION ALL
SELECT child.id, parent.id, NULL::BIGINT, FALSE
FROM rebac_access child JOIN rebac_access parent ON parent.scope = child.scope AND parent.collection = '/concept-descriptions'
WHERE child.concept_description_id IS NOT NULL;

CREATE OR REPLACE FUNCTION rebac_effective_policy(binding BIGINT, operation_aas BIGINT)
RETURNS BIGINT LANGUAGE plpgsql STABLE AS $$
DECLARE
 current_binding BIGINT := binding;
 local_policy BOOLEAN;
 parent_count BIGINT;
 next_binding BIGINT;
BEGIN
 FOR depth IN 1..256 LOOP
  SELECT policy IS NOT NULL INTO local_policy FROM rebac_access WHERE id = current_binding;
  IF NOT FOUND THEN RETURN NULL; END IF;
  IF local_policy THEN RETURN current_binding; END IF;
  SELECT count(*), min(parent_id) INTO parent_count, next_binding
   FROM rebac_parent WHERE child_id = current_binding
   AND ((operation_aas IS NULL AND NOT ambiguous)
     OR (operation_aas IS NOT NULL AND (aas_context IS NULL OR aas_context = operation_aas)));
  IF parent_count <> 1 THEN RETURN NULL; END IF;
  current_binding := next_binding;
 END LOOP;
 RAISE EXCEPTION 'REBAC-EFFECTIVE-DEPTH resource hierarchy exceeds 256 levels';
END;
$$;

UPDATE basyxsystem SET schema_version = 'v1.1.20', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
