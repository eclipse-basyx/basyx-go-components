CREATE TABLE IF NOT EXISTS aas (
    id           VARCHAR(2048) PRIMARY KEY,
    id_short     VARCHAR(2048),
    category     VARCHAR(2048),
    model_type   VARCHAR(128) NOT NULL
);