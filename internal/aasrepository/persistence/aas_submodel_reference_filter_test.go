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
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
* IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
* CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
* TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
* SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package persistence

import (
	"context"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestReadSubmodelReferencePayloadsAppliesAutomaticRowMatching(t *testing.T) {
	var actualSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
		actualSQL = actual
		return nil
	})))
	require.NoError(t, err)
	defer func() {
		_ = db.Close()
	}()

	rows := sqlmock.NewRows([]string{
		"owner_id",
		"ref_id",
		"ref_type",
		"key_id",
		"key_type",
		"key_value",
		"parent_reference_payload",
	}).AddRow(
		int64(42),
		int64(100),
		int64(types.ReferenceTypesModelReference),
		int64(101),
		int64(types.KeyTypesSubmodel),
		"urn:example:submodel:visible",
		[]byte(`{
			"type": "ModelReference",
			"keys": [
				{"type": "Submodel", "value": "urn:example:submodel:visible"},
				{"type": "Submodel", "value": "urn:example:submodel:hidden"}
			],
			"referredSemanticId": {
				"type": "ExternalReference",
				"keys": [
					{"type": "GlobalReference", "value": "urn:example:semantic:submodel"}
				]
			}
		}`),
	)
	mock.ExpectQuery("read filtered submodel references").WillReturnRows(rows)

	sut := &AssetAdministrationShellDatabase{}
	referencesByAASID, readErr := sut.readSubmodelReferencePayloadsByAASDBIDs(submodelReferenceFilterContext(t), db, []int64{42})
	require.NoError(t, readErr)
	require.Len(t, referencesByAASID[42], 1)
	filteredReference := referencesByAASID[42][0]
	require.Len(t, filteredReference.Keys(), 1)
	require.Equal(t, "urn:example:submodel:visible", filteredReference.Keys()[0].Value())
	require.NotNil(t, filteredReference.ReferredSemanticID())
	require.Len(t, filteredReference.ReferredSemanticID().Keys(), 1)
	require.Equal(t, "urn:example:semantic:submodel", filteredReference.ReferredSemanticID().Keys()[0].Value())
	require.Contains(t, actualSQL, `FROM "aas_submodel_reference" AS "aas_submodel_reference"`)
	require.Contains(t, actualSQL, `JOIN "aas" AS "aas"`)
	require.Contains(t, actualSQL, `aas_submodel_reference_key`)
	require.Contains(t, strings.ToUpper(actualSQL), "EXISTS")
	require.Contains(t, actualSQL, `"aas_submodel_reference"."id"`)
	require.Contains(t, actualSQL, `"aas__exists"."aas_id"`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func submodelReferenceFilterContext(t *testing.T) context.Context {
	t.Helper()

	aasIdentifier := grammar.StandardString("urn:example:aas:public")
	visibleReference := grammar.StandardString("urn:example:submodel:visible")
	aasIDField := grammar.ModelStringPattern("$aas#id")
	submodelReferenceKeyValueField := grammar.ModelStringPattern("$aas#submodels[].keys[].value")
	return auth.WithQueryFilter(aasSigningTestContext(t), &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$aas#submodels[]": {
				And: []grammar.LogicalExpression{
					{Eq: grammar.ComparisonItems{grammar.Value{Field: &aasIDField}, grammar.Value{StrVal: &aasIdentifier}}},
					{Eq: grammar.ComparisonItems{grammar.Value{Field: &submodelReferenceKeyValueField}, grammar.Value{StrVal: &visibleReference}}},
				},
			},
		},
	})
}
