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

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestAASRegistryListMasksMultiKeyReferencesWithoutDuplicates(t *testing.T) {
	deleteAllAASDescriptorsHTTP(t)
	t.Cleanup(func() {
		deleteAllAASDescriptorsHTTP(t)
	})
	descriptorID := fmt.Sprintf("urn:example:aas:multi-key-mask:%d", time.Now().UnixNano())
	createAASDescriptor(t, multiKeyMaskDescriptorPayload(descriptorID), http.StatusCreated)

	db, err := sql.Open("pgx", aasRegistryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	specificAssetNameFragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[].name")
	specificAssetKeyField := grammar.ModelStringPattern("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value")
	submodelSemanticFragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[].semanticId")
	submodelSemanticKeyField := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].semanticId.keys[].value")
	ctx := auth.MergeQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), grammar.Query{
		FilterConditions: []grammar.SubFilter{
			{Fragment: &specificAssetNameFragment, Condition: logicalExpressionPointer(descriptorFieldEquals(specificAssetKeyField, "MASK_MATCH"))},
			{Fragment: &submodelSemanticFragment, Condition: logicalExpressionPointer(descriptorFieldEquals(submodelSemanticKeyField, "MASK_MATCH"))},
		},
	})

	result, _, err := descriptors.ListAssetAdministrationShellDescriptors(
		ctx,
		db,
		100,
		"",
		model.AssetKind(""),
		"",
		descriptorID,
		time.Time{},
		time.Time{},
	)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].SpecificAssetIds, 1)
	require.Equal(t, "serialNumber", result[0].SpecificAssetIds[0].Name())
	require.Len(t, result[0].SpecificAssetIds[0].ExternalSubjectID().Keys(), 2)
	require.Len(t, result[0].SubmodelDescriptors, 1)
	require.NotNil(t, result[0].SubmodelDescriptors[0].SemanticId)
	require.Len(t, result[0].SubmodelDescriptors[0].SemanticId.Keys(), 2)
}

func TestNestedSubmodelDescriptorChildFiltersPreserveAASSiblings(t *testing.T) {
	deleteAllAASDescriptorsHTTP(t)
	t.Cleanup(func() {
		deleteAllAASDescriptorsHTTP(t)
	})
	descriptorID := fmt.Sprintf("urn:example:aas:nested-child-filter:%d", time.Now().UnixNano())
	createAASDescriptor(t, nestedChildFilterDescriptorPayload(descriptorID), http.StatusCreated)

	db, err := sql.Open("pgx", aasRegistryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	dialect := goqu.Dialect(common.Dialect)
	lookup := dialect.From(common.TAASDescriptor).
		Select(common.TAASDescriptor.Col(common.ColDescriptorID)).
		Where(common.TAASDescriptor.Col(common.ColID).Eq(descriptorID)).
		Prepared(true)
	lookupSQL, lookupArgs, err := lookup.ToSQL()
	require.NoError(t, err)
	var aasDescriptorID int64
	require.NoError(t, db.QueryRowContext(t.Context(), lookupSQL, lookupArgs...).Scan(&aasDescriptorID))

	tests := []struct {
		name     string
		fragment grammar.FragmentStringPattern
		assert   func(*testing.T, model.SubmodelDescriptor, bool)
	}{
		{
			name:     "endpoints",
			fragment: "$aasdesc#submodelDescriptors[].endpoints[]",
			assert: func(t *testing.T, descriptor model.SubmodelDescriptor, visible bool) {
				t.Helper()
				if visible {
					require.Len(t, descriptor.Endpoints, 1)
					return
				}
				require.Empty(t, descriptor.Endpoints)
			},
		},
		{
			name:     "semantic ID keys",
			fragment: "$aasdesc#submodelDescriptors[].semanticId.keys[]",
			assert: func(t *testing.T, descriptor model.SubmodelDescriptor, visible bool) {
				t.Helper()
				if visible {
					require.NotNil(t, descriptor.SemanticId)
					require.Len(t, descriptor.SemanticId.Keys(), 2)
					return
				}
				require.Nil(t, descriptor.SemanticId)
			},
		},
		{
			name:     "supplemental semantic IDs",
			fragment: "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
			assert: func(t *testing.T, descriptor model.SubmodelDescriptor, visible bool) {
				t.Helper()
				if visible {
					require.Len(t, descriptor.SupplementalSemanticId, 1)
					return
				}
				require.Empty(t, descriptor.SupplementalSemanticId)
			},
		},
		{
			name:     "supplemental semantic ID keys",
			fragment: "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]",
			assert: func(t *testing.T, descriptor model.SubmodelDescriptor, visible bool) {
				t.Helper()
				if visible {
					require.Len(t, descriptor.SupplementalSemanticId, 1)
					require.Len(t, descriptor.SupplementalSemanticId[0].Keys(), 2)
					return
				}
				require.Empty(t, descriptor.SupplementalSemanticId)
			},
		},
	}

	for _, test := range tests {
		for _, match := range []bool{false, true} {
			name := "non-MATCH"
			if match {
				name = "MATCH"
			}
			t.Run(test.name+"/"+name, func(t *testing.T) {
				field := grammar.ModelStringPattern("$aasdesc#submodelDescriptors[].idShort")
				ctx := auth.MergeQueryFilter(common.ContextWithConfig(t.Context(), &common.Config{}), grammar.Query{
					FilterConditions: []grammar.SubFilter{{
						Fragment:  &test.fragment,
						Condition: logicalExpressionPointer(descriptorFieldEquals(field, "matching-child")),
						Match:     &match,
					}},
				})

				result, err := descriptors.ReadSubmodelDescriptorsByAASDescriptorIDs(
					ctx,
					db,
					[]int64{aasDescriptorID},
					false,
				)
				require.NoError(t, err)
				require.Len(t, result[aasDescriptorID], 2)
				for _, descriptor := range result[aasDescriptorID] {
					visible := descriptor.IdShort == "matching-child" || !match
					test.assert(t, descriptor, visible)
				}
			})
		}
	}
}

func multiKeyMaskDescriptorPayload(descriptorID string) map[string]any {
	return map[string]any{
		"id": descriptorID,
		"specificAssetIds": []any{
			map[string]any{
				"name":              "serialNumber",
				"value":             "SN-1000",
				"externalSubjectId": multiKeyMaskReference(),
			},
		},
		"submodelDescriptors": []any{
			map[string]any{
				"id":         descriptorID + ":submodel",
				"semanticId": multiKeyMaskReference(),
				"endpoints": []any{
					map[string]any{
						"interface": "AAS-3.0",
						"protocolInformation": map[string]any{
							"href": "https://example.com/submodels/multi-key-mask",
						},
					},
				},
			},
		},
	}
}

func nestedChildFilterDescriptorPayload(descriptorID string) map[string]any {
	endpoint := func(id string) map[string]any {
		return map[string]any{
			"interface": "SUBMODEL-3.0",
			"protocolInformation": map[string]any{
				"href":             "https://example.com/submodels/" + id,
				"endpointProtocol": "https",
			},
		}
	}
	submodelDescriptor := func(id string, idShort string) map[string]any {
		return map[string]any{
			"id":                      descriptorID + ":" + id,
			"idShort":                 idShort,
			"endpoints":               []any{endpoint(id)},
			"semanticId":              multiKeyMaskReference(),
			"supplementalSemanticIds": []any{multiKeyMaskReference()},
		}
	}
	return map[string]any{
		"id":        descriptorID,
		"assetKind": "Instance",
		"endpoints": []any{endpoint("aas")},
		"submodelDescriptors": []any{
			submodelDescriptor("matching", "matching-child"),
			submodelDescriptor("sibling", "preserved-sibling"),
		},
	}
}

func multiKeyMaskReference() map[string]any {
	return map[string]any{
		"type": "ExternalReference",
		"keys": []any{
			map[string]any{"type": "GlobalReference", "value": "MASK_MATCH"},
			map[string]any{"type": "GlobalReference", "value": "MASK_MISS"},
		},
	}
}

func descriptorFieldEquals(field grammar.ModelStringPattern, value string) grammar.LogicalExpression {
	literal := grammar.StandardString(value)
	return grammar.LogicalExpression{
		Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &literal},
		},
	}
}

func logicalExpressionPointer(expression grammar.LogicalExpression) *grammar.LogicalExpression {
	return &expression
}
