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

-- ------------------------------------------
-- Extensions
-- ------------------------------------------
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;


-- ------------------------------------------
-- Enums
-- ------------------------------------------
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'security_type') THEN
    CREATE TYPE security_type AS ENUM ('NONE', 'RFC_TLSA', 'W3C_DID');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS level_type (
  id BIGSERIAL PRIMARY KEY,
  min BOOLEAN NOT NULL,
  max BOOLEAN NOT NULL,
  nom BOOLEAN NOT NULL,
  typ BOOLEAN NOT NULL
);

-- ------------------------------------------
-- AAS REPO
-- ------------------------------------------
CREATE TABLE IF NOT EXISTS aas (
  aas_id BIGSERIAL PRIMARY KEY,
  id varchar(2048) UNIQUE NOT NULL,
  id_short varchar(128),
  category varchar(128),
  model_type int NOT NULL DEFAULT 3
  -- submodel missing
); -- asset_information in asset_information

CREATE TABLE IF NOT EXISTS aas_payload ( 
  aas_payload_id BIGINT PRIMARY KEY REFERENCES aas(aas_id) ON DELETE CASCADE,
  aas_administrative_information_payload JSONB DEFAULT '[]',
  aas_display_name_payload JSONB DEFAULT '[]', 
  aas_description_payload JSONB DEFAULT '[]',
  aas_extension_payload JSONB DEFAULT '[]',
  aas_embedded_data_specification_payload JSONB DEFAULT '[]',
  aas_derived_from JSONB DEFAULT '[]'
); 

CREATE TABLE IF NOT EXISTS asset_information ( 
  asset_information_id BIGINT PRIMARY KEY REFERENCES aas(aas_id) ON DELETE CASCADE,
  asset_kind int, 
  global_asset_id varchar(2048),
  asset_type varchar(2048),
  default_thumbnail JSONB DEFAULT '[]',
  model_type int NOT NULL DEFAULT 4 
); -- specific_asset_id in specific_asset_id
-- 
-- ------------------------------------------

CREATE TABLE IF NOT EXISTS submodel (
  id          BIGSERIAL PRIMARY KEY, 
  submodel_identifier varchar(2048) UNIQUE NOT NULL,              -- Identifiable.id
  id_short    varchar(128),
  category    varchar(128),
  kind        int
);

CREATE TABLE IF NOT EXISTS submodel_payload (
  submodel_id BIGINT PRIMARY KEY REFERENCES submodel(id) ON DELETE CASCADE,
  description_payload JSONB,
  displayname_payload JSONB,
  administrative_information_payload JSONB,
  embedded_data_specification_payload JSONB,
  supplemental_semantic_ids_payload JSONB,
  extensions_payload JSONB,
  qualifiers_payload JSONB
);

CREATE TABLE IF NOT EXISTS submodel_element (
  id             BIGSERIAL PRIMARY KEY,
  submodel_id    BIGINT NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  root_sme_id    BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
  parent_sme_id  BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
  position       INTEGER,                                   -- for ordering in lists
  id_short       varchar(128),
  category       varchar(128),
  model_type     int NOT NULL,
  idshort_path   TEXT NOT NULL,                            -- e.g. sm_abc.sensors[2].temperature
  depth	BIGINT,
  CONSTRAINT uq_sibling_idshort UNIQUE (submodel_id, parent_sme_id, idshort_path),
  CONSTRAINT uq_sibling_pos     UNIQUE (submodel_id, parent_sme_id, position)
);
CREATE TABLE IF NOT EXISTS submodel_element_payload (
  submodel_element_id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  description_payload JSONB DEFAULT '[]',
  displayname_payload JSONB DEFAULT '[]',
  administrative_information_payload JSONB DEFAULT '[]',
  embedded_data_specification_payload JSONB DEFAULT '[]',
  supplemental_semantic_ids_payload JSONB DEFAULT '[]',
  extensions_payload JSONB DEFAULT '[]',
  qualifiers_payload JSONB
);

CREATE TABLE IF NOT EXISTS property_element (
  id            BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_type    int NOT NULL,
  value_text    TEXT,
  value_num     NUMERIC,
  value_bool    BOOLEAN,
  value_time    TIME,
  value_date    DATE,
  value_datetime TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS property_element_payload (
  property_element_id BIGINT PRIMARY KEY REFERENCES property_element(id) ON DELETE CASCADE,
  value_id_payload JSONB DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS multilanguage_property (
  id        BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_id_payload JSONB DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS multilanguage_property_value (
  id     BIGSERIAL PRIMARY KEY,
  mlp_id BIGINT NOT NULL REFERENCES multilanguage_property(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS blob_element (
  id           BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  content_type TEXT,
  value        BYTEA
);

CREATE TABLE IF NOT EXISTS file_element (
  id           BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  content_type TEXT,
  file_name    TEXT,
  value        TEXT
);
CREATE TABLE IF NOT EXISTS file_data (
  id BIGINT PRIMARY KEY REFERENCES file_element(id) ON DELETE CASCADE,
  file_oid oid
);
CREATE TABLE IF NOT EXISTS range_element (
  id            BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value_type    int NOT NULL,
  min_text      TEXT,  max_text      TEXT,
  min_num       NUMERIC, max_num     NUMERIC,
  min_time      TIME,   max_time     TIME,
  min_date      DATE,   max_date     DATE,
  min_datetime  TIMESTAMPTZ, max_datetime TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS reference_element (
  id        BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  value JSONB
);
CREATE TABLE IF NOT EXISTS relationship_element (
  id         BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  first JSONB,
  second JSONB
);
CREATE TABLE IF NOT EXISTS annotated_relationship_element (
  id         BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  first JSONB,
  second JSONB
);
CREATE TABLE IF NOT EXISTS submodel_element_collection (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS submodel_element_list (
  id                         BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  order_relevant             BOOLEAN,
  semantic_id_list_element   JSONB,
  type_value_list_element    int NOT NULL,
  value_type_list_element    int
);
CREATE TABLE IF NOT EXISTS entity_element (
  id              BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  entity_type     int NOT NULL,
  global_asset_id TEXT,
  specific_asset_ids JSONB DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS operation_element (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  input_variables JSONB DEFAULT '[]',
  output_variables JSONB DEFAULT '[]',
  inoutput_variables JSONB DEFAULT '[]'
);
CREATE TABLE IF NOT EXISTS operation_variable (
  id           BIGSERIAL PRIMARY KEY,
  operation_id BIGINT NOT NULL REFERENCES operation_element(id) ON DELETE CASCADE,
  role         int NOT NULL,
  position     INTEGER NOT NULL,
  value_sme    BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  UNIQUE (operation_id, role, position)
);
CREATE TABLE IF NOT EXISTS basic_event_element (
  id                BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  observed          JSONB,
  direction         int NOT NULL,
  state             int NOT NULL,
  message_topic     TEXT,
  message_broker    JSONB,
  last_update       TIMESTAMPTZ,
  min_interval      INTERVAL,
  max_interval      INTERVAL
);
CREATE TABLE IF NOT EXISTS capability_element (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS qualifier (
  id                BIGSERIAL PRIMARY KEY,
  position          INTEGER NOT NULL,
  kind              int,
  type              TEXT NOT NULL,
  value_type        int NOT NULL,
  value_text        TEXT,
  value_num         NUMERIC,
  value_bool        BOOLEAN,
  value_time        TIME,
  value_date        DATE,
  value_datetime    TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS qualifier_payload (
  qualifier_id BIGINT PRIMARY KEY REFERENCES qualifier(id) ON DELETE CASCADE,
  value_id_payload JSONB DEFAULT '[]',
  semantic_id_payload JSONB DEFAULT '[]',
  supplemental_semantic_ids_payload JSONB DEFAULT '[]'
);


CREATE TABLE IF NOT EXISTS submodel_element_qualifier (
  sme_id      BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  qualifier_id BIGINT NOT NULL REFERENCES qualifier(id) ON DELETE CASCADE,
  PRIMARY KEY (sme_id, qualifier_id)
);
CREATE TABLE IF NOT EXISTS submodel_qualifier (
  id BIGSERIAL PRIMARY KEY,
  submodel_id  BIGINT NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  qualifier_id BIGINT NOT NULL REFERENCES qualifier(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS descriptor (
  id BIGSERIAL PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS aas_identifier (
  id          BIGSERIAL PRIMARY KEY,
  aasId       VARCHAR(2048) UNIQUE NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS specific_asset_id (
  id BIGSERIAL PRIMARY KEY,
  position     INTEGER NOT NULL,                -- <- Array-Index
  descriptor_id BIGINT REFERENCES descriptor(id) ON DELETE CASCADE,
  aasRef BIGINT REFERENCES aas_identifier(id) ON DELETE CASCADE,
  aas BIGINT REFERENCES aas(aas_id) ON DELETE CASCADE, 
  name VARCHAR(64) NOT NULL,
  value VARCHAR(2048) NOT NULL
);

CREATE TABLE IF NOT EXISTS specific_asset_id_payload (
  specific_asset_id BIGINT PRIMARY KEY REFERENCES specific_asset_id(id) ON DELETE CASCADE,
  semantic_id_payload JSONB DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS aas_descriptor_endpoint (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,                -- <- Array-Index
  href VARCHAR(2048) NOT NULL,
  endpoint_protocol VARCHAR(128),
  endpoint_protocol_version JSONB DEFAULT '[]',
  sub_protocol VARCHAR(128),
  sub_protocol_body VARCHAR(2048),
  sub_protocol_body_encoding VARCHAR(128),
  security_attributes JSONB DEFAULT '[]',
  interface VARCHAR(128) NOT NULL
);


CREATE TABLE IF NOT EXISTS aas_descriptor (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  asset_kind int,
  asset_type VARCHAR(2048),
  global_asset_id VARCHAR(2048),
  id_short VARCHAR(128),
  id VARCHAR(2048) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS submodel_descriptor (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  position     INTEGER NOT NULL,                -- <- Array-Index
  aas_descriptor_id BIGINT REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
  id_short VARCHAR(128),
  id VARCHAR(2048) NOT NULL -- not unique because it can have duplicates over different aas descriptor.
);

CREATE TABLE IF NOT EXISTS descriptor_payload (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  description_payload JSONB NOT NULL,
  displayname_payload JSONB NOT NULL,
  administrative_information_payload JSONB NOT NULL,
  extensions_payload JSONB DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS infrastructure_descriptor (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  global_asset_id VARCHAR(2048),
  id_short VARCHAR(128),
  id VARCHAR(2048) NOT NULL UNIQUE,
  company VARCHAR(2048)
);

CREATE TABLE IF NOT EXISTS concept_description (
  id TEXT PRIMARY KEY,
  id_short TEXT,
  data JSONB
);

/*
 Auto-generated file. Do not edit manually.
 Naming pattern: <context>_reference and <context>_reference_key.
*/

-- =========================================================
-- 1) submodel_semantic_id -> submodel.id
-- =========================================================
CREATE TABLE IF NOT EXISTS submodel_semantic_id_reference (
  id   BIGINT PRIMARY KEY REFERENCES submodel(id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS submodel_semantic_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_semantic_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_semantic_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- =========================================================
-- 2) submodel_element_semantic_id -> submodel_element.id
-- =========================================================
CREATE TABLE IF NOT EXISTS submodel_element_semantic_id_reference (
  id   BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS submodel_element_semantic_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_element_semantic_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_element_semantic_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_element_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- =========================================================
-- 3) submodel_descriptor_semantic_id -> submodel_descriptor.descriptor_id
-- =========================================================
CREATE TABLE IF NOT EXISTS submodel_descriptor_semantic_id_reference (
  id   BIGINT PRIMARY KEY REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_semantic_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_descriptor_semantic_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_semantic_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_descriptor_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- =========================================================
-- 4) specific_asset_id_external_subject_id -> specific_asset_id.id
-- =========================================================
CREATE TABLE IF NOT EXISTS specific_asset_id_external_subject_id_reference (
  id   BIGINT PRIMARY KEY REFERENCES specific_asset_id(id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS specific_asset_id_external_subject_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES specific_asset_id_external_subject_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS specific_asset_id_external_subject_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES specific_asset_id_external_subject_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- =========================================================
-- 5) specific_asset_id_supplemental_semantic_id -> specific_asset_id.id
-- =========================================================
CREATE TABLE IF NOT EXISTS specific_asset_id_supplemental_semantic_id_reference (
  id BIGSERIAL PRIMARY KEY,
  specific_asset_id_id BIGINT NOT NULL REFERENCES specific_asset_id(id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS specific_asset_id_supplemental_semantic_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES specific_asset_id_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS specific_asset_id_supplemental_semantic_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES specific_asset_id_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- =========================================================
-- 6) submodel_descriptor_supplemental_semantic_id -> submodel_descriptor.descriptor_id
-- =========================================================

CREATE TABLE IF NOT EXISTS submodel_descriptor_supplemental_semantic_id_reference (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  type int NOT NULL
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_supplemental_semantic_id_reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_descriptor_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,
  type         int NOT NULL,
  value        TEXT NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_supplemental_semantic_id_reference_payload (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES submodel_descriptor_supplemental_semantic_id_reference(id) ON DELETE CASCADE,
  parent_reference_payload JSONB NOT NULL
);

-- ------------------------------------------
-- Indexes
-- ------------------------------------------

CREATE INDEX IF NOT EXISTS ix_sm_identifier  ON submodel(submodel_identifier);
CREATE INDEX IF NOT EXISTS ix_sm_idshort     ON submodel(id_short);

CREATE INDEX IF NOT EXISTS ix_sme_path_gin   ON submodel_element USING GIN (idshort_path gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_sme_sub_path   ON submodel_element(submodel_id, idshort_path);
CREATE INDEX IF NOT EXISTS ix_sme_parent_pos ON submodel_element(parent_sme_id, position);
CREATE INDEX IF NOT EXISTS ix_sme_sub_type   ON submodel_element(submodel_id, model_type);
CREATE INDEX IF NOT EXISTS ix_sme_sub_parent ON submodel_element(submodel_id, parent_sme_id);
CREATE INDEX IF NOT EXISTS ix_sme_sub_root   ON submodel_element(submodel_id, root_sme_id);
CREATE INDEX IF NOT EXISTS ix_sme_sub_depth  ON submodel_element(submodel_id, depth);
CREATE INDEX IF NOT EXISTS ix_sme_roots_order
  ON submodel_element (submodel_id,
                       (CASE WHEN position IS NULL THEN 1 ELSE 0 END),
                       position,
                       idshort_path,
                       id)
  WHERE parent_sme_id IS NULL;
CREATE INDEX IF NOT EXISTS ix_sme_roots_page
  ON submodel_element(submodel_id, idshort_path, id)
  WHERE parent_sme_id IS NULL;

CREATE INDEX IF NOT EXISTS ix_mlp_lang      ON multilanguage_property_value(mlp_id, language);
CREATE INDEX IF NOT EXISTS ix_mlp_text_trgm ON multilanguage_property_value USING GIN (text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_file_value_trgm ON file_element USING GIN (value gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_bee_lastupd ON basic_event_element(last_update);

CREATE INDEX IF NOT EXISTS ix_qual_type     ON qualifier(type);
CREATE INDEX IF NOT EXISTS ix_qual_position ON qualifier(position);
CREATE INDEX IF NOT EXISTS ix_smeq_qualifier_id ON submodel_element_qualifier(qualifier_id);
CREATE INDEX IF NOT EXISTS ix_smq_submodel_id   ON submodel_qualifier(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smq_qualifier_id  ON submodel_qualifier(qualifier_id);

CREATE UNIQUE INDEX IF NOT EXISTS ix_aas_identifier_aasid ON aas_identifier(aasId);
CREATE INDEX IF NOT EXISTS ix_aas_identifier_created_at ON aas_identifier(created_at);

CREATE INDEX IF NOT EXISTS ix_specasset_descriptor_id_name ON specific_asset_id(descriptor_id, name);
CREATE INDEX IF NOT EXISTS ix_specasset_descriptor_id_position ON specific_asset_id(descriptor_id, position);
CREATE INDEX IF NOT EXISTS ix_specasset_aasref ON specific_asset_id(aasRef);
CREATE INDEX IF NOT EXISTS ix_specasset_aas ON specific_asset_id(aas);
CREATE INDEX IF NOT EXISTS ix_specasset_name_value_aasref ON specific_asset_id(name, value, aasRef);
CREATE INDEX IF NOT EXISTS ix_specasset_descriptor_id ON specific_asset_id(descriptor_id);
CREATE INDEX IF NOT EXISTS ix_specasset_name ON specific_asset_id(name);
CREATE INDEX IF NOT EXISTS ix_specasset_name_value ON specific_asset_id(name, value);
CREATE INDEX IF NOT EXISTS ix_specasset_value_trgm ON specific_asset_id USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_aas_endpoint_descriptor_id ON aas_descriptor_endpoint(descriptor_id);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_interface ON aas_descriptor_endpoint(interface);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_protocols ON aas_descriptor_endpoint(endpoint_protocol, sub_protocol);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_href ON aas_descriptor_endpoint(href);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_href_trgm ON aas_descriptor_endpoint USING GIN (href gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_sp_body_trgm ON aas_descriptor_endpoint USING GIN (sub_protocol_body gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_descriptor_position ON aas_descriptor_endpoint(descriptor_id, position);
CREATE INDEX IF NOT EXISTS ix_aas_endpoint_position ON aas_descriptor_endpoint(position);

CREATE INDEX IF NOT EXISTS ix_aasd_created_at ON aas_descriptor(created_at);
CREATE INDEX IF NOT EXISTS ix_aasd_id_short ON aas_descriptor(id_short);
CREATE INDEX IF NOT EXISTS ix_aasd_global_asset_id ON aas_descriptor(global_asset_id);
CREATE INDEX IF NOT EXISTS ix_aasd_id_trgm ON aas_descriptor USING GIN (id gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_aasd_global_asset_id_trgm ON aas_descriptor USING GIN (global_asset_id gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_aasd_asset_kind ON aas_descriptor(asset_kind);
CREATE INDEX IF NOT EXISTS ix_aasd_asset_type ON aas_descriptor(asset_type);
CREATE INDEX IF NOT EXISTS ix_aasd_asset_kind_type ON aas_descriptor(asset_kind, asset_type);

CREATE INDEX IF NOT EXISTS ix_smd_aas_descriptor_id ON submodel_descriptor(aas_descriptor_id);
CREATE INDEX IF NOT EXISTS ix_smd_created_at ON submodel_descriptor(created_at);
CREATE INDEX IF NOT EXISTS ix_smd_id_short ON submodel_descriptor(id_short);
CREATE INDEX IF NOT EXISTS ix_smd_id_trgm ON submodel_descriptor USING GIN (id gin_trgm_ops);
CREATE UNIQUE INDEX IF NOT EXISTS ux_smd_id_null_aas ON submodel_descriptor(id) WHERE aas_descriptor_id IS NULL;
CREATE INDEX IF NOT EXISTS ix_smd_aas_descriptor_position ON submodel_descriptor(aas_descriptor_id, position);
CREATE INDEX IF NOT EXISTS ix_smd_position ON submodel_descriptor(position);

CREATE INDEX IF NOT EXISTS ix_regd_id_short ON infrastructure_descriptor(id_short);
CREATE INDEX IF NOT EXISTS ix_regd_global_asset_id ON infrastructure_descriptor(global_asset_id);
CREATE INDEX IF NOT EXISTS ix_regd_company ON infrastructure_descriptor(company);
CREATE INDEX IF NOT EXISTS ix_regd_id_trgm ON infrastructure_descriptor USING GIN (id gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_regd_global_asset_id_trgm ON infrastructure_descriptor USING GIN (global_asset_id gin_trgm_ops);

-- Context-specific reference indexes
CREATE INDEX IF NOT EXISTS ix_submodel_semantic_id_ref_type ON submodel_semantic_id_reference(type);
CREATE INDEX IF NOT EXISTS ix_submodel_semantic_id_refkey_refid ON submodel_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_submodel_semantic_id_refkey_refval ON submodel_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_submodel_semantic_id_refkey_type_val ON submodel_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_submodel_semantic_id_refkey_val_trgm ON submodel_semantic_id_reference_key USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_ref_type ON submodel_element_semantic_id_reference(type);
CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_refkey_refid ON submodel_element_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_refkey_refval ON submodel_element_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_refkey_type_val ON submodel_element_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_refkey_val_trgm ON submodel_element_semantic_id_reference_key USING GIN (value gin_trgm_ops);
CREATE INDEX IF NOT EXISTS ix_submodel_element_semantic_id_refpayload_refid ON submodel_element_semantic_id_reference_payload(reference_id);

CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_ref_type ON submodel_descriptor_semantic_id_reference(type);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_refkey_refid ON submodel_descriptor_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_refkey_refval ON submodel_descriptor_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_refkey_type_val ON submodel_descriptor_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_submodel_descriptor_semantic_id_refkey_val_trgm ON submodel_descriptor_semantic_id_reference_key USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_owner_id ON submodel_descriptor_supplemental_semantic_id_reference(descriptor_id);
CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_refkey_refid ON submodel_descriptor_supplemental_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_refkey_refval ON submodel_descriptor_supplemental_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_refkey_type_val ON submodel_descriptor_supplemental_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_smdesc_supp_sem_refkey_val_trgm ON submodel_descriptor_supplemental_semantic_id_reference_key USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_specasset_external_subject_id_ref_type ON specific_asset_id_external_subject_id_reference(type);
CREATE INDEX IF NOT EXISTS ix_specasset_external_subject_id_refkey_refid ON specific_asset_id_external_subject_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_specasset_external_subject_id_refkey_refval ON specific_asset_id_external_subject_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_specasset_external_subject_id_refkey_type_val ON specific_asset_id_external_subject_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_specasset_external_subject_id_refkey_val_trgm ON specific_asset_id_external_subject_id_reference_key USING GIN (value gin_trgm_ops);

CREATE INDEX IF NOT EXISTS ix_specasset_supp_semantic_owner_id ON specific_asset_id_supplemental_semantic_id_reference(specific_asset_id_id);
CREATE INDEX IF NOT EXISTS ix_specasset_supp_semantic_refkey_refid ON specific_asset_id_supplemental_semantic_id_reference_key(reference_id);
CREATE INDEX IF NOT EXISTS ix_specasset_supp_semantic_refkey_refval ON specific_asset_id_supplemental_semantic_id_reference_key(reference_id, value);
CREATE INDEX IF NOT EXISTS ix_specasset_supp_semantic_refkey_type_val ON specific_asset_id_supplemental_semantic_id_reference_key(type, value);
CREATE INDEX IF NOT EXISTS ix_specasset_supp_semantic_refkey_val_trgm ON specific_asset_id_supplemental_semantic_id_reference_key USING GIN (value gin_trgm_ops);
