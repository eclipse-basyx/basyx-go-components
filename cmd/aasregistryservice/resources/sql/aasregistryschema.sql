-- =========================
-- AAS Registry Schema (with performance-oriented indexes)
-- =========================

-- ---------- Enums ----------
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'asset_kind') THEN
    CREATE TYPE asset_kind AS ENUM ('Instance', 'Type', 'Role', 'NotApplicable');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'security_type') THEN
    CREATE TYPE security_type AS ENUM ('NONE', 'RFC_TLSA', 'W3C_DID');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'reference_types') THEN
    CREATE TYPE reference_types AS ENUM ('ExternalReference', 'ModelReference');
  END IF;
END $$;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'modelling_kind') THEN
    CREATE TYPE modelling_kind AS ENUM ('Instance', 'Template');
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
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'key_type') THEN
    CREATE TYPE key_type AS ENUM (
      'AnnotatedRelationshipElement','AssetAdministrationShell','BasicEventElement','Blob',
      'Capability','ConceptDescription','DataElement','Entity','EventElement','File','FragmentReference','GlobalReference','Identifiable',
      'MultiLanguageProperty','Operation','Property','Range','Referable','ReferenceElement','RelationshipElement','Submodel','SubmodelElement',
      'SubmodelElementCollection','SubmodelElementList'
    );
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

-- ---------- Core tables ----------
CREATE TABLE IF NOT EXISTS descriptor (
  id BIGSERIAL PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS lang_string_text_type_reference(
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS lang_string_name_type_reference(
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS lang_string_text_type (
  id BIGSERIAL PRIMARY KEY,
  lang_string_text_type_reference_id BIGINT NOT NULL REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text varchar(1023) NOT NULL
);

CREATE TABLE IF NOT EXISTS lang_string_name_type (
  id BIGSERIAL PRIMARY KEY,
  lang_string_name_type_reference_id BIGINT NOT NULL REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text varchar(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS reference (
  id           BIGSERIAL PRIMARY KEY,
  type         reference_types NOT NULL,
  parentReference BIGINT REFERENCES reference(id),  -- Optional nesting
  rootReference BIGINT REFERENCES reference(id)  -- The root of the nesting tree
);

CREATE INDEX IF NOT EXISTS idx_reference_rootreference ON reference(rootreference);
-- if you often filter by BOTH columns, this can help even more:
CREATE INDEX IF NOT EXISTS idx_reference_rootreference_id ON reference(rootreference, id);

CREATE TABLE IF NOT EXISTS reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,                -- <- Array-Index keys[i]
  type         key_type     NOT NULL,
  value        TEXT     NOT NULL,
  UNIQUE(reference_id, position)
);

CREATE TABLE IF NOT EXISTS extension (
  id BIGSERIAL PRIMARY KEY,
  semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  name varchar(128) NOT NULL,
  value_type data_type_def_xsd,
  value_text TEXT,
  value_num NUMERIC,
  value_bool BOOLEAN,
  value_time TIME,
  value_datetime TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS descriptor_extension (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS extension_reference_refer_to (
  id BIGSERIAL PRIMARY KEY,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS extension_reference_supplemental (
  id BIGSERIAL PRIMARY KEY,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS specific_asset_id (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
  semantic_id BIGINT REFERENCES reference(id),
  name VARCHAR(64) NOT NULL,
  value VARCHAR(2048) NOT NULL,
  external_subject_ref BIGINT REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS specific_asset_id_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  specific_asset_id_id BIGINT NOT NULL REFERENCES specific_asset_id(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS aas_descriptor_endpoint (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
  href VARCHAR(2048) NOT NULL,
  endpoint_protocol VARCHAR(128),
  sub_protocol VARCHAR(128),
  sub_protocol_body VARCHAR(2048),
  sub_protocol_body_encoding VARCHAR(128),
  interface VARCHAR(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS security_attributes (
  id BIGSERIAL NOT NULL PRIMARY KEY,
  endpoint_id BIGINT NOT NULL REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
  security_type security_type NOT NULL,
  security_key TEXT NOT NULL,
  security_value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS endpoint_protocol_version (
  id BIGSERIAL PRIMARY KEY,
  endpoint_id BIGINT NOT NULL REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
  endpoint_protocol_version VARCHAR(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS administrative_information (
  id BIGSERIAL PRIMARY KEY,
  version VARCHAR(4),
  revision VARCHAR(4),
  creator BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  templateid VARCHAR(2048)
);


CREATE TABLE IF NOT EXISTS data_specification_content (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

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

CREATE TABLE IF NOT EXISTS administrative_information_embedded_data_specification (
  id                BIGSERIAL PRIMARY KEY,
  administrative_information_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  data_specification_content_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_eds_id ON administrative_information_embedded_data_specification(id);


CREATE TABLE IF NOT EXISTS aas_descriptor (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  description_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
  displayname_id BIGINT REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
  administrative_information_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  asset_kind asset_kind,
  asset_type VARCHAR(2048),
  global_asset_id VARCHAR(2048),
  id_short VARCHAR(128),
  id VARCHAR(2048) NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS submodel_descriptor (
  descriptor_id BIGINT PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
  aas_descriptor_id BIGINT REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
  description_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
  displayname_id BIGINT REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
  administrative_information_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  id_short VARCHAR(128),
  id VARCHAR(2048) NOT NULL,
  semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  UNIQUE(id)
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  descriptor_id BIGINT NOT NULL REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);

-- Reference tree
CREATE INDEX IF NOT EXISTS idx_reference_rootreference ON reference(rootreference);
CREATE INDEX IF NOT EXISTS idx_reference_rootreference_id ON reference(rootreference, id);
CREATE INDEX IF NOT EXISTS idx_reference_parentReference ON reference(parentReference);
CREATE INDEX IF NOT EXISTS idx_reference_type ON reference(type);

-- Reference keys
CREATE INDEX IF NOT EXISTS idx_reference_key_reference ON reference_key(reference_id);
CREATE INDEX IF NOT EXISTS idx_reference_key_refid_id ON reference_key(reference_id, id);
CREATE INDEX IF NOT EXISTS idx_reference_key_type_value ON reference_key(type, value);

-- Lang string references
CREATE INDEX IF NOT EXISTS idx_lang_string_text_type_ref ON lang_string_text_type(lang_string_text_type_reference_id);
CREATE INDEX IF NOT EXISTS idx_lang_string_name_type_ref ON lang_string_name_type(lang_string_name_type_reference_id);
CREATE INDEX IF NOT EXISTS idx_lang_text_ref_lang ON lang_string_text_type(lang_string_text_type_reference_id, language, id);
CREATE INDEX IF NOT EXISTS idx_lang_name_ref_lang ON lang_string_name_type(lang_string_name_type_reference_id, language, id);

-- Extension & links
CREATE INDEX IF NOT EXISTS idx_extension_semantic_id ON extension(semantic_id);
CREATE INDEX IF NOT EXISTS idx_extension_value_type ON extension(value_type);
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_descriptor ON descriptor_extension(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_extension ON descriptor_extension(extension_id);
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_descriptor_extension ON descriptor_extension(descriptor_id, extension_id);

CREATE INDEX IF NOT EXISTS idx_ext_ref_refer_extension ON extension_reference_refer_to(extension_id);
CREATE INDEX IF NOT EXISTS idx_ext_ref_refer_reference ON extension_reference_refer_to(reference_id);
CREATE INDEX IF NOT EXISTS idx_ext_ref_refer_ext_id ON extension_reference_refer_to(extension_id, id);

CREATE INDEX IF NOT EXISTS idx_ext_ref_supp_extension ON extension_reference_supplemental(extension_id);
CREATE INDEX IF NOT EXISTS idx_ext_ref_supp_reference ON extension_reference_supplemental(reference_id);
CREATE INDEX IF NOT EXISTS idx_ext_ref_supp_ext_id ON extension_reference_supplemental(extension_id, id);

-- Specific Asset IDs
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_descriptor ON specific_asset_id(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_semantic ON specific_asset_id(semantic_id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_external_subject_ref ON specific_asset_id(external_subject_ref);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_name ON specific_asset_id(name);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_value ON specific_asset_id(value);
CREATE INDEX IF NOT EXISTS idx_sai_supp_semantic_sai_id ON specific_asset_id_supplemental_semantic_id(specific_asset_id_id, id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_sup_semantic_ref ON specific_asset_id_supplemental_semantic_id(reference_id);

-- Endpoints & security
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_descriptor ON aas_descriptor_endpoint(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_descriptor_id ON aas_descriptor_endpoint(descriptor_id, id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_interface ON aas_descriptor_endpoint(interface);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_protocols ON aas_descriptor_endpoint(endpoint_protocol, sub_protocol);
CREATE INDEX IF NOT EXISTS idx_security_attributes_endpoint ON security_attributes(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_security_attributes_endpoint_id ON security_attributes(endpoint_id, id);
CREATE INDEX IF NOT EXISTS idx_security_attributes_type ON security_attributes(security_type);
CREATE INDEX IF NOT EXISTS idx_endpoint_protocol_version_endpoint ON endpoint_protocol_version(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_ep_version_endpoint_id ON endpoint_protocol_version(endpoint_id, id);

-- Administrative info
CREATE INDEX IF NOT EXISTS idx_administrative_information_creator ON administrative_information(creator);

-- AAS descriptor lookups
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_admin_info ON aas_descriptor(administrative_information_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_description ON aas_descriptor(description_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_displayname ON aas_descriptor(displayname_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_global_asset_id ON aas_descriptor(global_asset_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_id_short ON aas_descriptor(id_short);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_asset_kind ON aas_descriptor(asset_kind);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_asset_type ON aas_descriptor(asset_type);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_assettype_id ON aas_descriptor(asset_type, id)
  INCLUDE (asset_kind, descriptor_id, global_asset_id, id_short, displayname_id, description_id, administrative_information_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_assetkind_id ON aas_descriptor(asset_kind, id)
  INCLUDE (asset_type, descriptor_id, global_asset_id, id_short, displayname_id, description_id, administrative_information_id);

-- Submodel descriptor lookups
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_aas ON submodel_descriptor(aas_descriptor_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_aas_id ON submodel_descriptor(aas_descriptor_id, id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_admin_info ON submodel_descriptor(administrative_information_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_description ON submodel_descriptor(description_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_displayname ON submodel_descriptor(displayname_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_semantic ON submodel_descriptor(semantic_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_id_short ON submodel_descriptor(id_short);
CREATE INDEX IF NOT EXISTS idx_sm_supp_semantic_desc_id ON submodel_descriptor_supplemental_semantic_id(descriptor_id, id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_sup_semantic_ref ON submodel_descriptor_supplemental_semantic_id(reference_id);
