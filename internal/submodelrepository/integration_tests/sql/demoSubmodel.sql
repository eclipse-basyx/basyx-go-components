INSERT INTO reference (type)
VALUES ('ModelReference');
-- -- Second Create the Key Entry for the Reference (Here: A Submodel Key)
INSERT INTO reference_key (reference_id, position, type, value)
VALUES (1, 0, 'Submodel', 'http://example.com/keys/123');
-- -- Finally Create the Submodel itself, linking to the Reference created above
INSERT INTO submodel (id, id_short, category, kind, semantic_id, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/DemoSubmodel', 'DemoSubmodel', 'DemoCategory', 'Instance', 1, 'Submodel');

-- Create OnlyFileSubmodel for file attachment tests
INSERT INTO submodel (id, id_short, kind, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel', 'OnlyFileSubmodel', 'Instance', 'Submodel');

-- Create DemoFile element in OnlyFileSubmodel
INSERT INTO submodel_element (submodel_id, id_short, idshort_path, model_type, position)
VALUES ('http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel', 'DemoFile', 'DemoFile', 'File', 0);

-- Create file_element entry for DemoFile
INSERT INTO file_element (id, content_type, value)
SELECT id, '', ''
FROM submodel_element
WHERE submodel_id = 'http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel' AND id_short = 'DemoFile';
