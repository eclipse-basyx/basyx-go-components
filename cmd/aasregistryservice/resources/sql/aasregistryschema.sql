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
    CREATE TYPE key_type AS ENUM ('AnnotatedRelationshipElement','AssetAdministrationShell','BasicEventElement','Blob',
      'Capability','ConceptDescription','DataElement','Entity','EventElement','File','FragmentReference','GlobalReference','Identifiable',
      'MultiLanguageProperty','Operation','Property','Range','Referable','ReferenceElement','RelationshipElement','Submodel','SubmodelElement',
      'SubmodelElementCollection','SubmodelElementList');
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS lang_string_text_type_reference(
  id       BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS lang_string_name_type_reference(
  id       BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS lang_string_text_type (
  id     BIGSERIAL PRIMARY KEY,
  lang_string_text_type_reference_id BIGINT NOT NULL REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     varchar(1023) NOT NULL
);

CREATE TABLE IF NOT EXISTS lang_string_name_type (
  id     BIGSERIAL PRIMARY KEY,
  lang_string_name_type_reference_id BIGINT NOT NULL REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  text     varchar(128) NOT NULL
);

CREATE TABLE IF NOT EXISTS reference (
  id                BIGSERIAL         PRIMARY KEY,
  type              reference_types   NOT NULL,
  parent_reference  BIGINT            REFERENCES reference(id) 
);


CREATE TABLE IF NOT EXISTS reference_key (
  id           BIGSERIAL PRIMARY KEY,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE,
  position     INTEGER NOT NULL,                -- <- Array-Index keys[i]
  type         key_type     NOT NULL,
  value        TEXT     NOT NULL,
  UNIQUE(reference_id, position)
);


CREATE TABLE IF NOT EXISTS specific_asset_id (
    id                            BIGSERIAL           PRIMARY KEY,
    semantic_id                   BIGINT              UNIQUE REFERENCES reference(id),
    supplemental_semantic_id      BIGINT              REFERENCES reference(id),
    name                          TEXT                NOT NULL,
    value                         TEXT                NOT NULL,
    external_subject_ref          BIGINT              UNIQUE REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS security_attributes (
    id                      BIGSERIAL       NOT NULL PRIMARY KEY,
    securityType            security_type   NOT NULL,
    securityKey             TEXT            NOT NULL,
    securityValue           TEXT            NOT NULL
);

CREATE TABLE IF NOT EXISTS endpoint_protocol_version (
    id                            BIGSERIAL       PRIMARY KEY,
    endpoint_protocol_version     VARCHAR(128)    NOT NULL
);

CREATE TABLE IF NOT EXISTS aas_descriptor_endpoint (
    id                            BIGSERIAL       PRIMARY KEY,
    href                          VARCHAR(2048)   NOT NULL,
    endpoint_protocol             VARCHAR(128),
    endpoint_protocol_version_id  BIGINT          REFERENCES endpoint_protocol_version(id) ON DELETE CASCADE,
    sub_protocol                  VARCHAR(128),
    sub_protocol_body             VARCHAR(2048),
    sub_protocol_body_encoding    VARCHAR(128),
    security_attributes_id        BIGINT          REFERENCES security_attributes(id) ON DELETE CASCADE,
    interface                     VARCHAR(128)    NOT NULL
);

CREATE TABLE IF NOT EXISTS administrative_information (
    id                      BIGSERIAL           PRIMARY KEY,
    version                 VARCHAR(4),
    revision                VARCHAR(4),
    creator                 BIGINT              UNIQUE REFERENCES reference(id) ON DELETE CASCADE, -- todo: add _id
    templateId              VARCHAR(2048),
    data_specification_id   BIGINT              REFERENCES reference(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS submodel (
    id          varchar(2048) PRIMARY KEY,
    id_short    varchar(128),
    category    varchar(128),
    kind        modelling_kind,
    administration_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
    semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
    description_id BIGINT REFERENCES lang_string_text_type_reference(id) ON DELETE CASCADE,
    displayname_id  BIGINT REFERENCES lang_string_name_type_reference(id) ON DELETE CASCADE,
    model_type  TEXT NOT NULL DEFAULT 'Submodel'
);

CREATE TABLE IF NOT EXISTS extension (
  id              BIGSERIAL         PRIMARY KEY,
  semantic_id     BIGINT            UNIQUE REFERENCES reference(id) ON DELETE CASCADE,
  supplemental_semantic_id      BIGINT              REFERENCES reference(id),
  name            VARCHAR(128)      NOT NULL
);


CREATE TABLE IF NOT EXISTS submodel_descriptor (
    id                            VARCHAR(2048)     NOT NULL PRIMARY KEY,
    administrative_information_id BIGINT            UNIQUE REFERENCES administrative_information(id) ON DELETE CASCADE,
    description_id                BIGINT            REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
    displayname_id                BIGINT            REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
    extension_id                  BIGINT            REFERENCES extension(id) ON DELETE CASCADE,
    aas_descriptor_endpoint_id    BIGINT            NOT NULL REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
    id_short                      VARCHAR(128),
    semantic_id                   BIGINT            UNIQUE REFERENCES reference(id) ON DELETE CASCADE,
    supplemental_semantic_id      BIGINT            REFERENCES reference(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS aas_descriptor (
    id                            VARCHAR(2048)     NOT NULL PRIMARY KEY,
    description_id                BIGINT            REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
    displayname_id                BIGINT            REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
    extension_id                  BIGINT            REFERENCES extension(id) ON DELETE CASCADE,
    administrative_information_id BIGINT            UNIQUE REFERENCES administrative_information(id) ON DELETE CASCADE,
    asset_kind                    asset_kind,
    asset_type                    VARCHAR(2048),
    aas_descriptor_endpoint_id    BIGINT            REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
    id_short                      VARCHAR(128),
    globalAssetId                 VARCHAR(2048),
    specific_asset_id             BIGINT            REFERENCES specific_asset_id(id) ON DELETE CASCADE,
    submodel_descriptor_id        VARCHAR(2048)     REFERENCES submodel_descriptor(id) ON DELETE CASCADE
);
