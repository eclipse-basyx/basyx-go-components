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

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubmodelDeletionCleansUpAllData verifies that deleting a submodel
// removes all related data from the database, including:
// - administrative_information records
// - reference records  
// - lang_string_text_type_reference records (descriptions)
// - lang_string_name_type_reference records (display names)
// - extension records
// - qualifier records
// - submodel_element records and their associated data
func TestSubmodelDeletionCleansUpAllData(t *testing.T) {
	// Wait for services to be ready
	time.Sleep(15 * time.Second)

	// Connect to database
	db, err := sql.Open("postgres", "postgres://admin:admin123@127.0.0.1:5432/basyxTestDB?sslmode=disable")
	require.NoError(t, err, "Failed to connect to database")
	defer db.Close()

	// Define a complex submodel with all possible nested structures
	submodelJSON := `{
		"modelType": "Submodel",
		"id": "http://test.example.com/submodel/test-cleanup",
		"idShort": "TestCleanupSubmodel",
		"kind": "Instance",
		"semanticId": {
			"type": "ModelReference",
			"keys": [{
				"type": "Submodel",
				"value": "http://example.com/semantic/test"
			}]
		},
		"description": [{
			"language": "en",
			"text": "Test submodel for cleanup verification"
		}],
		"displayName": [{
			"language": "en",
			"text": "Test Cleanup Submodel"
		}],
		"administration": {
			"version": "1.0",
			"revision": "0"
		},
		"qualifier": [{
			"type": "testQualifier",
			"valueType": "xs:string",
			"value": "testValue",
			"semanticId": {
				"type": "ModelReference",
				"keys": [{
					"type": "Property",
					"value": "http://example.com/qualifier/semantic"
				}]
			}
		}],
		"extension": [{
			"name": "testExtension",
			"valueType": "xs:string",
			"value": "extensionValue",
			"semanticId": {
				"type": "ExternalReference",
				"keys": [{
					"type": "GlobalReference",
					"value": "http://example.com/extension/semantic"
				}]
			}
		}],
		"submodelElements": [{
			"idShort": "testProperty",
			"modelType": "Property",
			"valueType": "xs:string",
			"value": "testValue",
			"semanticId": {
				"type": "ModelReference",
				"keys": [{
					"type": "Property",
					"value": "http://example.com/property/semantic"
				}]
			},
			"description": [{
				"language": "en",
				"text": "Test property description"
			}],
			"displayName": [{
				"language": "en",
				"text": "Test Property"
			}]
		}]
	}`

	// Count records before creation
	countsBefore := getRecordCounts(t, db)
	t.Logf("Record counts before creation: %+v", countsBefore)

	// Create the submodel via API
	req, err := http.NewRequest("POST", "http://localhost:5004/submodels", bytes.NewBufferString(submodelJSON))
	require.NoError(t, err, "Failed to create POST request")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to execute POST request")
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "Submodel creation should return 201")

	// Wait a bit for data to be written
	time.Sleep(2 * time.Second)

	// Count records after creation
	countsAfterCreate := getRecordCounts(t, db)
	t.Logf("Record counts after creation: %+v", countsAfterCreate)

	// Verify that records were created
	assert.Greater(t, countsAfterCreate.Submodels, countsBefore.Submodels, "Submodel should be created")
	assert.Greater(t, countsAfterCreate.References, countsBefore.References, "References should be created")
	assert.Greater(t, countsAfterCreate.LangStringTextRefs, countsBefore.LangStringTextRefs, "LangStringTextRefs should be created")
	assert.Greater(t, countsAfterCreate.LangStringNameRefs, countsBefore.LangStringNameRefs, "LangStringNameRefs should be created")
	assert.Greater(t, countsAfterCreate.AdministrativeInfo, countsBefore.AdministrativeInfo, "AdministrativeInfo should be created")
	assert.Greater(t, countsAfterCreate.Extensions, countsBefore.Extensions, "Extensions should be created")
	assert.Greater(t, countsAfterCreate.Qualifiers, countsBefore.Qualifiers, "Qualifiers should be created")
	assert.Greater(t, countsAfterCreate.SubmodelElements, countsBefore.SubmodelElements, "SubmodelElements should be created")

	// Get the specific record IDs for this submodel to verify they're deleted
	recordIDs := getSubmodelRecordIDs(t, db, "http://test.example.com/submodel/test-cleanup")
	t.Logf("Record IDs for test submodel: %+v", recordIDs)

	// Delete the submodel via API
	deleteReq, err := http.NewRequest("DELETE", "http://localhost:5004/submodels/aHR0cDovL3Rlc3QuZXhhbXBsZS5jb20vc3VibW9kZWwvdGVzdC1jbGVhbnVw", nil)
	require.NoError(t, err, "Failed to create DELETE request")

	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err, "Failed to execute DELETE request")
	defer deleteResp.Body.Close()

	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode, "Submodel deletion should return 204")

	// Wait a bit for triggers to execute
	time.Sleep(2 * time.Second)

	// Count records after deletion
	countsAfterDelete := getRecordCounts(t, db)
	t.Logf("Record counts after deletion: %+v", countsAfterDelete)

	// Verify the submodel is deleted
	assert.Equal(t, countsBefore.Submodels, countsAfterDelete.Submodels, "Submodel count should return to original")

	// Verify all related records are cleaned up
	verifyRecordsDeleted(t, db, recordIDs)

	// The counts should be back to the original or very close
	// (allowing for some shared references that might still exist)
	assert.LessOrEqual(t, countsAfterDelete.References, countsAfterCreate.References, 
		"References should be cleaned up")
	assert.LessOrEqual(t, countsAfterDelete.LangStringTextRefs, countsAfterCreate.LangStringTextRefs,
		"LangStringTextRefs should be cleaned up")
	assert.LessOrEqual(t, countsAfterDelete.LangStringNameRefs, countsAfterCreate.LangStringNameRefs,
		"LangStringNameRefs should be cleaned up")
	assert.Equal(t, countsBefore.AdministrativeInfo, countsAfterDelete.AdministrativeInfo,
		"AdministrativeInfo should be cleaned up")
	assert.Equal(t, countsBefore.Extensions, countsAfterDelete.Extensions,
		"Extensions should be cleaned up")
	assert.Equal(t, countsBefore.Qualifiers, countsAfterDelete.Qualifiers,
		"Qualifiers should be cleaned up")
	assert.Equal(t, countsBefore.SubmodelElements, countsAfterDelete.SubmodelElements,
		"SubmodelElements should be cleaned up")
}

// RecordCounts holds the count of various record types
type RecordCounts struct {
	Submodels          int
	References         int
	ReferenceKeys      int
	LangStringTextRefs int
	LangStringNameRefs int
	AdministrativeInfo int
	Extensions         int
	Qualifiers         int
	SubmodelElements   int
}

// getRecordCounts returns the count of various record types
func getRecordCounts(t *testing.T, db *sql.DB) RecordCounts {
	var counts RecordCounts

	err := db.QueryRow("SELECT COUNT(*) FROM submodel").Scan(&counts.Submodels)
	require.NoError(t, err, "Failed to count submodels")

	err = db.QueryRow("SELECT COUNT(*) FROM reference").Scan(&counts.References)
	require.NoError(t, err, "Failed to count references")

	err = db.QueryRow("SELECT COUNT(*) FROM reference_key").Scan(&counts.ReferenceKeys)
	require.NoError(t, err, "Failed to count reference_keys")

	err = db.QueryRow("SELECT COUNT(*) FROM lang_string_text_type_reference").Scan(&counts.LangStringTextRefs)
	require.NoError(t, err, "Failed to count lang_string_text_type_reference")

	err = db.QueryRow("SELECT COUNT(*) FROM lang_string_name_type_reference").Scan(&counts.LangStringNameRefs)
	require.NoError(t, err, "Failed to count lang_string_name_type_reference")

	err = db.QueryRow("SELECT COUNT(*) FROM administrative_information").Scan(&counts.AdministrativeInfo)
	require.NoError(t, err, "Failed to count administrative_information")

	err = db.QueryRow("SELECT COUNT(*) FROM extension").Scan(&counts.Extensions)
	require.NoError(t, err, "Failed to count extensions")

	err = db.QueryRow("SELECT COUNT(*) FROM qualifier").Scan(&counts.Qualifiers)
	require.NoError(t, err, "Failed to count qualifiers")

	err = db.QueryRow("SELECT COUNT(*) FROM submodel_element").Scan(&counts.SubmodelElements)
	require.NoError(t, err, "Failed to count submodel_elements")

	return counts
}

// SubmodelRecordIDs holds the IDs of records associated with a specific submodel
type SubmodelRecordIDs struct {
	SemanticID      sql.NullInt64
	DescriptionID   sql.NullInt64
	DisplayNameID   sql.NullInt64
	AdministrationID sql.NullInt64
	ExtensionIDs    []int64
	QualifierIDs    []int64
	ElementIDs      []int64
}

// getSubmodelRecordIDs retrieves the IDs of all records associated with a submodel
func getSubmodelRecordIDs(t *testing.T, db *sql.DB, submodelID string) SubmodelRecordIDs {
	var ids SubmodelRecordIDs

	// Get main submodel foreign key IDs
	err := db.QueryRow(`
		SELECT semantic_id, description_id, displayname_id, administration_id
		FROM submodel WHERE id = $1
	`, submodelID).Scan(&ids.SemanticID, &ids.DescriptionID, &ids.DisplayNameID, &ids.AdministrationID)
	require.NoError(t, err, "Failed to query submodel record IDs")

	// Get extension IDs
	rows, err := db.Query(`
		SELECT extension_id FROM submodel_extension WHERE submodel_id = $1
	`, submodelID)
	require.NoError(t, err, "Failed to query extension IDs")
	defer rows.Close()
	for rows.Next() {
		var extID int64
		err := rows.Scan(&extID)
		require.NoError(t, err, "Failed to scan extension ID")
		ids.ExtensionIDs = append(ids.ExtensionIDs, extID)
	}

	// Get qualifier IDs
	rows, err = db.Query(`
		SELECT qualifier_id FROM submodel_qualifier WHERE submodel_id = $1
	`, submodelID)
	require.NoError(t, err, "Failed to query qualifier IDs")
	defer rows.Close()
	for rows.Next() {
		var qualID int64
		err := rows.Scan(&qualID)
		require.NoError(t, err, "Failed to scan qualifier ID")
		ids.QualifierIDs = append(ids.QualifierIDs, qualID)
	}

	// Get submodel element IDs
	rows, err = db.Query(`
		SELECT id FROM submodel_element WHERE submodel_id = $1
	`, submodelID)
	require.NoError(t, err, "Failed to query submodel element IDs")
	defer rows.Close()
	for rows.Next() {
		var elemID int64
		err := rows.Scan(&elemID)
		require.NoError(t, err, "Failed to scan element ID")
		ids.ElementIDs = append(ids.ElementIDs, elemID)
	}

	return ids
}

// verifyRecordsDeleted checks that all records associated with the submodel are deleted
func verifyRecordsDeleted(t *testing.T, db *sql.DB, ids SubmodelRecordIDs) {
	// Check semantic_id reference
	if ids.SemanticID.Valid {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM reference WHERE id = $1", ids.SemanticID.Int64).Scan(&count)
		require.NoError(t, err, "Failed to check reference existence")
		assert.Equal(t, 0, count, fmt.Sprintf("Reference %d should be deleted", ids.SemanticID.Int64))
	}

	// Check description_id
	if ids.DescriptionID.Valid {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM lang_string_text_type_reference WHERE id = $1", ids.DescriptionID.Int64).Scan(&count)
		require.NoError(t, err, "Failed to check lang_string_text_type_reference existence")
		assert.Equal(t, 0, count, fmt.Sprintf("LangStringTextTypeReference %d should be deleted", ids.DescriptionID.Int64))
	}

	// Check displayname_id
	if ids.DisplayNameID.Valid {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM lang_string_name_type_reference WHERE id = $1", ids.DisplayNameID.Int64).Scan(&count)
		require.NoError(t, err, "Failed to check lang_string_name_type_reference existence")
		assert.Equal(t, 0, count, fmt.Sprintf("LangStringNameTypeReference %d should be deleted", ids.DisplayNameID.Int64))
	}

	// Check administration_id
	if ids.AdministrationID.Valid {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM administrative_information WHERE id = $1", ids.AdministrationID.Int64).Scan(&count)
		require.NoError(t, err, "Failed to check administrative_information existence")
		assert.Equal(t, 0, count, fmt.Sprintf("AdministrativeInformation %d should be deleted", ids.AdministrationID.Int64))
	}

	// Check extensions
	for _, extID := range ids.ExtensionIDs {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM extension WHERE id = $1", extID).Scan(&count)
		require.NoError(t, err, "Failed to check extension existence")
		assert.Equal(t, 0, count, fmt.Sprintf("Extension %d should be deleted", extID))
	}

	// Check qualifiers
	for _, qualID := range ids.QualifierIDs {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM qualifier WHERE id = $1", qualID).Scan(&count)
		require.NoError(t, err, "Failed to check qualifier existence")
		assert.Equal(t, 0, count, fmt.Sprintf("Qualifier %d should be deleted", qualID))
	}

	// Check submodel elements
	for _, elemID := range ids.ElementIDs {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM submodel_element WHERE id = $1", elemID).Scan(&count)
		require.NoError(t, err, "Failed to check submodel_element existence")
		assert.Equal(t, 0, count, fmt.Sprintf("SubmodelElement %d should be deleted", elemID))
	}
}

// Helper function to pretty print JSON for debugging
func prettyPrintJSON(data interface{}) string {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling: %v", err)
	}
	return string(b)
}
