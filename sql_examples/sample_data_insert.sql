-- Active: 1756820296152@@127.0.0.1@5432@basyxTestDatabase@public
-- ---------------------------------------------------
-- Insert a Submodel
-- ---------------------------------------------------
INSERT INTO submodel (id, id_short, kind)
VALUES ('sm-42', 'ExampleSM', 'Instance');

-- ---------------------------------------------------
-- Root-level Property: temperature
-- ---------------------------------------------------
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (1001, 'sm-42', NULL, 0, 'temperature', 'Property', 'sm_42.temperature');

INSERT INTO property_element (id, value_type, value_num)
VALUES (1001, 'xs:int', 42);

-- ---------------------------------------------------
-- Root-level MultiLanguageProperty: title
-- ---------------------------------------------------
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (1002, 'sm-42', NULL, 1, 'title', 'MultiLanguageProperty', 'sm_42.title'),
 (5005, 'sm-42', 5004, 0, 'prop_in_smc', 'Property', 'sm_42.smc_one.smc_two.prop_in_smc'),
 (5004, 'sm-42', 5003, 1, 'smc_two', 'SubmodelElementCollection', 'sm_42.smc_one.smc_two'),
 (5003, 'sm-42', NULL, 1, 'smc_one', 'SubmodelElementCollection', 'sm_42.smc_one');

INSERT INTO multilanguage_property (id) VALUES (1002);

INSERT INTO multilanguage_property_value (mlp_id, language, text)
VALUES 
  (1002, 'de', 'Temperaturanzeige'),
  (1002, 'en', 'Temperature Display');

-- ---------------------------------------------------
-- Root-level File: manualUrl
-- ---------------------------------------------------
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (1003, 'sm-42', NULL, 2, 'manualUrl', 'File', 'sm_42.manualUrl');

INSERT INTO file_element (id, content_type, value)
VALUES (1003, 'text/html', 'https://example.com/manual.html');

-- ---------------------------------------------------
-- Root-level Collection: sensors
-- ---------------------------------------------------
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (2000, 'sm-42', NULL, 3, 'sensors', 'SubmodelElementCollection', 'sm_42.sensors');

INSERT INTO submodel_element_collection (id) VALUES (5004), (2000);

-- Nested element in sensors: sensor1.temperature
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (2001, 'sm-42', 2000, 0, 'sensor1_temperature', 'Property', 'sm_42.sensors.sensor1_temperature');

INSERT INTO property_element (id, value_type, value_text)
VALUES (2001, 'xs:double', 23.5),
       (5005, 'xs:string', 'my_string');

-- Nested element in sensors: sensor1.status
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, model_type, idshort_path)
VALUES (2002, 'sm-42', 2000, 1, 'sensor1_status', 'Property', 'sm_42.sensors.sensor1_status');

INSERT INTO property_element (id, value_type, value_text)
VALUES (2002, 'xs:string', 'OK');

-- ---------------------------------------------------
-- Submodel Element List example, List items do not have id_shorts
-- ---------------------------------------------------
INSERT INTO submodel_element (id, submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
VALUES
(9000, 'sm-42', NULL, 0, 'sml_example', NULL, 'SubmodelElementList', NULL, 'sm_42.sml_example'),
(9001, 'sm-42', 9000, 0, 'item0', NULL, 'Property', NULL, 'sm_42.sml_example[0]'),
(9002, 'sm-42', 9000, 1, 'item1', NULL, 'Property', NULL, 'sm_42.sml_example[1]'),
(9003, 'sm-42', 9000, 2, 'item2', NULL, 'SubmodelElementCollection', NULL, 'sm_42.sml_example[2]');