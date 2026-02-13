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
  id   BIGINT PRIMARY KEY REFERENCES specific_asset_id(id) ON DELETE CASCADE,
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
