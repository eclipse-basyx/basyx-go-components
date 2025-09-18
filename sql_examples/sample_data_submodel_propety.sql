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

SELECT 
    sme.id, sme.id_short, sme.category, sme.model_type, sme.idshort_path, sme.position, sme.parent_sme_id,
    -- Property data
    prop.value_type as prop_value_type, 
    COALESCE(prop.value_text, prop.value_num::text, prop.value_bool::text, 
                prop.value_time::text, prop.value_datetime::text) as prop_value,
    -- MultiLanguageProperty data  
    mlp.id as mlp_id,
    -- Blob data
    blob.content_type as blob_content_type, blob.value as blob_value,
    -- File data
    file.content_type as file_content_type, file.value as file_value,
    -- Range data
    range_elem.value_type as range_value_type,
    COALESCE(range_elem.min_text, range_elem.min_num::text, range_elem.min_time::text, range_elem.min_datetime::text) as range_min,
    COALESCE(range_elem.max_text, range_elem.max_num::text, range_elem.max_time::text, range_elem.max_datetime::text) as range_max,
    -- SubmodelElementList data
    sme_list.type_value_list_element, sme_list.value_type_list_element, sme_list.order_relevant
FROM submodel_element sme
LEFT JOIN property_element prop ON sme.id = prop.id
LEFT JOIN multilanguage_property mlp ON sme.id = mlp.id
LEFT JOIN blob_element blob ON sme.id = blob.id
LEFT JOIN file_element file ON sme.id = file.id
LEFT JOIN range_element range_elem ON sme.id = range_elem.id
LEFT JOIN submodel_element_list sme_list ON sme.id = sme_list.id
WHERE sme.submodel_id = 'http://iese.fraunhofer.de/id/sm/DemoSubmodel'
AND (sme.idshort_path LIKE 'MyFirstList9[0]' || '.%' OR sme.idshort_path LIKE 'MyFirstList9[0]' || '[%' OR sme.idshort_path = 'MyFirstList9[0]')