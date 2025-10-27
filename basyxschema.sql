/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

-- ------------------------------------------
-- Extensions
-- ------------------------------------------
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;


DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'modelling_kind') THEN
    CREATE TYPE modelling_kind AS ENUM ('Instance', 'Template');
 END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'aas_submodel_elements') THEN
    CREATE TYPE aas_submodel_elements AS ENUM (
      'AnnotatedRelationshipElement','BasicEventElement','Blob','Capability',
      'DataElement','Entity','EventElement','File','MultiLanguageProperty',
      'Operation','Property','Range','ReferenceElement','RelationshipElement',
      'SubmodelElement','SubmodelElementCollection','SubmodelElementList'
    );
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'data_type_def_xsd') THEN
    CREATE TYPE data_type_def_xsd AS ENUM (
      'xs:anyURI','xs:base64Binary','xs:boolean','xs:byte','xs:date','xs:dateTime',
      'xs:decimal','xs:double','xs:duration','xs:float','xs:gDay','xs:gMonth',
      'xs:gMonthDay','xs:gYear','xs:gYearMonth','xs:hexBinary','xs:int','xs:integer',
      'xs:long','xs:negativeInteger','xs:nonNegativeInteger','xs:nonPositiveInteger',
      'xs:positiveInteger','xs:short','xs:string','xs:time','xs:unsignedByte',
      'xs:unsignedInt','xs:unsignedLong','xs:unsignedShort'
    );
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'reference_types') THEN
    CREATE TYPE reference_types AS ENUM ('ExternalReference', 'ModelReference');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'qualifier_kind') THEN
    CREATE TYPE qualifier_kind AS ENUM ('ConceptQualifier','TemplateQualifier','ValueQualifier');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'entity_type') THEN
    CREATE TYPE entity_type AS ENUM ('CoManagedEntity','SelfManagedEntity');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'direction') THEN
    CREATE TYPE direction AS ENUM ('input','output');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'state_of_event') THEN
    CREATE TYPE state_of_event AS ENUM ('off','on');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'operation_var_role') THEN
    CREATE TYPE operation_var_role AS ENUM ('in','out','inout');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'key_type') THEN
    CREATE TYPE key_type AS ENUM ('AnnotatedRelationshipElement','AssetAdministrationShell','BasicEventElement','Blob',
      'Capability','ConceptDescription','DataElement','Entity','EventElement','File','FragmentReference','GlobalReference','Identifiable',
      'MultiLanguageProperty','Operation','Property','Range','Referable','ReferenceElement','RelationshipElement','Submodel','SubmodelElement',
      'SubmodelElementCollection','SubmodelElementList');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'data_type_iec61360') THEN
    CREATE TYPE data_type_iec61360 AS ENUM (
      'Date',
      'String',
      'StringTranslatable',
      'IntegerMeasure',
      'IntegerCount',
      'IntegerCurrency',
      'RealMeasure',
      'RealCount',
      'RealCurrency',
      'Boolean',
      'Iri',
      'Irdi',
      'Rational',
      'RationalMeasure',
      'Time',
      'Timestamp',
      'Html',
      'Blob',
      'File'
    );
  END IF;
END $$;

-- Reference (for semanticId etc.)  --  keys[i] keeps track of order
CREATE TABLE IF NOT EXISTS reference (
  id           BIGSERIAL PRIMARY KEY,
  type         reference_types NOT NULL,
  parentReference BIGINT REFERENCES reference(id),  -- Optional nesting
  rootReference BIGINT REFERENCES reference(id)  -- The root of the nesting tree
);

CREATE INDEX IF NOT EXISTS ix_ref_parentref ON reference(parentReference);
CREATE INDEX IF NOT EXISTS ix_ref_rootref ON reference(rootReference);

CREATE INDEX IF NOT EXISTS ix_ref_id ON reference(id);

CREATE TABLE IF NOT EXISTS reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,                -- <- Array-Index keys[i]
  type         key_type     NOT NULL,
  value        TEXT     NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE INDEX IF NOT EXISTS ix_refkey_type_val     ON reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_refkey_val_trgm     ON reference_key USING GIN (value gin_trgm_ops);


CREATE TABLE IF NOT EXISTS lang_string_text_type_reference(
  id       BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS lang_string_text_type (
  id     BIGSERIAL PRIMARY KEY,
  lang_string_text_type_reference_id BIGINT NOT NULL REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     varchar(1023) NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_lsttr_id ON lang_string_text_type_reference(id);
CREATE INDEX IF NOT EXISTS ix_lstt_refid ON lang_string_text_type(lang_string_text_type_reference_id);



CREATE TABLE IF NOT EXISTS lang_string_name_type_reference(
  id       BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);
CREATE INDEX IF NOT EXISTS ix_lsntr_id ON lang_string_name_type_reference(id);

CREATE TABLE IF NOT EXISTS lang_string_name_type (
  id     BIGSERIAL PRIMARY KEY,
  lang_string_name_type_reference_id BIGINT NOT NULL REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     varchar(128) NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_lsnt_refid ON lang_string_name_type(lang_string_name_type_reference_id);

CREATE TABLE IF NOT EXISTS administrative_information (
  id                BIGSERIAL PRIMARY KEY,
  version           VARCHAR(4),
  revision          VARCHAR(4),
  creator           BIGSERIAL REFERENCES reference(id),
  embedded_data_specification JSONB,
  templateId        VARCHAR(2048)
);

CREATE TABLE IF NOT EXISTS data_specification_content (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE INDEX IF NOT EXISTS ix_edscontent_id ON data_specification_content(id);

CREATE INDEX IF NOT EXISTS ix_admin_id ON administrative_information(id);

CREATE TABLE IF NOT EXISTS data_specification (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
  data_specification BIGINT REFERENCES reference(id) NOT NULL,
  data_specification_content BIGINT REFERENCES data_specification_content(id) NOT NULL
);


CREATE TABLE IF NOT EXISTS value_list (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE INDEX IF NOT EXISTS ix_valuelist_id ON value_list(id);

CREATE TABLE IF NOT EXISTS value_list_value_reference_pair (
  id BIGSERIAL PRIMARY KEY,
  position INTEGER NOT NULL,  -- <- Array-Index valueReferencePairs[i]
  value_list_id BIGINT NOT NULL REFERENCES value_list(id) ON DELETE CASCADE,
  value TEXT NOT NULL,
  value_id BIGINT REFERENCES reference(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_vlvrp_id ON value_list_value_reference_pair(id);
CREATE INDEX IF NOT EXISTS ix_vlvrp_value_id ON value_list_value_reference_pair(value_id);
CREATE INDEX IF NOT EXISTS ix_vlvrp_valuelist_value_id ON value_list_value_reference_pair(value_list_id, value_id);

CREATE TABLE IF NOT EXISTS level_type (
  id BIGSERIAL PRIMARY KEY,
  min BOOLEAN NOT NULL,
  max BOOLEAN NOT NULL,
  nom BOOLEAN NOT NULL,
  typ BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_lt_id ON level_type(id);

CREATE TABLE IF NOT EXISTS data_specification_iec61360 (
  id                BIGINT REFERENCES data_specification_content(id) ON DELETE CASCADE PRIMARY KEY,
  preferred_name_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE NOT NULL,
  short_name_id     BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  unit              TEXT,
  unit_id           BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  source_of_definition TEXT,
  symbol           TEXT,
  data_type        data_type_iec61360,
  definition_id    BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  value_format     TEXT,
  value_list_id    BIGINT REFERENCES value_list(id) ON DELETE CASCADE,
  level_type_id BIGINT REFERENCES level_type(id) ON DELETE CASCADE,
  value VARCHAR(2048)
);

CREATE INDEX IF NOT EXISTS ix_ds_dataspec_content ON data_specification(data_specification_content);
CREATE INDEX IF NOT EXISTS ix_ds_dataspec_reference ON data_specification(data_specification);

CREATE INDEX IF NOT EXISTS ix_iec61360_value_list_id ON data_specification_iec61360(value_list_id);
CREATE INDEX IF NOT EXISTS ix_iec61360_level_type_id ON data_specification_iec61360(level_type_id);
CREATE INDEX IF NOT EXISTS ix_iec61360_data_type ON data_specification_iec61360(data_type);


CREATE TABLE IF NOT EXISTS administrative_information_embedded_data_specification (
  id                BIGSERIAL PRIMARY KEY,
  administrative_information_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  embedded_data_specification_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_ai_creator ON administrative_information(creator);
CREATE INDEX IF NOT EXISTS ix_ai_templateid ON administrative_information(templateid);

CREATE INDEX IF NOT EXISTS ix_aieds_aiid ON administrative_information_embedded_data_specification(administrative_information_id);
CREATE INDEX IF NOT EXISTS ix_aieds_edsid ON administrative_information_embedded_data_specification(embedded_data_specification_id);

CREATE INDEX IF NOT EXISTS ix_eds_id ON administrative_information_embedded_data_specification(id);


CREATE TABLE IF NOT EXISTS submodel (
  id          varchar(2048) PRIMARY KEY,                 -- Identifiable.id
  id_short    varchar(128),
  category    varchar(128),
  kind        modelling_kind,
  administration_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  description_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  displayname_id  BIGINT REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
  embedded_data_specification JSONB,
  model_type  TEXT NOT NULL DEFAULT 'Submodel'
);
CREATE INDEX IF NOT EXISTS ix_sm_idshort ON submodel(id_short);
CREATE INDEX IF NOT EXISTS ix_sm_admin_id ON submodel(administration_id);
CREATE INDEX IF NOT EXISTS ix_sm_semantic_id ON submodel(semantic_id);
CREATE INDEX IF NOT EXISTS ix_sm_desc_id ON submodel(description_id);
CREATE INDEX IF NOT EXISTS ix_sm_displayname_id ON submodel(displayname_id);

CREATE TABLE IF NOT EXISTS submodel_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  submodel_id VARCHAR(2048) NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_smssi_submodel_id ON submodel_supplemental_semantic_id(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smssi_reference_id ON submodel_supplemental_semantic_id(reference_id);



CREATE INDEX IF NOT EXISTS ix_smssi_smid ON submodel_supplemental_semantic_id(submodel_id);

CREATE INDEX IF NOT EXISTS ix_smsup_id ON submodel_supplemental_semantic_id(id);

CREATE TABLE IF NOT EXISTS submodel_embedded_data_specification (
  id                BIGSERIAL PRIMARY KEY,
  submodel_id       VARCHAR(2048) REFERENCES submodel(id) ON DELETE CASCADE,
  embedded_data_specification_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_eds_id ON submodel_embedded_data_specification(id);

CREATE TABLE IF NOT EXISTS extension (
  id          BIGSERIAL PRIMARY KEY,
  semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  name       varchar(128) NOT NULL,
  value_type    data_type_def_xsd,
  value_text    TEXT,
  value_num     NUMERIC,
  value_bool    BOOLEAN,
  value_time    TIME,
  value_datetime TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_ext_id ON extension(id);

CREATE TABLE IF NOT EXISTS submodel_extension (
  id BIGSERIAL PRIMARY KEY,
  submodel_id VARCHAR(2048) NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE 
);
CREATE INDEX IF NOT EXISTS ix_smext_submodel_id ON submodel_extension(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smext_extension_id ON submodel_extension(extension_id);

CREATE INDEX IF NOT EXISTS ix_ext_semantic_id ON extension(semantic_id);

CREATE INDEX IF NOT EXISTS ix_smext_id ON submodel_extension(id);

CREATE TABLE IF NOT EXISTS extension_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
); 


CREATE INDEX IF NOT EXISTS ix_essi_extension_id ON extension_supplemental_semantic_id(extension_id);
CREATE INDEX IF NOT EXISTS ix_essi_reference_id ON extension_supplemental_semantic_id(reference_id);
CREATE INDEX IF NOT EXISTS ix_extsup_id ON extension_supplemental_semantic_id(id);
CREATE INDEX IF NOT EXISTS ix_extsup_eid ON extension_supplemental_semantic_id(extension_id);

CREATE TABLE IF NOT EXISTS extension_refers_to (
  id BIGSERIAL PRIMARY KEY,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_extref_id ON extension_refers_to(id);
CREATE INDEX IF NOT EXISTS ix_extref_eid ON extension_refers_to(extension_id);
CREATE INDEX IF NOT EXISTS ix_extref_reference_id ON extension_refers_to(reference_id);

CREATE TABLE IF NOT EXISTS submodel_semantic_key (
  submodel_id TEXT NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  position    INTEGER NOT NULL,
  key_type    TEXT NOT NULL,
  key_value   TEXT NOT NULL,
  PRIMARY KEY (submodel_id, position)
);
CREATE INDEX IF NOT EXISTS ix_smsem_key     ON submodel_semantic_key(key_type, key_value);
CREATE INDEX IF NOT EXISTS ix_smsem_val_trgm ON submodel_semantic_key USING GIN (key_value gin_trgm_ops);

CREATE TABLE IF NOT EXISTS submodel_element (
  id             BIGSERIAL PRIMARY KEY,
  submodel_id    TEXT NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  parent_sme_id  BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
  position       INTEGER,                                   -- for ordering in lists
  id_short       varchar(128) NOT NULL,
  category       varchar(128),
  model_type     aas_submodel_elements NOT NULL,
  semantic_id    BIGINT REFERENCES reference(id),
  description_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  displayname_id BIGINT REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
  idshort_path   TEXT NOT NULL,                            -- e.g. sm_abc.sensors[2].temperature
  CONSTRAINT uq_sibling_idshort UNIQUE (submodel_id, parent_sme_id, id_short),
  CONSTRAINT uq_sibling_pos     UNIQUE (submodel_id, parent_sme_id, position)
);

CREATE INDEX IF NOT EXISTS ix_sme_path_gin       ON submodel_element USING GIN (idshort_path gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_sme_sub_path       ON submodel_element(submodel_id, idshort_path);
CREATE INDEX IF NOT EXISTS ix_sme_parent_pos     ON submodel_element(parent_sme_id, position);
CREATE INDEX IF NOT EXISTS ix_sme_sub_type       ON submodel_element(submodel_id, model_type);

CREATE TABLE IF NOT EXISTS sme_supplemental_semantic (
  sme_id       BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE,
  PRIMARY KEY (sme_id, reference_id)
);

CREATE TABLE IF NOT EXISTS sme_semantic_key (
  sme_id     BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  key_type   TEXT NOT NULL,
  key_value  TEXT NOT NULL,
  PRIMARY KEY (sme_id, position)
);
CREATE INDEX IF NOT EXISTS ix_smesem_key       ON sme_semantic_key(key_type, key_value);
CREATE INDEX IF NOT EXISTS ix_smesem_val_trgm  ON sme_semantic_key USING GIN (key_value gin_trgm_ops);

-- Property (typed for fast comparisons)
CREATE TABLE IF NOT EXISTS property_element (
  id            BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_type    data_type_def_xsd NOT NULL,
  value_text    TEXT,
  value_num     NUMERIC,
  value_bool    BOOLEAN,
  value_time    TIME,
  value_datetime TIMESTAMPTZ,
  value_id      BIGINT REFERENCES reference(id)
);
-- Partial indexes (small + fast)
CREATE INDEX IF NOT EXISTS ix_prop_num      ON property_element(value_num)
  WHERE value_type IN ('xs:byte','xs:int','xs:integer','xs:long','xs:short',
                       'xs:decimal','xs:double','xs:float','xs:nonNegativeInteger',
                       'xs:nonPositiveInteger','xs:positiveInteger',
                       'xs:unsignedByte','xs:unsignedInt','xs:unsignedLong','xs:unsignedShort');
CREATE INDEX IF NOT EXISTS ix_prop_dt       ON property_element(value_datetime)
  WHERE value_type IN ('xs:dateTime','xs:date');
CREATE INDEX IF NOT EXISTS ix_prop_time     ON property_element(value_time)
  WHERE value_type = 'xs:time';
CREATE INDEX IF NOT EXISTS ix_prop_bool     ON property_element(value_bool)
  WHERE value_type = 'xs:boolean';
CREATE INDEX IF NOT EXISTS ix_prop_text_trgm ON property_element USING GIN (value_text gin_trgm_ops)
  WHERE value_type = 'xs:string';

CREATE TABLE IF NOT EXISTS multilanguage_property (
  id        BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_id  BIGINT REFERENCES reference(id)
);
CREATE TABLE IF NOT EXISTS multilanguage_property_value (
  id     BIGSERIAL PRIMARY KEY,
  mlp_id BIGINT NOT NULL REFERENCES multilanguage_property(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_mlp_lang      ON multilanguage_property_value(mlp_id, language);
CREATE INDEX IF NOT EXISTS ix_mlp_text_trgm ON multilanguage_property_value USING GIN (text gin_trgm_ops);

CREATE TABLE IF NOT EXISTS blob_element (
  id           BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  content_type TEXT,
  value        BYTEA
);

CREATE TABLE IF NOT EXISTS file_element (
  id           BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  content_type TEXT,
  value        TEXT
);
CREATE INDEX IF NOT EXISTS ix_file_value_trgm ON file_element USING GIN (value gin_trgm_ops);

-- Range (also typed)
CREATE TABLE IF NOT EXISTS range_element (
  id            BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_type    data_type_def_xsd NOT NULL,
  min_text      TEXT,  max_text      TEXT,
  min_num       NUMERIC, max_num     NUMERIC,
  min_time      TIME,   max_time     TIME,
  min_datetime  TIMESTAMPTZ, max_datetime TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS ix_range_num ON range_element(min_num, max_num)
  WHERE value_type IN ('xs:byte','xs:int','xs:integer','xs:long','xs:short',
                       'xs:decimal','xs:double','xs:float','xs:nonNegativeInteger',
                       'xs:nonPositiveInteger','xs:positiveInteger',
                       'xs:unsignedByte','xs:unsignedInt','xs:unsignedLong','xs:unsignedShort');
CREATE INDEX IF NOT EXISTS ix_range_dt  ON range_element(min_datetime, max_datetime)
  WHERE value_type IN ('xs:dateTime','xs:date');
CREATE INDEX IF NOT EXISTS ix_range_time ON range_element(min_time, max_time)
  WHERE value_type = 'xs:time';

CREATE TABLE IF NOT EXISTS reference_element (
  id        BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_ref BIGINT REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS relationship_element (
  id         BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  first_ref  BIGINT REFERENCES reference(id),
  second_ref BIGINT REFERENCES reference(id)
);
CREATE TABLE IF NOT EXISTS annotated_rel_annotation (
  rel_id      BIGINT NOT NULL REFERENCES relationship_element(id) ON DELETE CASCADE,
  annotation_sme BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  PRIMARY KEY (rel_id, annotation_sme)
);

CREATE TABLE IF NOT EXISTS submodel_element_collection (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS submodel_element_list (
  id                         BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  order_relevant             BOOLEAN,
  semantic_id_list_element   BIGINT REFERENCES reference(id),
  type_value_list_element    aas_submodel_elements NOT NULL,
  value_type_list_element    data_type_def_xsd
);

CREATE TABLE IF NOT EXISTS entity_element (
  id              BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  entity_type     entity_type NOT NULL,
  global_asset_id TEXT
);
CREATE TABLE IF NOT EXISTS entity_specific_asset_id (
  id                   BIGSERIAL PRIMARY KEY,
  entity_id            BIGINT NOT NULL REFERENCES entity_element(id) ON DELETE CASCADE,
  name                 TEXT NOT NULL,
  value                TEXT NOT NULL,
  external_subject_ref BIGINT REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS operation_element (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS operation_variable (
  id           BIGSERIAL PRIMARY KEY,
  operation_id BIGINT NOT NULL REFERENCES operation_element(id) ON DELETE CASCADE,
  role         operation_var_role NOT NULL,
  position     INTEGER NOT NULL,
  value_sme    BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  UNIQUE (operation_id, role, position)
);

CREATE TABLE IF NOT EXISTS basic_event_element (
  id                BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  observed_ref      BIGINT REFERENCES reference(id),
  direction         direction NOT NULL,
  state             state_of_event NOT NULL,
  message_topic     TEXT,
  message_broker_ref BIGINT REFERENCES reference(id),
  last_update       TIMESTAMPTZ,
  min_interval      INTERVAL,
  max_interval      INTERVAL
);
CREATE INDEX IF NOT EXISTS ix_bee_lastupd ON basic_event_element(last_update);

CREATE TABLE IF NOT EXISTS capability_element (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);

-- Qualifier (on any SME)
CREATE TABLE IF NOT EXISTS qualifier (
  id                BIGSERIAL PRIMARY KEY,
  kind              qualifier_kind,
  type              TEXT NOT NULL,
  value_type        data_type_def_xsd NOT NULL,
  value_text        TEXT,
  value_num         NUMERIC,
  value_bool        BOOLEAN,
  value_time        TIME,
  value_datetime    TIMESTAMPTZ,
  value_id          BIGINT REFERENCES reference(id),
  semantic_id       BIGINT REFERENCES reference(id)
);

CREATE INDEX IF NOT EXISTS ix_qual_semantic_id ON qualifier(semantic_id);
CREATE INDEX IF NOT EXISTS ix_qual_value_id ON qualifier(value_id);

CREATE TABLE IF NOT EXISTS submodel_element_qualifier (
  sme_id      BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  qualifier_id BIGINT NOT NULL REFERENCES qualifier(id) ON DELETE CASCADE,
  PRIMARY KEY (sme_id, qualifier_id)
);

CREATE TABLE IF NOT EXISTS submodel_qualifier (
  submodel_id  VARCHAR(2048) NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  qualifier_id BIGINT NOT NULL REFERENCES qualifier(id) ON DELETE CASCADE,
  PRIMARY KEY (submodel_id, qualifier_id)
);

CREATE INDEX IF NOT EXISTS ix_smq_submodel_id ON submodel_qualifier(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smq_qualifier_id ON submodel_qualifier(qualifier_id);
CREATE INDEX IF NOT EXISTS ix_subm_qual      ON submodel_qualifier(submodel_id);

CREATE INDEX IF NOT EXISTS ix_qual_sme       ON submodel_element_qualifier(sme_id);

CREATE INDEX IF NOT EXISTS ix_qual_type      ON qualifier(type);
CREATE INDEX IF NOT EXISTS ix_qual_num       ON qualifier(value_num)
  WHERE value_type IN ('xs:decimal','xs:double','xs:float','xs:int','xs:integer','xs:long','xs:short');
CREATE INDEX IF NOT EXISTS ix_qual_text_trgm ON qualifier USING GIN (value_text gin_trgm_ops)
  WHERE value_type = 'xs:string';

ALTER TABLE submodel_element
  ADD COLUMN IF NOT EXISTS depth INTEGER;

CREATE INDEX IF NOT EXISTS ix_sme_sub_parent  ON submodel_element (submodel_id, parent_sme_id);
CREATE INDEX IF NOT EXISTS ix_sme_sub_depth   ON submodel_element (submodel_id, depth);
CREATE INDEX IF NOT EXISTS ix_sme_roots_order
  ON submodel_element (submodel_id,
                       (CASE WHEN position IS NULL THEN 1 ELSE 0 END),  -- NULLS LAST
                       position,
                       idshort_path,
                       id)
  WHERE parent_sme_id IS NULL;

CREATE TABLE IF NOT EXISTS qualifier_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  qualifier_id BIGINT NOT NULL REFERENCES qualifier(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_qssi_qualifier_id ON qualifier_supplemental_semantic_id(qualifier_id);
CREATE INDEX IF NOT EXISTS ix_qssi_reference_id ON qualifier_supplemental_semantic_id(reference_id);
CREATE INDEX IF NOT EXISTS ix_qualsup_id ON qualifier_supplemental_semantic_id(id);
CREATE INDEX IF NOT EXISTS ix_qualsup_qid ON qualifier_supplemental_semantic_id(qualifier_id);


CREATE INDEX IF NOT EXISTS ix_seds_submodel ON submodel_embedded_data_specification (submodel_id);

CREATE INDEX IF NOT EXISTS ix_dataspec_content ON data_specification (data_specification_content);

CREATE INDEX IF NOT EXISTS ix_iec61360_preferred_name ON data_specification_iec61360 (preferred_name_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_short_name ON data_specification_iec61360 (short_name_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_definition ON data_specification_iec61360 (definition_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_unit_id ON data_specification_iec61360 (unit_id);

CREATE INDEX IF NOT EXISTS ix_vlvrp_valuelist ON value_list_value_reference_pair (value_list_id);

CREATE INDEX IF NOT EXISTS ix_dsiec_id ON data_specification_iec61360(id);

CREATE INDEX IF NOT EXISTS ix_ref_root_id ON reference(rootreference, id);
CREATE INDEX IF NOT EXISTS ix_ref_type ON reference(type);