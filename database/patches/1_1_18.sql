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


CREATE TABLE rebac_scope (
 scope TEXT PRIMARY KEY,
 revision BIGINT NOT NULL DEFAULT 1
);
CREATE TABLE rebac_access (
 id BIGSERIAL PRIMARY KEY,
 scope TEXT NOT NULL REFERENCES rebac_scope(scope),
 collection TEXT CHECK (collection IN ('/shells', '/submodels')),
 aas_id BIGINT REFERENCES aas(id) ON DELETE CASCADE,
 submodel_id BIGINT REFERENCES submodel(id) ON DELETE CASCADE,
 sme_id BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
 revision BIGINT NOT NULL DEFAULT 1,
 policy JSONB,
 CHECK (num_nonnulls(collection, aas_id, submodel_id, sme_id) = 1),
 UNIQUE(scope, collection), UNIQUE(scope, aas_id), UNIQUE(scope, submodel_id), UNIQUE(scope, sme_id)
);
CREATE TABLE rebac_principal (
 access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
 issuer TEXT NOT NULL CHECK (issuer <> ''),
 subject TEXT NOT NULL CHECK (subject <> ''),
 relation TEXT NOT NULL CHECK (relation IN ('owner', 'manager')),
 PRIMARY KEY(access_id, issuer, subject, relation)
);
CREATE INDEX ix_rebac_principal_subject ON rebac_principal(issuer, subject, relation, access_id);
CREATE TABLE rebac_grant (
 id UUID PRIMARY KEY,
 access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
 issuer TEXT NOT NULL,
 subject TEXT NOT NULL,
 rights JSONB NOT NULL,
 rule JSONB NOT NULL
);
CREATE INDEX ix_rebac_grant_access ON rebac_grant(access_id);
CREATE TABLE rebac_policy_version (
 id BIGSERIAL PRIMARY KEY,
 scope TEXT NOT NULL REFERENCES rebac_scope(scope),
 access_id BIGINT,
 revision BIGINT NOT NULL,
 policy JSONB,
 actor_issuer TEXT NOT NULL,
 actor_subject TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX ix_rebac_version_access ON rebac_policy_version(scope, access_id, revision);
CREATE VIEW rebac_parent AS
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
 AND NOT EXISTS (SELECT 1 FROM aas_submodel_reference_key extra WHERE extra.reference_id = r.id AND extra.position <> 0));

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

UPDATE basyxsystem SET schema_version = 'v1.1.18', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
