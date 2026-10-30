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

package api

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/jsonization"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/stretchr/testify/require"
)

func metadataTestPropertyJSON(idShort string, valueType string, value string) map[string]any {
	property := map[string]any{"modelType": "Property", "valueType": valueType, "value": value}
	if idShort != "" {
		property["idShort"] = idShort
	}
	return property
}

func metadataTestListJSON(valueTypeListElement string) map[string]any {
	return map[string]any{
		"idShort":              "Readings",
		"modelType":            "SubmodelElementList",
		"orderRelevant":        true,
		"typeValueListElement": "Property",
		"valueTypeListElement": valueTypeListElement,
		"value":                []any{metadataTestPropertyJSON("", "xs:int", "1")},
	}
}

func newMetadataTestService(t *testing.T, verificationMode string) *SubmodelRepositoryAPIAPIService {
	t.Helper()
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	backend, err := persistencepostgresql.NewSubmodelDatabaseFromDB(db, nil, verificationMode)
	require.NoError(t, err)
	return &SubmodelRepositoryAPIAPIService{submodelBackend: *backend}
}

func TestToSubmodelElementMetadataKeepsOnlyMetadataFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		element  map[string]any
		expected []string
	}{
		{
			name:     "property",
			element:  metadataTestPropertyJSON("Temperature", "xs:int", "20"),
			expected: []string{"idShort", "modelType", "valueType"},
		},
		{
			name: "collection",
			element: map[string]any{
				"idShort":     "Motor",
				"modelType":   "SubmodelElementCollection",
				"description": []any{map[string]any{"language": "en", "text": "Motor"}},
				"value":       []any{metadataTestPropertyJSON("Temperature", "xs:int", "20")},
			},
			expected: []string{"idShort", "modelType", "description"},
		},
		{
			name:     "list",
			element:  metadataTestListJSON("xs:int"),
			expected: []string{"idShort", "modelType", "orderRelevant", "typeValueListElement", "valueTypeListElement"},
		},
		{
			name: "operation",
			element: map[string]any{
				"idShort":        "Calibrate",
				"modelType":      "Operation",
				"inputVariables": []any{map[string]any{"value": metadataTestPropertyJSON("Factor", "xs:double", "1.5")}},
			},
			expected: []string{"idShort", "modelType"},
		},
		{
			name: "entity",
			element: map[string]any{
				"idShort":       "Motor",
				"modelType":     "Entity",
				"entityType":    "SelfManagedEntity",
				"globalAssetId": "urn:example:asset:motor",
				"statements":    []any{metadataTestPropertyJSON("Speed", "xs:int", "1")},
			},
			expected: []string{"idShort", "modelType"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			element, err := jsonization.SubmodelElementFromJsonable(testCase.element)
			require.NoError(t, err)

			metadata, err := toSubmodelElementMetadata(element)
			require.NoError(t, err)

			fields := make([]string, 0, len(metadata))
			for field := range metadata {
				fields = append(fields, field)
			}
			require.ElementsMatch(t, testCase.expected, fields)
		})
	}
}

func TestValidateSubmodelElementMetadataPatchInStrictMode(t *testing.T) {
	t.Parallel()
	service := newMetadataTestService(t, "strict")

	testCases := []struct {
		name          string
		existing      map[string]any
		patch         map[string]any
		expectedError string
	}{
		{
			name:     "consistent valueType",
			existing: metadataTestPropertyJSON("Temperature", "xs:int", "20"),
			patch:    map[string]any{"modelType": "Property", "valueType": "xs:long"},
		},
		{
			name:          "valueType inconsistent with stored value",
			existing:      metadataTestPropertyJSON("Status", "xs:string", "running"),
			patch:         map[string]any{"modelType": "Property", "valueType": "xs:int"},
			expectedError: "SMREPO-PATCHSMEMETA-VERIFY",
		},
		{
			name:          "list value type conflicting with children",
			existing:      metadataTestListJSON("xs:int"),
			patch:         map[string]any{"modelType": "SubmodelElementList", "valueTypeListElement": "xs:string"},
			expectedError: "SMREPO-PATCHSMEMETA-VERIFY",
		},
		{
			name:          "modelType mismatch",
			existing:      metadataTestPropertyJSON("Temperature", "xs:int", "20"),
			patch:         map[string]any{"modelType": "Range"},
			expectedError: "SMREPO-PATCHSMEMETA-MODELTYPEMISMATCH",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := service.validateSubmodelElementMetadataPatch(testCase.existing, testCase.patch)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.expectedError)
		})
	}
}

func TestValidateSubmodelElementMetadataPatchHonorsDisabledVerification(t *testing.T) {
	t.Parallel()
	service := newMetadataTestService(t, "off")

	err := service.validateSubmodelElementMetadataPatch(
		metadataTestPropertyJSON("Status", "xs:string", "running"),
		map[string]any{"modelType": "Property", "valueType": "xs:int"},
	)
	require.NoError(t, err)
}
