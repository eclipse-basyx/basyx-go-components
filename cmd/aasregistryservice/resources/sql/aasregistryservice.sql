DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'asset_kind') THEN
    CREATE TYPE asset_kind AS ENUM ('Instance', 'Type', 'Role', 'NotApplicable');
 END IF;

 DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'security_type') THEN
    CREATE TYPE security_type AS ENUM ('NONE', 'RFC_TLSA', 'W3C_DID');
 END IF;

CREATE TABLE IF NOT EXISTS aas_descriptor (
    id              VARCHAR(2048)   PRIMARY KEY,
    idShort         VARCHAR(128),
    globalAssetId   VARCHAR(2048),
    assetType       VARCHAR(2048),
    assetKind       asset_kind,
    administration  BIGINT          REFERENCES administrativeInformation(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS 'endpoint' (
    id                  BIGINT          PRIMARY KEY,
    aasDescriptor       BIGINT          NOT NULL REFERENCES aas_descriptor(id),
    interface           VARCHAR(128)    NOT NULL,
    protocolInformation BIGINT          NOT NULL REFERENCES protocol_information(id)
)

CREATE TABLE IF NOT EXISTS protocol_information (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    href                    VARCHAR(2048)   NOT NULL,
    schemeType              VARCHAR(128),
    subProtocol             VARCHAR(128),
    subProtocolBody         VARCHAR(2048),
    subProtocolBodyEncoding VARCHAR(2048)
)

CREATE TABLE IF NOT EXISTS endpoint_protocol_version (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    protocol_information    BIGINT          NOT NULL REFERENCES protocol_information(id),
    value                   VARCHAR(128)    NOT NULL
)

CREATE TABLE IF NOT EXISTS security_attributes (
    id                      BIGINT          NOT NULL PRIMARY KEY,
    protocol_information    BIGINT          NOT NULL REFERENCES protocol_information(id),
    securityType            security_type   NOT NULL,
    securityKey             TEXT            NOT NULL,
    securityValue           TEXT            NOT NULL
)