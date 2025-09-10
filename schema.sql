-- ------------------------------------------
-- Extensions
-- ------------------------------------------
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ------------------------------------------
-- Enums
-- ------------------------------------------
CREATE TYPE modelling_kind AS ENUM ('Instance', 'Template');
CREATE TYPE aas_submodel_elements AS ENUM (
  'AnnotatedRelationshipElement','BasicEventElement','Blob','Capability',
  'DataElement','Entity','EventElement','File','MultiLanguageProperty',
  'Operation','Property','Range','ReferenceElement','RelationshipElement',
  'SubmodelElement','SubmodelElementCollection','SubmodelElementList'
);
CREATE TYPE data_type_def_xsd AS ENUM (
  'xs:anyURI','xs:base64Binary','xs:boolean','xs:byte','xs:date','xs:dateTime',
  'xs:decimal','xs:double','xs:duration','xs:float','xs:gDay','xs:gMonth',
  'xs:gMonthDay','xs:gYear','xs:gYearMonth','xs:hexBinary','xs:int','xs:integer',
  'xs:long','xs:negativeInteger','xs:nonNegativeInteger','xs:nonPositiveInteger',
  'xs:positiveInteger','xs:short','xs:string','xs:time','xs:unsignedByte',
  'xs:unsignedInt','xs:unsignedLong','xs:unsignedShort'
);
CREATE TYPE reference_types AS ENUM ('ExternalReference', 'ModelReference');
CREATE TYPE qualifier_kind AS ENUM ('ConceptQualifier','TemplateQualifier','ValueQualifier');
CREATE TYPE entity_type AS ENUM ('CoManagedEntity','SelfManagedEntity');
CREATE TYPE direction AS ENUM ('input','output');
CREATE TYPE state_of_event AS ENUM ('off','on');
CREATE TYPE operation_var_role AS ENUM ('in','out','inout');
CREATE TYPE key_type AS ENUM ('AnnotatedRelationshipElement','AssetAdministrationShell','BasicEventElement','Blob',
'Capability','ConceptDescription','DataElement','Entity','EventElement','File','FragmentReference','GlobalReference','Identifiable',
'MultiLanguageProperty','Operation','Property','Range','Referable','ReferenceElement','RelationshipElement','Submodel','SubmodelElement',
'SubmodelElementCollection','SubmodelElementList');

-- Reference (for semanticId etc.)  --  keys[i] keeps track of order
CREATE TABLE IF NOT EXISTS reference (
  id           BIGSERIAL PRIMARY KEY,
  type         reference_types NOT NULL
);

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

CREATE TABLE IF NOT EXISTS submodel (
  id          TEXT PRIMARY KEY,                 -- Identifiable.id
  id_short    TEXT,
  category    TEXT,
  kind        modelling_kind,
  semantic_id BIGINT REFERENCES reference(id),
  model_type  TEXT NOT NULL DEFAULT 'Submodel'
);
CREATE INDEX IF NOT EXISTS ix_sm_idshort ON submodel(id_short);

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
  id_short       TEXT NOT NULL,
  category       TEXT,
  model_type     aas_submodel_elements NOT NULL,
  semantic_id    BIGINT REFERENCES reference(id),
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
  submodel_element_id BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  kind              qualifier_kind NOT NULL,
  type              TEXT NOT NULL,
  value_type        data_type_def_xsd NOT NULL,
  value_text        TEXT,
  value_num         NUMERIC,
  value_bool        BOOLEAN,
  value_time        TIME,
  value_datetime    TIMESTAMPTZ,
  value_id          BIGINT REFERENCES reference(id)
);
CREATE INDEX IF NOT EXISTS ix_qual_sme       ON qualifier(submodel_element_id);
CREATE INDEX IF NOT EXISTS ix_qual_type      ON qualifier(type);
CREATE INDEX IF NOT EXISTS ix_qual_num       ON qualifier(value_num)
  WHERE value_type IN ('xs:decimal','xs:double','xs:float','xs:int','xs:integer','xs:long','xs:short');
CREATE INDEX IF NOT EXISTS ix_qual_text_trgm ON qualifier USING GIN (value_text gin_trgm_ops)
  WHERE value_type = 'xs:string';
