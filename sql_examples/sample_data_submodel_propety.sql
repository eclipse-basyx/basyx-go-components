-- Submodel
-- -- First Create the Reference for the SemanticID of the Submodel
INSERT INTO reference (id, type)
VALUES (1, 'ModelReference');
-- -- Second Create the Key Entry for the Reference (Here: A Submodel Key)
INSERT INTO reference_key (id, reference_id, position, type, value)
VALUES (1, 1, 0, 'Submodel', 'http://example.com/keys/123');
-- -- Finally Create the Submodel itself, linking to the Reference created above
INSERT INTO submodel (id, id_short, category, kind, semantic_id, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/DemoSubmodel', 'DemoSubmodel', 'DemoCategory', 'Instance', 1, 'Submodel');


-- Submodel Element: Property
INSERT INTO reference (id, type)
VALUES (2, 'ModelReference');
INSERT INTO reference_key (id, reference_id, position, type, value)
VALUES (2, 2, 0, 'Submodel', 'http://iese.fraunhofer.de/id/sm/DemoSubmodel');
INSERT INTO submodel_element(id, submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
VALUES (2, 'http://iese.fraunhofer.de/id/sm/DemoSubmodel', NULL, 0, 'DemoProperty', 'DemoCategory', 'Property', 2, 'DemoProperty');

INSERT INTO property_element (id, value_type, value_text)
VALUES(1, 'xs:string', 'Demo Property Value');