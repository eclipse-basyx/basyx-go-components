/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package submodelelements

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestMetadataDescriptionPatchDoesNotReadOrWriteValues(t *testing.T) {
	for _, modelType := range []string{
		"Property", "Range", "Blob", "File", "ReferenceElement", "RelationshipElement",
		"AnnotatedRelationshipElement", "MultiLanguageProperty", "Entity", "Operation",
		"BasicEventElement", "SubmodelElementList", "SubmodelElementCollection", "Capability",
	} {
		t.Run(modelType, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			patch := map[string]any{
				"modelType":   modelType,
				"description": []any{map[string]any{"language": "en", "text": "Updated"}},
			}
			metadata, err := common.ParseSubmodelElementMetadataPayload(patch)
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO \"submodel_element_payload\" \\(\"description_payload\",\"submodel_element_id\"\\) VALUES \\(\\$1::jsonb,\\$2\\) ON CONFLICT \\(submodel_element_id\\) DO UPDATE SET \"description_payload\"=\\$3::jsonb").
				WithArgs("[{\"language\":\"en\",\"text\":\"Updated\"}]", int64(42), "[{\"language\":\"en\",\"text\":\"Updated\"}]").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			tx, err := db.Begin()
			require.NoError(t, err)
			require.NoError(t, ApplyMetadataPatchTx(t.Context(), tx, MetadataTarget{ID: 42, ModelType: metadata.ModelType}, patch, nil))
			require.NoError(t, tx.Commit())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMetadataCategoryPatchDistinguishesOmissionAndNull(t *testing.T) {
	for _, value := range []any{nil, ""} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		patch := map[string]any{"modelType": "Property", "category": value}
		metadata, err := common.ParseSubmodelElementMetadataPayload(patch)
		require.NoError(t, err)
		mock.ExpectBegin()
		if value == nil {
			mock.ExpectExec("UPDATE \"submodel_element\" SET \"category\"=NULL WHERE \\(\"id\" = \\$1\\)").
				WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
		} else {
			mock.ExpectExec("UPDATE \"submodel_element\" SET \"category\"=\\$1 WHERE \\(\"id\" = \\$2\\)").
				WithArgs(value, int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		mock.ExpectCommit()
		tx, err := db.Begin()
		require.NoError(t, err)
		require.NoError(t, ApplyMetadataPatchTx(t.Context(), tx, MetadataTarget{ID: 42, ModelType: metadata.ModelType}, patch, nil))
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
		require.NoError(t, db.Close())
	}
}

func TestMetadataTypePatchDoesNotIncludeOtherFields(t *testing.T) {
	table, record, err := buildMetadataTypeRecord(types.ModelTypeBasicEventElement, map[string]any{"state": "off"}, nil)
	require.NoError(t, err)
	require.Equal(t, "basic_event_element", table)
	require.Equal(t, goqu.Record{"state": types.StateOfEventOff}, record)

	table, record, err = buildMetadataTypeRecord(types.ModelTypeSubmodelElementList, map[string]any{"orderRelevant": false}, nil)
	require.NoError(t, err)
	require.Equal(t, "submodel_element_list", table)
	require.Equal(t, goqu.Record{"order_relevant": false}, record)
}

func TestMetadataValueTypeErrorsDoNotDiscloseStoredValues(t *testing.T) {
	property := types.NewProperty(types.DataTypeDefXSDString)
	property.SetValue(new("hidden-not-a-number"))
	_, _, err := buildMetadataValueTypeRecord(property, "xs:int")
	require.Error(t, err)
	require.True(t, common.IsErrBadRequest(err))
	require.NotContains(t, err.Error(), "hidden-not-a-number")
}

func TestMetadataValueTypePatchConvertsCurrentPropertyValue(t *testing.T) {
	property := types.NewProperty(types.DataTypeDefXSDString)
	property.SetValue(new("17"))
	table, record, err := buildMetadataValueTypeRecord(property, "xs:int")
	require.NoError(t, err)
	require.Equal(t, "property_element", table)
	require.Equal(t, types.DataTypeDefXSDInt, record["value_type"])
	require.Contains(t, record, "value_num")
	require.Equal(t, "17", *property.Value())
}

func TestMetadataReferencePatchPreservesOmittedNestedFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	nested := map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{"type": "GlobalReference", "value": "urn:nested"}},
	}
	previous := map[string]any{
		"type":               "ExternalReference",
		"keys":               []any{map[string]any{"type": "GlobalReference", "value": "urn:old"}},
		"referredSemanticId": nested,
	}
	payload, err := json.Marshal(previous)
	require.NoError(t, err)
	reference := map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{"type": "GlobalReference", "value": "urn:new"}},
	}
	patch := map[string]any{"modelType": "Property", "semanticId": reference}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \"parent_reference_payload\" FROM \"submodel_element_semantic_id_reference_payload\"").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_reference_payload"}).AddRow(string(payload)))
	mock.ExpectRollback()
	tx, err := db.Begin()
	require.NoError(t, err)
	merged, err := mergeMetadataReferencePatchesTx(t.Context(), tx, 42, patch)
	require.NoError(t, err)
	actual := merged["semanticId"].(map[string]any)
	require.Equal(t, reference["keys"], actual["keys"])
	require.Equal(t, nested, actual["referredSemanticId"])
	require.NotContains(t, reference, "referredSemanticId")
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}
