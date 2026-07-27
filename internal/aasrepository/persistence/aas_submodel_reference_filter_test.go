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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestReadSubmodelReferencePayloadsAppliesSubmodelReferenceFilterMatchModes(t *testing.T) {
	tests := []struct {
		name           string
		match          bool
		payloads       []string
		containsExists bool
		aasIDColumn    string
	}{
		{
			name:           "match true filters the current reference row",
			match:          true,
			payloads:       []string{referencePayload("urn:example:submodel:visible")},
			containsExists: false,
			aasIDColumn:    `"aas"."aas_id"`,
		},
		{
			name:           "without match uses the AAS scoped predicate",
			payloads:       []string{referencePayload("urn:example:submodel:visible"), referencePayload("urn:example:submodel:hidden")},
			containsExists: true,
			aasIDColumn:    `"aas__exists"."aas_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actualSQL string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(_ string, actual string) error {
				actualSQL = actual
				return nil
			})))
			require.NoError(t, err)
			defer func() {
				_ = db.Close()
			}()

			rows := sqlmock.NewRows([]string{"aas_id", "parent_reference_payload"})
			for _, payload := range tt.payloads {
				rows.AddRow(int64(42), payload)
			}
			mock.ExpectQuery("read filtered submodel references").WillReturnRows(rows)

			sut := &AssetAdministrationShellDatabase{}
			referencesByAASID, readErr := sut.readSubmodelReferencePayloadsByAASDBIDs(submodelReferenceFilterContext(t, tt.match), db, []int64{42})
			require.NoError(t, readErr)
			require.Len(t, referencesByAASID[42], len(tt.payloads))
			require.Contains(t, actualSQL, `FROM "aas_submodel_reference" AS "aas_submodel_reference"`)
			require.Contains(t, actualSQL, `INNER JOIN "aas" AS "aas"`)
			require.Contains(t, actualSQL, tt.aasIDColumn)
			require.Contains(t, actualSQL, `aas_submodel_reference_key`)
			if tt.containsExists {
				require.Contains(t, strings.ToUpper(actualSQL), "EXISTS")
			} else {
				require.NotContains(t, strings.ToUpper(actualSQL), "EXISTS")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func submodelReferenceFilterContext(t *testing.T, match bool) context.Context {
	t.Helper()

	aasIdentifier := grammar.StandardString("urn:example:aas:public")
	visibleReference := grammar.StandardString("urn:example:submodel:visible")
	aasIDField := grammar.ModelStringPattern("$aas#id")
	submodelReferenceKeyValueField := grammar.ModelStringPattern("$aas#submodels[].keys[].value")
	queryFilter := &auth.QueryFilter{
		Filters: auth.FragmentFilters{
			"$aas#submodels[]": {
				And: []grammar.LogicalExpression{
					{Eq: grammar.ComparisonItems{grammar.Value{Field: &aasIDField}, grammar.Value{StrVal: &aasIdentifier}}},
					{Eq: grammar.ComparisonItems{grammar.Value{Field: &submodelReferenceKeyValueField}, grammar.Value{StrVal: &visibleReference}}},
				},
			},
		},
	}
	if match {
		queryFilter.FilterMatch = auth.FragmentMatchModes{"$aas#submodels[]": true}
	}

	return auth.WithQueryFilter(aasSigningTestContext(t), queryFilter)
}

func referencePayload(identifier string) string {
	return `{"type":"ModelReference","keys":[{"type":"Submodel","value":"` + identifier + `"}]}`
}
