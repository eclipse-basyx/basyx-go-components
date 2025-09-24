INSERT INTO reference (type)
VALUES ('ModelReference');
-- -- Second Create the Key Entry for the Reference (Here: A Submodel Key)
INSERT INTO reference_key (reference_id, position, type, value)
VALUES (1, 0, 'Submodel', 'http://example.com/keys/123');
-- -- Finally Create the Submodel itself, linking to the Reference created above
INSERT INTO submodel (id, id_short, category, kind, semantic_id, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/DemoSubmodel', 'DemoSubmodel', 'DemoCategory', 'Instance', 1, 'Submodel');
