-- Enums
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

-- AdministrativeInformation (FK target)
CREATE TABLE IF NOT EXISTS administrativeInformation (
    id          BIGINT          PRIMARY KEY,
    version     VARCHAR(128),
    revision    VARCHAR(128),
    creator     VARCHAR(2048),
    templateId  VARCHAR(2048),
    CONSTRAINT administrativeInformation_version_revision_chk
      CHECK (version IS NOT NULL OR revision IS NULL)
);

-- Protocol info FIRST (others will reference this)
CREATE TABLE IF NOT EXISTS protocol_information (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    href                    VARCHAR(2048)   NOT NULL,
    schemeType              VARCHAR(128),
    subProtocol             VARCHAR(128),
    subProtocolBody         VARCHAR(2048),
    subProtocolBodyEncoding VARCHAR(2048)
);

-- AAS descriptor (uses AdministrativeInformation)
CREATE TABLE IF NOT EXISTS aas_descriptor (
    id              VARCHAR(2048)   PRIMARY KEY,
    idShort         VARCHAR(128),
    globalAssetId   VARCHAR(2048),
    assetType       VARCHAR(2048),
    assetKind       asset_kind,
    administration  BIGINT          REFERENCES administrativeInformation(id) ON DELETE CASCADE
);

-- Submodel descriptor (references AAS descriptor)
CREATE TABLE IF NOT EXISTS submodel_descriptor (
    id            VARCHAR(2048) NOT NULL PRIMARY KEY,
    aasDescriptor VARCHAR(2048) REFERENCES aas_descriptor(id),
    idShort       VARCHAR(128)
);

-- Endpoint tables (now after parents exist)
CREATE TABLE IF NOT EXISTS aas_descriptor_endpoint (
    id                  BIGINT          PRIMARY KEY,
    aasDescriptor       VARCHAR(2048)   NOT NULL REFERENCES aas_descriptor(id) ON DELETE CASCADE,
    interface           VARCHAR(128)    NOT NULL,
    protocolInformation BIGINT          NOT NULL REFERENCES protocol_information(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS submodel_descriptor_endpoint (
    id                  BIGINT          PRIMARY KEY,
    submodelDescriptor  VARCHAR(2048)   NOT NULL REFERENCES submodel_descriptor(id) ON DELETE CASCADE,
    interface           VARCHAR(128)    NOT NULL,
    protocolInformation BIGINT          NOT NULL REFERENCES protocol_information(id) ON DELETE CASCADE
);

-- Other protocol-related tables (after protocol_information)
CREATE TABLE IF NOT EXISTS endpoint_protocol_version (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    protocol_information    BIGINT          NOT NULL REFERENCES protocol_information(id) ON DELETE CASCADE,
    value                   VARCHAR(128)    NOT NULL
);

CREATE TABLE IF NOT EXISTS security_attributes (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    protocol_information    BIGINT          NOT NULL REFERENCES protocol_information(id) ON DELETE CASCADE,
    securityType            security_type   NOT NULL,
    securityKey             TEXT            NOT NULL,
    securityValue           TEXT            NOT NULL
);
