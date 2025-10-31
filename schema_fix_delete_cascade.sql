/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

-- Schema fix for proper cascading deletion of submodel-owned data
-- This script creates triggers to automatically clean up orphaned records
-- when a submodel is deleted, eliminating the need for application-level cleanup.

-- Function to recursively delete a reference and all its nested references
CREATE OR REPLACE FUNCTION delete_reference_recursively(ref_id BIGINT)
RETURNS VOID AS $$
DECLARE
    child_ref_id BIGINT;
BEGIN
    -- Delete all child references first (recursively)
    FOR child_ref_id IN 
        SELECT id FROM reference 
        WHERE parentReference = ref_id OR rootReference = ref_id
    LOOP
        PERFORM delete_reference_recursively(child_ref_id);
    END LOOP;
    
    -- Delete the reference itself (CASCADE will delete reference_key entries)
    DELETE FROM reference WHERE id = ref_id;
END;
$$ LANGUAGE plpgsql;

-- Trigger function to clean up submodel-owned data after submodel deletion
CREATE OR REPLACE FUNCTION cleanup_submodel_orphaned_data()
RETURNS TRIGGER AS $$
DECLARE
    qual_record RECORD;
    ext_record RECORD;
    eds_record RECORD;
    supp_semantic_record RECORD;
    admin_eds_record RECORD;
    vl_ref_id BIGINT;
    ds_ref_id BIGINT;
BEGIN
    -- Clean up qualifiers and their references
    FOR qual_record IN 
        SELECT q.id, q.semantic_id, q.value_id
        FROM qualifier q
        INNER JOIN submodel_qualifier sq ON sq.qualifier_id = q.id
        WHERE sq.submodel_id = OLD.id
    LOOP
        -- Get supplemental semantic IDs for this qualifier
        FOR supp_semantic_record IN
            SELECT reference_id FROM qualifier_supplemental_semantic_id
            WHERE qualifier_id = qual_record.id
        LOOP
            PERFORM delete_reference_recursively(supp_semantic_record.reference_id);
        END LOOP;
        
        -- Delete the qualifier (CASCADE will delete join table entries)
        DELETE FROM qualifier WHERE id = qual_record.id;
        
        -- Delete qualifier's references
        IF qual_record.semantic_id IS NOT NULL THEN
            PERFORM delete_reference_recursively(qual_record.semantic_id);
        END IF;
        IF qual_record.value_id IS NOT NULL THEN
            PERFORM delete_reference_recursively(qual_record.value_id);
        END IF;
    END LOOP;

    -- Clean up extensions and their references
    FOR ext_record IN 
        SELECT e.id, e.semantic_id
        FROM extension e
        INNER JOIN submodel_extension se ON se.extension_id = e.id
        WHERE se.submodel_id = OLD.id
    LOOP
        -- Get supplemental semantic IDs for this extension
        FOR supp_semantic_record IN
            SELECT reference_id FROM extension_supplemental_semantic_id
            WHERE extension_id = ext_record.id
        LOOP
            PERFORM delete_reference_recursively(supp_semantic_record.reference_id);
        END LOOP;
        
        -- Get refers_to references for this extension
        FOR supp_semantic_record IN
            SELECT reference_id FROM extension_refers_to
            WHERE extension_id = ext_record.id
        LOOP
            PERFORM delete_reference_recursively(supp_semantic_record.reference_id);
        END LOOP;
        
        -- Delete the extension (CASCADE will delete join table entries)
        DELETE FROM extension WHERE id = ext_record.id;
        
        -- Delete extension's semantic_id reference
        IF ext_record.semantic_id IS NOT NULL THEN
            PERFORM delete_reference_recursively(ext_record.semantic_id);
        END IF;
    END LOOP;

    -- Clean up embedded data specifications
    FOR eds_record IN
        SELECT seds.embedded_data_specification_id
        FROM submodel_embedded_data_specification seds
        WHERE seds.submodel_id = OLD.id
    LOOP
        -- Get data_specification details
        SELECT data_specification, data_specification_content INTO ds_ref_id
        FROM data_specification
        WHERE id = eds_record.embedded_data_specification_id;
        
        IF FOUND THEN
            -- Clean up data_specification_iec61360 references if they exist
            DECLARE
                iec_record RECORD;
            BEGIN
                SELECT preferred_name_id, short_name_id, unit_id, definition_id, value_list_id
                INTO iec_record
                FROM data_specification_iec61360
                WHERE id = (SELECT data_specification_content FROM data_specification WHERE id = eds_record.embedded_data_specification_id);
                
                IF FOUND THEN
                    -- Clean up value_list references
                    IF iec_record.value_list_id IS NOT NULL THEN
                        FOR vl_ref_id IN
                            SELECT value_id FROM value_list_value_reference_pair
                            WHERE value_list_id = iec_record.value_list_id AND value_id IS NOT NULL
                        LOOP
                            PERFORM delete_reference_recursively(vl_ref_id);
                        END LOOP;
                    END IF;
                    
                    -- Clean up unit_id reference
                    IF iec_record.unit_id IS NOT NULL THEN
                        PERFORM delete_reference_recursively(iec_record.unit_id);
                    END IF;
                    
                    -- Clean up lang string references
                    IF iec_record.preferred_name_id IS NOT NULL THEN
                        DELETE FROM lang_string_text_type_reference WHERE id = iec_record.preferred_name_id;
                    END IF;
                    IF iec_record.short_name_id IS NOT NULL THEN
                        DELETE FROM lang_string_text_type_reference WHERE id = iec_record.short_name_id;
                    END IF;
                    IF iec_record.definition_id IS NOT NULL THEN
                        DELETE FROM lang_string_text_type_reference WHERE id = iec_record.definition_id;
                    END IF;
                END IF;
            END;
            
            -- Delete data_specification (CASCADE will delete content)
            DELETE FROM data_specification WHERE id = eds_record.embedded_data_specification_id;
            
            -- Delete the data_specification reference
            IF ds_ref_id IS NOT NULL THEN
                PERFORM delete_reference_recursively(ds_ref_id);
            END IF;
        END IF;
    END LOOP;

    -- Clean up supplemental semantic ID references
    FOR supp_semantic_record IN
        SELECT reference_id FROM submodel_supplemental_semantic_id
        WHERE submodel_id = OLD.id
    LOOP
        PERFORM delete_reference_recursively(supp_semantic_record.reference_id);
    END LOOP;

    -- Clean up administrative information
    IF OLD.administration_id IS NOT NULL THEN
        DECLARE
            admin_creator_id BIGINT;
        BEGIN
            -- Get creator reference
            SELECT creator INTO admin_creator_id
            FROM administrative_information
            WHERE id = OLD.administration_id;
            
            -- Clean up embedded data specifications from administrative_information
            FOR admin_eds_record IN
                SELECT embedded_data_specification_id
                FROM administrative_information_embedded_data_specification
                WHERE administrative_information_id = OLD.administration_id
            LOOP
                SELECT data_specification INTO ds_ref_id
                FROM data_specification
                WHERE id = admin_eds_record.embedded_data_specification_id;
                
                DELETE FROM data_specification WHERE id = admin_eds_record.embedded_data_specification_id;
                
                IF ds_ref_id IS NOT NULL THEN
                    PERFORM delete_reference_recursively(ds_ref_id);
                END IF;
            END LOOP;
            
            -- Delete administrative_information (CASCADE will delete join table entries)
            DELETE FROM administrative_information WHERE id = OLD.administration_id;
            
            -- Delete creator reference
            IF admin_creator_id IS NOT NULL THEN
                PERFORM delete_reference_recursively(admin_creator_id);
            END IF;
        END;
    END IF;

    -- Clean up main submodel references
    IF OLD.semantic_id IS NOT NULL THEN
        PERFORM delete_reference_recursively(OLD.semantic_id);
    END IF;
    
    IF OLD.description_id IS NOT NULL THEN
        DELETE FROM lang_string_text_type_reference WHERE id = OLD.description_id;
    END IF;
    
    IF OLD.displayname_id IS NOT NULL THEN
        DELETE FROM lang_string_name_type_reference WHERE id = OLD.displayname_id;
    END IF;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Create trigger on submodel deletion
DROP TRIGGER IF EXISTS cleanup_submodel_data_trigger ON submodel;
CREATE TRIGGER cleanup_submodel_data_trigger
    AFTER DELETE ON submodel
    FOR EACH ROW
    EXECUTE FUNCTION cleanup_submodel_orphaned_data();

-- Note: With this trigger in place, the application-level cleanup in
-- PostgreSQLSubmodelDatabase.DeleteSubmodel() can be simplified to just:
-- DELETE FROM submodel WHERE id=$1
