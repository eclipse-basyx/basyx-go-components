
CREATE TABLE IF NOT EXISTS entity_specific_asset_id (
  id                   BIGSERIAL PRIMARY KEY,
  entity_id            BIGINT NOT NULL REFERENCES entity_element(id) ON DELETE CASCADE,
  name                 TEXT NOT NULL,
  value                TEXT NOT NULL,
  external_subject_ref BIGINT REFERENCES reference(id)
);


CREATE TABLE IF NOT EXISTS operation_variable (
  id           BIGSERIAL PRIMARY KEY,
  operation_id BIGINT NOT NULL REFERENCES operation_element(id) ON DELETE CASCADE,
  role         operation_var_role NOT NULL,
  position     INTEGER NOT NULL,
  value_sme    BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  UNIQUE (operation_id, role, position)
);

CREATE TABLE IF NOT EXISTS capability_element (
  id BIGINT PRIMARY KEY REFERENCES submodel_element(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS administrative_information_embedded_data_specification (
  id                BIGSERIAL PRIMARY KEY,
  administrative_information_id BIGINT REFERENCES administrative_information(id) ON DELETE CASCADE,
  embedded_data_specification_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_aieds_aiid ON administrative_information_embedded_data_specification(administrative_information_id);
CREATE INDEX IF NOT EXISTS ix_aieds_edsid ON administrative_information_embedded_data_specification(embedded_data_specification_id);

CREATE INDEX IF NOT EXISTS ix_eds_id ON administrative_information_embedded_data_specification(id);


CREATE TABLE IF NOT EXISTS submodel_supplemental_semantic_id (
  id BIGSERIAL PRIMARY KEY,
  submodel_id VARCHAR(2048) NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  reference_id BIGINT NOT NULL REFERENCES reference(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_smssi_submodel_id ON submodel_supplemental_semantic_id(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smssi_reference_id ON submodel_supplemental_semantic_id(reference_id);

CREATE TABLE IF NOT EXISTS submodel_embedded_data_specification (
  id                BIGSERIAL PRIMARY KEY,
  submodel_id       VARCHAR(2048) REFERENCES submodel(id) ON DELETE CASCADE,
  embedded_data_specification_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS ix_eds_id ON submodel_embedded_data_specification(id);

CREATE TABLE IF NOT EXISTS submodel_extension (
  id BIGSERIAL PRIMARY KEY,
  submodel_id VARCHAR(2048) NOT NULL REFERENCES submodel(id) ON DELETE CASCADE,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE 
);
CREATE INDEX IF NOT EXISTS ix_smext_submodel_id ON submodel_extension(submodel_id);
CREATE INDEX IF NOT EXISTS ix_smext_extension_id ON submodel_extension(extension_id);



CREATE INDEX IF NOT EXISTS ix_ext_semantic_id ON extension(semantic_id);

CREATE INDEX IF NOT EXISTS ix_smext_id ON submodel_extension(id);

CREATE TABLE IF NOT EXISTS submodel_element_extension (
  submodel_element_id       BIGINT NOT NULL REFERENCES submodel_element(id) ON DELETE CASCADE,
  extension_id BIGINT NOT NULL REFERENCES extension(id) ON DELETE CASCADE,
  PRIMARY KEY (submodel_element_id, extension_id)
);

CREATE INDEX IF NOT EXISTS ix_smeext_smeid ON submodel_element_extension(submodel_element_id);


CREATE TABLE IF NOT EXISTS data_specification_iec61360 (
  id                BIGINT REFERENCES data_specification_content(id) ON DELETE CASCADE PRIMARY KEY,
  position          INTEGER,
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

CREATE INDEX IF NOT EXISTS ix_ds_dataspeciec61360_position ON data_specification_iec61360(position);
CREATE INDEX IF NOT EXISTS ix_ds_dataspec_content ON data_specification(data_specification_content);
CREATE INDEX IF NOT EXISTS ix_ds_dataspec_reference ON data_specification(data_specification);

CREATE INDEX IF NOT EXISTS ix_iec61360_value_list_id ON data_specification_iec61360(value_list_id);
CREATE INDEX IF NOT EXISTS ix_iec61360_level_type_id ON data_specification_iec61360(level_type_id);
CREATE INDEX IF NOT EXISTS ix_iec61360_data_type ON data_specification_iec61360(data_type);

CREATE TABLE IF NOT EXISTS submodel_element_embedded_data_specification (
  submodel_element_id BIGINT REFERENCES submodel_element(id) ON DELETE CASCADE,
  embedded_data_specification_id BIGSERIAL REFERENCES data_specification(id) ON DELETE CASCADE,
  PRIMARY KEY (submodel_element_id, embedded_data_specification_id)
);
CREATE INDEX IF NOT EXISTS ix_smeeds_smeid ON submodel_element_embedded_data_specification(submodel_element_id);
CREATE INDEX IF NOT EXISTS ix_dataspec_content ON data_specification (data_specification_content);

CREATE INDEX IF NOT EXISTS ix_iec61360_preferred_name ON data_specification_iec61360 (preferred_name_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_short_name ON data_specification_iec61360 (short_name_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_definition ON data_specification_iec61360 (definition_id);

CREATE INDEX IF NOT EXISTS ix_iec61360_unit_id ON data_specification_iec61360 (unit_id);

CREATE INDEX IF NOT EXISTS ix_seds_submodel ON submodel_embedded_data_specification (submodel_id);
CREATE INDEX IF NOT EXISTS ix_dsiec_id ON data_specification_iec61360(id);

CREATE TABLE IF NOT EXISTS data_specification_content (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY
);

CREATE TABLE IF NOT EXISTS data_specification (
  id BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
  data_specification BIGINT REFERENCES reference(id) NOT NULL,
  data_specification_content BIGINT REFERENCES data_specification_content(id) NOT NULL
);

CREATE INDEX IF NOT EXISTS ix_edscontent_id ON data_specification_content(id);
CREATE INDEX IF NOT EXISTS ix_smsup_id ON submodel_supplemental_semantic_id(id);
