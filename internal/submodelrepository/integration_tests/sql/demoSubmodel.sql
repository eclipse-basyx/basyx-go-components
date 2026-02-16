
-- Create DemoSubmodel
WITH sm AS (
	INSERT INTO submodel (submodel_identifier, id_short, category, kind, model_type)
	VALUES ('http://iese.fraunhofer.de/id/sm/DemoSubmodel', 'DemoSubmodel', 'DemoCategory', 1, 7)
	RETURNING id
), sm_payload AS (
	INSERT INTO submodel_payload (
		submodel_id,
		description_payload,
		displayname_payload,
		administrative_information_payload,
		embedded_data_specification_payload,
		supplemental_semantic_ids_payload,
		extensions_payload,
		qualifiers_payload
	)
	SELECT id, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb
	FROM sm
), sem_ref AS (
	INSERT INTO submodel_semantic_id_reference (id, type)
	SELECT id, 1 FROM sm
	RETURNING id
), sem_ref_key AS (
	INSERT INTO submodel_semantic_id_reference_key (reference_id, position, type, value)
	SELECT id, 0, 20, 'http://example.com/keys/123' FROM sem_ref
)
SELECT 1;

-- Create OnlyFileSubmodel for file attachment tests
WITH sm AS (
	INSERT INTO submodel (submodel_identifier, id_short, kind, model_type)
	VALUES ('http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel', 'OnlyFileSubmodel', 1, 7)
	RETURNING id
), sm_payload AS (
	INSERT INTO submodel_payload (
		submodel_id,
		description_payload,
		displayname_payload,
		administrative_information_payload,
		embedded_data_specification_payload,
		supplemental_semantic_ids_payload,
		extensions_payload,
		qualifiers_payload
	)
	SELECT id, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb
	FROM sm
)
SELECT 1;

-- Create DemoFile element in OnlyFileSubmodel
WITH sm AS (
	SELECT id
	FROM submodel
	WHERE submodel_identifier = 'http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel'
)
INSERT INTO submodel_element (submodel_id, id_short, idshort_path, model_type, position)
SELECT id, 'DemoFile', 'DemoFile', 16, 0
FROM sm;

-- Create payload entry for DemoFile element
INSERT INTO submodel_element_payload (
	submodel_element_id,
	description_payload,
	displayname_payload,
	administrative_information_payload,
	embedded_data_specification_payload,
	supplemental_semantic_ids_payload,
	extensions_payload
)
SELECT id, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb, '[]'::jsonb
FROM submodel_element
WHERE id_short = 'DemoFile'
  AND submodel_id = (
	SELECT id FROM submodel WHERE submodel_identifier = 'http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel'
  );

-- Create file_element entry for DemoFile
INSERT INTO file_element (id, content_type, value)
SELECT id, '', ''
FROM submodel_element
WHERE id_short = 'DemoFile'
  AND submodel_id = (
	SELECT id FROM submodel WHERE submodel_identifier = 'http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel'
  );
