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
  id BIGSERIAL PRIMARY KEY,
  type reference_types NOT NULL,
  parent_reference BIGINT REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS reference_key (
  id BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  type key_type NOT NULL,
  value TEXT NOT NULL,
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

CREATE TABLE IF NOT EXISTS extension_reference (
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
  template_id VARCHAR(2048)
);

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

-- ---------- Existing helper indexes ----------
-- parent/child references
CREATE INDEX IF NOT EXISTS idx_reference_parent_reference ON reference(parent_reference);
CREATE INDEX IF NOT EXISTS idx_reference_type ON reference(type);

-- Keys
CREATE INDEX IF NOT EXISTS idx_reference_key_reference ON reference_key(reference_id);
CREATE INDEX IF NOT EXISTS idx_reference_key_type_value ON reference_key(type, value);

-- Lang string references
CREATE INDEX IF NOT EXISTS idx_lang_string_text_type_ref ON lang_string_text_type(lang_string_text_type_reference_id);
CREATE INDEX IF NOT EXISTS idx_lang_string_name_type_ref ON lang_string_name_type(lang_string_name_type_reference_id);

-- Extension & links
CREATE INDEX IF NOT EXISTS idx_extension_semantic_id ON extension(semantic_id);
CREATE INDEX IF NOT EXISTS idx_extension_value_type ON extension(value_type);
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_descriptor ON descriptor_extension(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_extension ON descriptor_extension(extension_id);
CREATE INDEX IF NOT EXISTS idx_extension_reference_extension ON extension_reference(extension_id);
CREATE INDEX IF NOT EXISTS idx_extension_reference_reference ON extension_reference(reference_id);

-- Specific Asset IDs
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_descriptor ON specific_asset_id(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_semantic ON specific_asset_id(semantic_id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_external_subject_ref ON specific_asset_id(external_subject_ref);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_name ON specific_asset_id(name);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_value ON specific_asset_id(value);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_sup_semantic_specific ON specific_asset_id_supplemental_semantic_id(specific_asset_id_id);
CREATE INDEX IF NOT EXISTS idx_specific_asset_id_sup_semantic_ref ON specific_asset_id_supplemental_semantic_id(reference_id);

-- Endpoints & security
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_descriptor ON aas_descriptor_endpoint(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_interface ON aas_descriptor_endpoint(interface);
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_protocols ON aas_descriptor_endpoint(endpoint_protocol, sub_protocol);
CREATE INDEX IF NOT EXISTS idx_security_attributes_endpoint ON security_attributes(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_security_attributes_type ON security_attributes(security_type);
CREATE INDEX IF NOT EXISTS idx_endpoint_protocol_version_endpoint ON endpoint_protocol_version(endpoint_id);

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

-- Submodel descriptor lookups
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_aas ON submodel_descriptor(aas_descriptor_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_admin_info ON submodel_descriptor(administrative_information_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_description ON submodel_descriptor(description_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_displayname ON submodel_descriptor(displayname_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_semantic ON submodel_descriptor(semantic_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_id_short ON submodel_descriptor(id_short);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_sup_semantic_descriptor ON submodel_descriptor_supplemental_semantic_id(descriptor_id);
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_sup_semantic_ref ON submodel_descriptor_supplemental_semantic_id(reference_id);

-- ---------- New/Improved Indexes for Query Path ----------
-- 1) Page CTE fast paths: WHERE asset_type|asset_kind [AND id >= ?] ORDER BY id ASC
--    INCLUDE helps index-only scans for the CTE projection.
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_assettype_id
  ON aas_descriptor(asset_type, id)
  INCLUDE (asset_kind, descriptor_id, global_asset_id, id_short, displayname_id, description_id, administrative_information_id);

CREATE INDEX IF NOT EXISTS idx_aas_descriptor_assetkind_id
  ON aas_descriptor(asset_kind, id)
  INCLUDE (asset_type, descriptor_id, global_asset_id, id_short, displayname_id, description_id, administrative_information_id);

-- Optional if you frequently filter by BOTH asset_type AND asset_kind:
-- CREATE INDEX IF NOT EXISTS idx_aas_descriptor_assettype_kind_id
--   ON aas_descriptor(asset_type, asset_kind, id);

-- 2) Avoid sorts inside LATERAL JSON aggregates by providing order-friendly indexes
-- Endpoints aggregated ORDER BY e.id (filter on descriptor_id)
CREATE INDEX IF NOT EXISTS idx_aas_descriptor_endpoint_descriptor_id
  ON aas_descriptor_endpoint(descriptor_id, id);

-- Submodel descriptors aggregated ORDER BY smd.id (filter on aas_descriptor_id)
CREATE INDEX IF NOT EXISTS idx_submodel_descriptor_aas_id
  ON submodel_descriptor(aas_descriptor_id, id);

-- DisplayName / Description aggregated ORDER BY language (and stable by id)
CREATE INDEX IF NOT EXISTS idx_lang_name_ref_lang
  ON lang_string_name_type(lang_string_name_type_reference_id, language, id);

CREATE INDEX IF NOT EXISTS idx_lang_text_ref_lang
  ON lang_string_text_type(lang_string_text_type_reference_id, language, id);

-- Extensions: join via descriptor_extension(descriptor_id) then order by extension.id
CREATE INDEX IF NOT EXISTS idx_descriptor_extension_descriptor_extension
  ON descriptor_extension(descriptor_id, extension_id);

-- ---------- (Optional) Planner statistics tweaks ----------
-- Uncomment if you have many distinct values or skewed distributions:
-- ALTER TABLE aas_descriptor ALTER COLUMN asset_type SET STATISTICS 2000;
-- ALTER TABLE aas_descriptor ALTER COLUMN asset_kind SET STATISTICS 2000;

-- ---------- (Optional) Physical layout ----------
-- If most scans are id-ascending pages, clustering can reduce heap I/O over time.
-- Replace the index name below with your actual unique index name on aas_descriptor(id).
-- CLUSTER VERBOSE aas_descriptor USING aas_descriptor_id_key;
-- ALTER TABLE aas_descriptor SET (fillfactor = 90);

-- Final maintenance suggestion (non-blocking if you prefer): ANALYZE;
