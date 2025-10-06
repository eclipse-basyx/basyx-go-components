CREATE TABLE IF NOT EXISTS aas_identifier (
    id          BIGSERIAL PRIMARY KEY,
    aasId       VARCHAR(2048) UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS asset_link (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(64) NOT NULL,
    value       VARCHAR(64) NOT NULL,
    aasRef      BIGSERIAL REFERENCES aas_identifier(id) ON DELETE CASCADE       
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_aas_identifier_aasid
    ON aas_identifier (aasId);

CREATE INDEX IF NOT EXISTS idx_asset_link_aasref
    ON asset_link (aasRef);

CREATE INDEX IF NOT EXISTS idx_asset_link_name_value_aasref
    ON asset_link (name, value, aasRef);