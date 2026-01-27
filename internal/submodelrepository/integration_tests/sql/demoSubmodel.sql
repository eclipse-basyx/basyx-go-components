
-- Create the reference and key, capturing the reference id
WITH ref AS (
	INSERT INTO reference (type) VALUES (1) RETURNING id
), key_insert AS (
	INSERT INTO reference_key (reference_id, position, type, value)
	SELECT id, 0, 1, 'http://example.com/keys/123' FROM ref
)
INSERT INTO submodel (id, id_short, category, kind, semantic_id, model_type)
SELECT
	'http://iese.fraunhofer.de/id/sm/DemoSubmodel',
	'DemoSubmodel',
	'DemoCategory',
	1,
	id,
	'Submodel'
FROM ref;



-- Create OnlyFileSubmodel for file attachment tests (no semantic_id, so no reference needed)
INSERT INTO submodel (id, id_short, kind, model_type)
VALUES ('http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel', 'OnlyFileSubmodel', 1, 'Submodel');

-- Create DemoFile element in OnlyFileSubmodel
INSERT INTO submodel_element (submodel_id, id_short, idshort_path, model_type, position)
VALUES ('http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel', 'DemoFile', 'DemoFile', 'File', 0);

-- Create file_element entry for DemoFile
INSERT INTO file_element (id, content_type, value)
SELECT id, '', ''
FROM submodel_element
WHERE submodel_id = 'http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel' AND id_short = 'DemoFile';
