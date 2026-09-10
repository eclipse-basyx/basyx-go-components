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

ALTER TABLE rebac_access DROP CONSTRAINT rebac_access_collection_check;
ALTER TABLE rebac_access DROP CONSTRAINT rebac_access_check;

ALTER TABLE rebac_access
  ADD COLUMN aas_descriptor_id BIGINT REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
  ADD COLUMN submodel_descriptor_id BIGINT REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  ADD COLUMN concept_description_id TEXT REFERENCES concept_description(id) ON DELETE CASCADE,
  ADD COLUMN discovery_aas_id BIGINT REFERENCES aas_identifier(id) ON DELETE CASCADE;

ALTER TABLE rebac_access ADD CONSTRAINT rebac_access_collection_check CHECK (
  collection IN ('/shells', '/submodels', '/shell-descriptors', '/submodel-descriptors', '/concept-descriptions', '/lookup/shells')
);
ALTER TABLE rebac_access ADD CONSTRAINT rebac_access_binding_check CHECK (
  num_nonnulls(collection, aas_id, submodel_id, sme_id, aas_descriptor_id, submodel_descriptor_id, concept_description_id, discovery_aas_id) = 1
);

CREATE UNIQUE INDEX ux_rebac_access_aas_descriptor ON rebac_access(scope, aas_descriptor_id) WHERE aas_descriptor_id IS NOT NULL;
CREATE UNIQUE INDEX ux_rebac_access_submodel_descriptor ON rebac_access(scope, submodel_descriptor_id) WHERE submodel_descriptor_id IS NOT NULL;
CREATE UNIQUE INDEX ux_rebac_access_concept_description ON rebac_access(scope, concept_description_id) WHERE concept_description_id IS NOT NULL;
CREATE UNIQUE INDEX ux_rebac_access_discovery_aas ON rebac_access(scope, discovery_aas_id) WHERE discovery_aas_id IS NOT NULL;

CREATE TABLE rebac_policy_import (
  scope TEXT NOT NULL REFERENCES rebac_scope(scope) ON DELETE CASCADE,
  access_id BIGINT NOT NULL REFERENCES rebac_access(id) ON DELETE CASCADE,
  PRIMARY KEY (scope, access_id)
);

INSERT INTO rebac_policy_import(scope, access_id)
SELECT scope, id FROM rebac_access;

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

UPDATE basyxsystem SET schema_version = 'v1.1.19', state = 'clean'
WHERE identifier = (SELECT identifier FROM basyxsystem ORDER BY identifier ASC LIMIT 1);
