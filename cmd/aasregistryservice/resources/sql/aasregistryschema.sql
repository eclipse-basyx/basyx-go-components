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

CREATE TABLE IF NOT EXISTS descriptor (id BIGSERIAL PRIMARY KEY);

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


CREATE TABLE IF NOT EXISTS extension (
  id          BIGSERIAL PRIMARY KEY,
  semantic_id BIGINT REFERENCES reference(id) ON DELETE CASCADE,
  name       varchar(128) NOT NULL,
  value_type    data_type_def_xsd NOT NULL,
  value_text    TEXT,
  value_num     NUMERIC,
  value_bool    BOOLEAN,
  value_time    TIME,
  value_datetime TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS descriptor_extension (
  id              BIGSERIAL   PRIMARY KEY,
  descriptor_id   BIGINT      NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
  extension_id    BIGINT      NOT NULL REFERENCES extension(id) ON DELETE CASCADE 
);


CREATE TABLE IF NOT EXISTS extension_reference (
  id              BIGSERIAL   PRIMARY KEY,
  extension_id    BIGINT      NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  reference_id    BIGINT      NOT NULL REFERENCES reference(id) ON DELETE CASCADE 
);

CREATE TABLE IF NOT EXISTS specific_asset_id (
    id                            BIGSERIAL           PRIMARY KEY,
    descriptor_id                 BIGINT              NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
    semantic_id                   BIGINT              REFERENCES reference(id),
    name                          TEXT                NOT NULL,
    value                         TEXT                NOT NULL,
    external_subject_ref          BIGINT              REFERENCES reference(id)
);

CREATE TABLE IF NOT EXISTS specific_asset_id_supplemental_semantic_id (
  id                      BIGSERIAL   PRIMARY KEY,
  specific_asset_id_id    BIGINT      NOT NULL REFERENCES specific_asset_id(id) ON DELETE CASCADE,
  reference_id            BIGINT      NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS aas_descriptor_endpoint (
    id                            BIGSERIAL       PRIMARY KEY,
    descriptor_id                 BIGINT          NOT NULL REFERENCES descriptor(id) ON DELETE CASCADE,
    href                          VARCHAR(2048)   NOT NULL,
    endpoint_protocol             VARCHAR(128),
    sub_protocol                  VARCHAR(128),
    sub_protocol_body             VARCHAR(2048),
    sub_protocol_body_encoding    VARCHAR(128),
    interface                     VARCHAR(128)    NOT NULL
);

CREATE TABLE IF NOT EXISTS security_attributes (
    id                      BIGSERIAL       NOT NULL PRIMARY KEY,
    endpoint_id             BIGINT          NOT NULL REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
    securityType            security_type   NOT NULL,
    securityKey             TEXT            NOT NULL,
    securityValue           TEXT            NOT NULL
);

CREATE TABLE IF NOT EXISTS endpoint_protocol_version (
    id                            BIGSERIAL       PRIMARY KEY,
    endpoint_id                   BIGINT          NOT NULL REFERENCES aas_descriptor_endpoint(id) ON DELETE CASCADE,
    endpoint_protocol_version     VARCHAR(128)    NOT NULL
);


CREATE TABLE IF NOT EXISTS administrative_information (
    id                      BIGSERIAL           PRIMARY KEY,
    version                 VARCHAR(4),
    revision                VARCHAR(4),
    creator                 BIGINT              REFERENCES reference(id) ON DELETE CASCADE, -- todo: add _id
    templateId              VARCHAR(2048)
);

CREATE TABLE IF NOT EXISTS aas_descriptor (
    descriptor_id                 BIGINT            PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
    description_id                BIGINT            REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
    displayname_id                BIGINT            REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
    administrative_information_id BIGINT            REFERENCES administrative_information(id) ON DELETE CASCADE,
    asset_kind                    asset_kind,
    asset_type                    VARCHAR(2048),
    globalAssetId                 VARCHAR(2048),
    id_short                      VARCHAR(128),
    id                            VARCHAR(2048)     NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS submodel_descriptor (
    descriptor_id                 BIGINT            PRIMARY KEY REFERENCES descriptor(id) ON DELETE CASCADE,
    aas_descriptor_id             BIGINT            REFERENCES aas_descriptor(descriptor_id) ON DELETE CASCADE,
    description_id                BIGINT            REFERENCES lang_string_text_type_reference(id) ON DELETE SET NULL,
    displayname_id                BIGINT            REFERENCES lang_string_name_type_reference(id) ON DELETE SET NULL,
    administrative_information_id BIGINT            REFERENCES administrative_information(id) ON DELETE CASCADE,
    id_short                      VARCHAR(128),
    id                            VARCHAR(2048)     NOT NULL,
    semantic_id                   BIGINT            REFERENCES reference(id) ON DELETE CASCADE,
    UNIQUE(id)
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_supplemental_semantic_id (
  id                      BIGSERIAL   PRIMARY KEY,
  descriptor_id           BIGINT      NOT NULL REFERENCES submodel_descriptor(descriptor_id) ON DELETE CASCADE,
  reference_id            BIGINT      NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);
