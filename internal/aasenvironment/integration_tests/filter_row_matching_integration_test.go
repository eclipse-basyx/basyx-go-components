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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type rowMatchingIntegrationFixture struct {
	aasID                  string
	aasDescriptorIDShort   string
	submodelID             string
	submodelDescriptorName string
	simpleSMEsubmodelID    string
	nestedSMEsubmodelID    string
}

type rowMatchingIntegrationCase struct {
	name                    string
	endpoint                string
	rootField               string
	rootValue               string
	fragment                string
	condition               map[string]any
	resultPath              string
	expectedCandidateValues []string
	expectedValues          []string
	explanation             string
}

func TestAutomaticArrayFragmentRowMatchingThroughProductionRepositories(t *testing.T) {
	resetDatabase(t)
	fixture := createRowMatchingIntegrationFixture(t)

	tests := append(
		aasRowMatchingIntegrationCases(fixture),
		descriptorRowMatchingIntegrationCases(fixture)...,
	)
	tests = append(tests, submodelRowMatchingIntegrationCases(fixture)...)
	tests = append(tests, smeRowMatchingIntegrationCases(fixture)...)
	tests = append(tests, nonRowMatchingIntegrationCases(fixture)...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("fragment: %s", tt.fragment)
			t.Logf("candidate data path: %s", tt.resultPath)
			t.Logf("expected unfiltered candidates: %v", tt.expectedCandidateValues)
			t.Logf("expected filtered result: %v", tt.expectedValues)
			t.Log(tt.explanation)

			unfilteredResource := querySingleRowMatchingResource(t, tt, false)
			actualCandidateValues := collectStringValues(unfilteredResource, tt.resultPath)
			expectedCandidateValues := append([]string{}, tt.expectedCandidateValues...)
			sort.Strings(actualCandidateValues)
			sort.Strings(expectedCandidateValues)
			t.Logf("unfiltered candidates: %v", actualCandidateValues)

			require.Equalf(
				t,
				expectedCandidateValues,
				actualCandidateValues,
				"sanity query without $filters must return every candidate at %s; resource=%v",
				tt.resultPath,
				unfilteredResource,
			)

			resource := querySingleRowMatchingResource(t, tt, true)
			actualValues := collectStringValues(resource, tt.resultPath)
			expectedValues := append([]string{}, tt.expectedValues...)
			sort.Strings(actualValues)
			sort.Strings(expectedValues)
			t.Logf("filtered result: %v", actualValues)

			require.Equalf(
				t,
				expectedValues,
				actualValues,
				"%s; fragment=%s condition=%v resource=%v",
				tt.explanation,
				tt.fragment,
				tt.condition,
				resource,
			)
		})
	}
}

func TestNestedAndStandaloneEndpointFiltersRemainIndependent(t *testing.T) {
	resetDatabase(t)
	fixture := createRowMatchingIntegrationFixture(t)

	testCase := rowMatchingIntegrationCase{
		endpoint:   "/query/submodel-descriptors",
		rootField:  "$smdesc#idShort",
		rootValue:  fixture.submodelDescriptorName,
		resultPath: "endpoints[].protocolInformation.href",
		expectedCandidateValues: []string{
			"https://admin.example/standalone-submodel",
			"https://public.example/standalone-submodel",
		},
	}
	unfilteredResource := querySingleRowMatchingResource(t, testCase, false)
	require.ElementsMatch(
		t,
		testCase.expectedCandidateValues,
		collectStringValues(unfilteredResource, testCase.resultPath),
		"the fixture must contain one public and one internal endpoint",
	)

	condition := equalStringCondition(
		"$smdesc#endpoints[].protocolinformation.href",
		"https://public.example/standalone-submodel",
	)
	resource := querySingleRowMatchingResourceWithFilters(t, testCase, []any{
		map[string]any{
			"$fragment":  "$aasdesc#submodelDescriptors[].endpoints[]",
			"$condition": condition,
		},
		map[string]any{
			"$fragment":  "$smdesc#endpoints[]",
			"$condition": condition,
		},
	})

	require.ElementsMatch(
		t,
		[]string{"https://public.example/standalone-submodel"},
		collectStringValues(resource, testCase.resultPath),
		"the standalone row filter must not be deduplicated with the differently scoped nested fragment filter",
	)
}

func aasRowMatchingIntegrationCases(fixture rowMatchingIntegrationFixture) []rowMatchingIntegrationCase {
	return []rowMatchingIntegrationCase{
		{
			name:       "AAS submodels[] filters references by their own key",
			endpoint:   "/query/shells",
			rootField:  "$aas#id",
			rootValue:  fixture.aasID,
			fragment:   "$aas#submodels[]",
			condition:  equalStringCondition("$aas#submodels[].keys[].value", "urn:test:row-match:submodel:visible"),
			resultPath: "submodels[].keys[].value",
			expectedCandidateValues: []string{
				"urn:test:row-match:submodel:hidden",
				"urn:test:row-match:submodel:visible",
			},
			expectedValues: []string{"urn:test:row-match:submodel:visible"},
			explanation:    "The visible reference must remain and the hidden reference must be removed by the production AAS reader.",
		},
		{
			name:       "AAS submodels keys[] filters reference key rows",
			endpoint:   "/query/shells",
			rootField:  "$aas#id",
			rootValue:  fixture.aasID,
			fragment:   "$aas#submodels[].keys[]",
			condition:  equalStringCondition("$aas#submodels[].keys[].value", "urn:test:row-match:submodel:visible"),
			resultPath: "submodels[].keys[].value",
			expectedCandidateValues: []string{
				"urn:test:row-match:submodel:hidden",
				"urn:test:row-match:submodel:visible",
			},
			expectedValues: []string{"urn:test:row-match:submodel:visible"},
			explanation:    "The keys[] fragment must be applied by the production submodel-reference payload query.",
		},
		{
			name:                    "AAS specificAssetIds[] filters current rows",
			endpoint:                "/query/shells",
			rootField:               "$aas#id",
			rootValue:               fixture.aasID,
			fragment:                "$aas#assetInformation.specificAssetIds[]",
			condition:               equalStringCondition("$aas#assetInformation.specificAssetIds[].name", "public-id"),
			resultPath:              "assetInformation.specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"public-id"},
			explanation:             "The private specificAssetId must not survive because a different row has the public name.",
		},
		{
			name:                    "AAS externalSubjectId keys[] filters current key rows",
			endpoint:                "/query/shells",
			rootField:               "$aas#id",
			rootValue:               fixture.aasID,
			fragment:                "$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[]",
			condition:               equalStringCondition("$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[].value", "PUBLIC"),
			resultPath:              "assetInformation.specificAssetIds[].externalSubjectId.keys[].value",
			expectedCandidateValues: []string{"PRIVATE", "PUBLIC"},
			expectedValues:          []string{"PUBLIC"},
			explanation:             "Only the externalSubjectId key row whose own value is PUBLIC must remain.",
		},
	}
}

func descriptorRowMatchingIntegrationCases(fixture rowMatchingIntegrationFixture) []rowMatchingIntegrationCase {
	root := rowMatchingIntegrationCase{
		endpoint:  "/query/shell-descriptors",
		rootField: "$aasdesc#idShort",
		rootValue: fixture.aasDescriptorIDShort,
	}

	return []rowMatchingIntegrationCase{
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "AAS descriptor specificAssetIds[] filters current rows",
			fragment:                "$aasdesc#specificAssetIds[]",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[].name", "public-id"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"public-id"},
			explanation:             "Only the descriptor specificAssetId whose own name is public must remain.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:     "AAS descriptor match filters only the matching specificAssetId row",
			fragment: "$aasdesc#specificAssetIds[]",
			condition: map[string]any{
				"$match": []any{
					equalStringCondition("$aasdesc#specificAssetIds[].name", "public-id"),
				},
			},
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"public-id"},
			explanation:             "A matching sibling must not make the $match expression true for every specificAssetId row.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "AAS descriptor boolCast filters only the true specificAssetId row",
			fragment:   "$aasdesc#specificAssetIds[]",
			condition:  map[string]any{"$boolCast": map[string]any{"$field": "$aasdesc#specificAssetIds[].value"}},
			resultPath: "specificAssetIds[].name",
			expectedCandidateValues: []string{
				"private-id",
				"public-id",
			},
			expectedValues: []string{"public-id"},
			explanation:    "The standalone $boolCast must resolve the field against each current candidate row.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "AAS descriptor condition below specificAssetIds[] stays on owner row",
			fragment:                "$aasdesc#specificAssetIds[]",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value", "PUBLIC"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"public-id"},
			explanation:             "A nested PUBLIC key must keep its owning specificAssetId without borrowing a key from another row.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "AAS descriptor externalSubjectId keys[] filters current rows",
			fragment:                "$aasdesc#specificAssetIds[].externalSubjectId.keys[]",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value", "PUBLIC"),
			resultPath:              "specificAssetIds[].externalSubjectId.keys[].value",
			expectedCandidateValues: []string{"PRIVATE", "PUBLIC"},
			expectedValues:          []string{"PUBLIC"},
			explanation:             "Only the selected externalSubjectId key payload must remain.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "AAS descriptor endpoints[] filters current rows",
			fragment:   "$aasdesc#endpoints[]",
			condition:  equalStringCondition("$aasdesc#endpoints[].protocolinformation.href", "https://public.example/shell"),
			resultPath: "endpoints[].protocolInformation.href",
			expectedCandidateValues: []string{
				"https://admin.example/shell",
				"https://public.example/shell",
			},
			expectedValues: []string{"https://public.example/shell"},
			explanation:    "Only the endpoint whose own href is public must remain.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "AAS descriptor submodelDescriptors[] filters current rows",
			fragment:   "$aasdesc#submodelDescriptors[]",
			condition:  equalStringCondition("$aasdesc#submodelDescriptors[].idShort", "VisibleNestedDescriptor"),
			resultPath: "submodelDescriptors[].idShort",
			expectedCandidateValues: []string{
				"HiddenNestedDescriptor",
				"VisibleNestedDescriptor",
			},
			expectedValues: []string{"VisibleNestedDescriptor"},
			explanation:    "The hidden nested submodel descriptor must be removed from the array.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "AAS descriptor submodelDescriptors[] supports a condition below the fragment",
			fragment:   "$aasdesc#submodelDescriptors[]",
			condition:  equalStringCondition("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath: "submodelDescriptors[].idShort",
			expectedCandidateValues: []string{
				"HiddenNestedDescriptor",
				"VisibleNestedDescriptor",
			},
			expectedValues: []string{"VisibleNestedDescriptor"},
			explanation:    "The nested semantic key must filter its owning submodel descriptor row.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "nested submodel descriptor semanticId keys[] filters current rows",
			fragment:                "$aasdesc#submodelDescriptors[].semanticId.keys[]",
			condition:               equalStringCondition("$aasdesc#submodelDescriptors[].semanticId.keys[].value", "semantic-public"),
			resultPath:              "submodelDescriptors[].semanticId.keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "The nested semanticId key query must return only the matching key payload.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "nested submodel descriptor supplementalSemanticIds[] filters current rows",
			fragment:   "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
			condition:  equalStringCondition("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath: "submodelDescriptors[].supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{
				"semantic-extra",
				"semantic-internal",
				"semantic-public",
			},
			expectedValues: []string{"semantic-public"},
			explanation:    "Only the matching nested supplementalSemanticId reference must remain.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "nested submodel descriptor supplementalSemanticIds keys[] filters current rows",
			fragment:   "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]",
			condition:  equalStringCondition("$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath: "submodelDescriptors[].supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{
				"semantic-extra",
				"semantic-internal",
				"semantic-public",
			},
			expectedValues: []string{"semantic-public"},
			explanation:    "The terminal keys[] must filter individual nested key payloads.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "nested submodel descriptor endpoints[] filters current rows",
			fragment:   "$aasdesc#submodelDescriptors[].endpoints[]",
			condition:  equalStringCondition("$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href", "https://public.example/submodel"),
			resultPath: "submodelDescriptors[].endpoints[].protocolInformation.href",
			expectedCandidateValues: []string{
				"https://admin.example/submodel",
				"https://internal.example/submodel",
				"https://public.example/submodel",
			},
			expectedValues: []string{"https://public.example/submodel"},
			explanation:    "Only the matching nested endpoint row must remain.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "unrelated matching endpoint condition retains all specificAssetIds[] rows",
			fragment:                "$aasdesc#specificAssetIds[]",
			condition:               equalStringCondition("$aasdesc#endpoints[].protocolinformation.href", "https://public.example/shell"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues: []string{
				"private-id",
				"public-id",
			},
			explanation: "The endpoint condition is correlated to the descriptor and must be true for both selected specificAssetId rows.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:                    "unrelated nonmatching endpoint condition removes all specificAssetIds[] rows",
			fragment:                "$aasdesc#specificAssetIds[]",
			condition:               equalStringCondition("$aasdesc#endpoints[].protocolinformation.href", "https://missing.example/shell"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{},
			explanation:             "A false unrelated condition must remove every selected row without leaking endpoint aliases.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:     "mixed local and unrelated branches filter specificAssetIds[] row wise",
			fragment: "$aasdesc#specificAssetIds[]",
			condition: andCondition(
				equalStringCondition("$aasdesc#specificAssetIds[].name", "public-id"),
				orCondition(
					equalStringCondition("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value", "MISSING"),
					equalStringCondition("$aasdesc#endpoints[].protocolinformation.href", "https://public.example/shell"),
				),
			),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"public-id"},
			explanation:             "The local name must match on the current row while the unrelated endpoint supplies the OR fallback.",
		}),
		withRowMatchingRoot(root, rowMatchingIntegrationCase{
			name:       "ancestor condition retains all endpoints[] rows",
			fragment:   "$aasdesc#endpoints[]",
			condition:  equalStringCondition("$aasdesc#idShort", fixture.aasDescriptorIDShort),
			resultPath: "endpoints[].protocolInformation.href",
			expectedCandidateValues: []string{
				"https://admin.example/shell",
				"https://public.example/shell",
			},
			expectedValues: []string{"https://admin.example/shell", "https://public.example/shell"},
			explanation:    "The shared ancestor condition is true for every endpoint candidate.",
		}),
	}
}

func submodelRowMatchingIntegrationCases(fixture rowMatchingIntegrationFixture) []rowMatchingIntegrationCase {
	return []rowMatchingIntegrationCase{
		{
			name:                    "submodel semanticId keys[] filters current rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.submodelID,
			fragment:                "$sm#semanticId.keys[]",
			condition:               equalStringCondition("$sm#semanticId.keys[].value", "semantic-public"),
			resultPath:              "semanticId.keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "The production submodel reader must return only the matching semanticId key.",
		},
		{
			name:                    "submodel supplementalSemanticIds[] filters current rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.submodelID,
			fragment:                "$sm#supplementalSemanticIds[]",
			condition:               equalStringCondition("$sm#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "Only the matching supplementalSemanticId reference must remain.",
		},
		{
			name:                    "submodel supplementalSemanticIds keys[] filters current rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.submodelID,
			fragment:                "$sm#supplementalSemanticIds[].keys[]",
			condition:               equalStringCondition("$sm#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "The terminal keys[] must filter the key payloads themselves.",
		},
		{
			name:                    "submodel descriptor semanticId keys[] filters current rows",
			endpoint:                "/query/submodel-descriptors",
			rootField:               "$smdesc#idShort",
			rootValue:               fixture.submodelDescriptorName,
			fragment:                "$smdesc#semanticId.keys[]",
			condition:               equalStringCondition("$smdesc#semanticId.keys[].value", "semantic-public"),
			resultPath:              "semanticId.keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "Only the matching standalone submodel descriptor semanticId key must remain.",
		},
		{
			name:                    "submodel descriptor supplementalSemanticIds[] filters current rows",
			endpoint:                "/query/submodel-descriptors",
			rootField:               "$smdesc#idShort",
			rootValue:               fixture.submodelDescriptorName,
			fragment:                "$smdesc#supplementalSemanticIds[]",
			condition:               equalStringCondition("$smdesc#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "Only the matching standalone supplementalSemanticId reference must remain.",
		},
		{
			name:                    "submodel descriptor supplementalSemanticIds keys[] filters current rows",
			endpoint:                "/query/submodel-descriptors",
			rootField:               "$smdesc#idShort",
			rootValue:               fixture.submodelDescriptorName,
			fragment:                "$smdesc#supplementalSemanticIds[].keys[]",
			condition:               equalStringCondition("$smdesc#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "Only the matching standalone supplemental semantic key must remain.",
		},
		{
			name:       "submodel descriptor endpoints[] filters current rows",
			endpoint:   "/query/submodel-descriptors",
			rootField:  "$smdesc#idShort",
			rootValue:  fixture.submodelDescriptorName,
			fragment:   "$smdesc#endpoints[]",
			condition:  equalStringCondition("$smdesc#endpoints[].protocolinformation.href", "https://public.example/standalone-submodel"),
			resultPath: "endpoints[].protocolInformation.href",
			expectedCandidateValues: []string{
				"https://admin.example/standalone-submodel",
				"https://public.example/standalone-submodel",
			},
			expectedValues: []string{"https://public.example/standalone-submodel"},
			explanation:    "Only the matching standalone submodel descriptor endpoint must remain.",
		},
	}
}

func smeRowMatchingIntegrationCases(fixture rowMatchingIntegrationFixture) []rowMatchingIntegrationCase {
	return []rowMatchingIntegrationCase{
		{
			name:                    "SME a[] filters list items by their own values",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]",
			condition:               equalStringCondition("$sme.a[]#value", "visible"),
			resultPath:              "submodelElements[].value[].value",
			expectedCandidateValues: []string{"hidden", "visible"},
			expectedValues:          []string{"visible"},
			explanation:             "The production SME reader must remove the hidden a[] property item.",
		},
		{
			name:                    "SME a[] keeps only candidates with a matching b[] descendant",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.nestedSMEsubmodelID,
			fragment:                "$sme.a[]",
			condition:               equalStringCondition("$sme.a[].b[]#value", "visible"),
			resultPath:              "submodelElements[].value[].idShort",
			expectedCandidateValues: []string{"without-visible-child", "with-visible-child"},
			expectedValues:          []string{"with-visible-child"},
			explanation:             "The first a[] row owns the visible b[] child; the second a[] row must not borrow that descendant match.",
		},
		{
			name:                    "SME a[].b[] filters nested list items by their own values",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.nestedSMEsubmodelID,
			fragment:                "$sme.a[].b[]",
			condition:               equalStringCondition("$sme.a[].b[]#value", "visible"),
			resultPath:              "submodelElements[].value[].value[].value[].value",
			expectedCandidateValues: []string{"blocked", "hidden", "visible"},
			expectedValues:          []string{"visible"},
			explanation:             "The final b[] must identify the row being filtered despite the earlier a[].",
		},
		{
			name:                    "SME semanticId keys[] filters current key rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]#semanticId.keys[]",
			condition:               equalStringCondition("$sme.a[]#semanticId.keys[].value", "semantic-public"),
			resultPath:              "submodelElements[].value[].semanticId.keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "The terminal semanticId keys[] must filter keys below the selected SME path.",
		},
		{
			name:                    "SME supplementalSemanticIds[] filters current reference rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]#supplementalSemanticIds[]",
			condition:               equalStringCondition("$sme.a[]#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "submodelElements[].value[].supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "Only matching SME supplementalSemanticId references must remain.",
		},
		{
			name:                    "SME supplementalSemanticIds keys[] filters current key rows",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]#supplementalSemanticIds[].keys[]",
			condition:               equalStringCondition("$sme.a[]#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "submodelElements[].value[].supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-public"},
			explanation:             "The final keys[] must filter individual SME supplemental semantic keys.",
		},
		{
			name:                    "SME unrelated sibling condition retains every a[] row",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]",
			condition:               equalStringCondition("$sme.other#value", "enabled"),
			resultPath:              "submodelElements[].value[].value",
			expectedCandidateValues: []string{"hidden", "visible"},
			expectedValues:          []string{"hidden", "visible"},
			explanation:             "The matching sibling condition must be correlated at submodel scope and remain true for both a[] rows.",
		},
		{
			name:      "SME complex local and sibling condition stays row wise",
			endpoint:  "/query/submodels",
			rootField: "$sm#id",
			rootValue: fixture.simpleSMEsubmodelID,
			fragment:  "$sme.a[]",
			condition: andCondition(
				equalStringCondition("$sme.a[]#value", "visible"),
				orCondition(
					equalStringCondition("$sme.a[]#semanticId.keys[].value", "semantic-missing"),
					equalStringCondition("$sme.other#value", "enabled"),
				),
			),
			resultPath:              "submodelElements[].value[].value",
			expectedCandidateValues: []string{"hidden", "visible"},
			expectedValues:          []string{"visible"},
			explanation:             "The local value must match on the current a[] row while the sibling supplies the OR fallback.",
		},
	}
}

func nonRowMatchingIntegrationCases(fixture rowMatchingIntegrationFixture) []rowMatchingIntegrationCase {
	return []rowMatchingIntegrationCase{
		{
			name:                    "specificAssetIds object without [] remains collection scoped",
			endpoint:                "/query/shell-descriptors",
			rootField:               "$aasdesc#idShort",
			rootValue:               fixture.aasDescriptorIDShort,
			fragment:                "$aasdesc#specificAssetIds",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[].name", "public-id"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"private-id", "public-id"},
			explanation:             "A fragment not ending in [] must not remove individual specificAssetId rows.",
		},
		{
			name:                    "externalSubjectId object after earlier [] remains collection scoped",
			endpoint:                "/query/shell-descriptors",
			rootField:               "$aasdesc#idShort",
			rootValue:               fixture.aasDescriptorIDShort,
			fragment:                "$aasdesc#specificAssetIds[].externalSubjectId",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[].externalSubjectId.keys[].value", "PUBLIC"),
			resultPath:              "specificAssetIds[].externalSubjectId.keys[].value",
			expectedCandidateValues: []string{"PRIVATE", "PUBLIC"},
			expectedValues:          []string{"PRIVATE", "PUBLIC"},
			explanation:             "The earlier [] must not turn an object-ending fragment into a row filter.",
		},
		{
			name:                    "fixed specificAssetId index does not enable wildcard row matching",
			endpoint:                "/query/shell-descriptors",
			rootField:               "$aasdesc#idShort",
			rootValue:               fixture.aasDescriptorIDShort,
			fragment:                "$aasdesc#specificAssetIds[0]",
			condition:               equalStringCondition("$aasdesc#specificAssetIds[0].name", "public-id"),
			resultPath:              "specificAssetIds[].name",
			expectedCandidateValues: []string{"private-id", "public-id"},
			expectedValues:          []string{"private-id", "public-id"},
			explanation:             "Only an exact wildcard [] ending activates automatic row matching; [0] must remain scoped.",
		},
		{
			name:                    "SME scalar after earlier [] remains collection scoped",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]#value",
			condition:               equalStringCondition("$sme.a[]#value", "visible"),
			resultPath:              "submodelElements[].value[].value",
			expectedCandidateValues: []string{"hidden", "visible"},
			expectedValues:          []string{"hidden", "visible"},
			explanation:             "Because the fragment ends in value, it must not filter individual a[] rows.",
		},
		{
			name:                    "SME supplementalSemanticIds object after earlier [] remains collection scoped",
			endpoint:                "/query/submodels",
			rootField:               "$sm#id",
			rootValue:               fixture.simpleSMEsubmodelID,
			fragment:                "$sme.a[]#supplementalSemanticIds",
			condition:               equalStringCondition("$sme.a[]#supplementalSemanticIds[].keys[].value", "semantic-public"),
			resultPath:              "submodelElements[].value[].supplementalSemanticIds[].keys[].value",
			expectedCandidateValues: []string{"semantic-internal", "semantic-public"},
			expectedValues:          []string{"semantic-internal", "semantic-public"},
			explanation:             "An object-ending SME fragment must preserve the complete matching-scope payload.",
		},
	}
}

func createRowMatchingIntegrationFixture(t *testing.T) rowMatchingIntegrationFixture {
	t.Helper()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fixture := rowMatchingIntegrationFixture{
		aasID:                  "urn:test:row-match:aas:" + suffix,
		aasDescriptorIDShort:   "RowMatchAASDescriptor" + suffix,
		submodelID:             "urn:test:row-match:submodel:" + suffix,
		submodelDescriptorName: "RowMatchSubmodelDescriptor" + suffix,
		simpleSMEsubmodelID:    "urn:test:row-match:sme-simple:" + suffix,
		nestedSMEsubmodelID:    "urn:test:row-match:sme-nested:" + suffix,
	}

	postRowMatchingFixture(t, "/shells", rowMatchingAASPayload(fixture))
	postRowMatchingFixture(t, "/shell-descriptors", rowMatchingAASDescriptorPayload(fixture))
	postRowMatchingFixture(t, "/submodels", rowMatchingSubmodelPayload(fixture))
	postRowMatchingFixture(t, "/submodel-descriptors", rowMatchingSubmodelDescriptorPayload(fixture))
	postRowMatchingFixture(t, "/submodels", rowMatchingSimpleSMEPayload(fixture))
	postRowMatchingFixture(t, "/submodels", rowMatchingNestedSMEPayload(fixture))

	return fixture
}

func rowMatchingAASPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":        fixture.aasID,
		"idShort":   "RowMatchAAS",
		"modelType": "AssetAdministrationShell",
		"assetInformation": map[string]any{
			"assetKind": "Instance",
			"specificAssetIds": []any{
				specificAssetIDPayload("public-id", "true", "PUBLIC"),
				specificAssetIDPayload("private-id", "false", "PRIVATE"),
			},
		},
		"submodels": []any{
			referencePayload("Submodel", "urn:test:row-match:submodel:visible"),
			referencePayload("Submodel", "urn:test:row-match:submodel:hidden"),
		},
	}
}

func rowMatchingAASDescriptorPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":            "urn:test:row-match:aas-descriptor:" + fixture.aasDescriptorIDShort,
		"idShort":       fixture.aasDescriptorIDShort,
		"assetKind":     "Instance",
		"globalAssetId": "urn:test:row-match:global-asset",
		"specificAssetIds": []any{
			specificAssetIDPayload("public-id", "true", "PUBLIC"),
			specificAssetIDPayload("private-id", "false", "PRIVATE"),
		},
		"endpoints": []any{
			endpointPayload("AAS-3.0", "https://public.example/shell"),
			endpointPayload("AAS-3.0", "https://admin.example/shell"),
		},
		"submodelDescriptors": []any{
			map[string]any{
				"id":         "urn:test:row-match:nested-descriptor:visible",
				"idShort":    "VisibleNestedDescriptor",
				"semanticId": referencePayload("GlobalReference", "semantic-public"),
				"supplementalSemanticIds": []any{
					referencePayload("GlobalReference", "semantic-public"),
					referencePayload("GlobalReference", "semantic-extra"),
				},
				"endpoints": []any{
					endpointPayload("SUBMODEL-3.0", "https://public.example/submodel"),
					endpointPayload("SUBMODEL-3.0", "https://admin.example/submodel"),
				},
			},
			map[string]any{
				"id":         "urn:test:row-match:nested-descriptor:hidden",
				"idShort":    "HiddenNestedDescriptor",
				"semanticId": referencePayload("GlobalReference", "semantic-internal"),
				"supplementalSemanticIds": []any{
					referencePayload("GlobalReference", "semantic-internal"),
				},
				"endpoints": []any{
					endpointPayload("SUBMODEL-3.0", "https://internal.example/submodel"),
				},
			},
		},
	}
}

func rowMatchingSubmodelPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":        fixture.submodelID,
		"idShort":   "RowMatchSubmodel",
		"modelType": "Submodel",
		"semanticId": map[string]any{
			"type": "ExternalReference",
			"keys": []any{
				map[string]any{"type": "GlobalReference", "value": "semantic-public"},
				map[string]any{"type": "GlobalReference", "value": "semantic-internal"},
			},
		},
		"supplementalSemanticIds": []any{
			referencePayload("GlobalReference", "semantic-public"),
			referencePayload("GlobalReference", "semantic-internal"),
		},
		"submodelElements": []any{},
	}
}

func rowMatchingSubmodelDescriptorPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":      "urn:test:row-match:submodel-descriptor:" + fixture.submodelDescriptorName,
		"idShort": fixture.submodelDescriptorName,
		"semanticId": map[string]any{
			"type": "ExternalReference",
			"keys": []any{
				map[string]any{"type": "GlobalReference", "value": "semantic-public"},
				map[string]any{"type": "GlobalReference", "value": "semantic-internal"},
			},
		},
		"supplementalSemanticIds": []any{
			referencePayload("GlobalReference", "semantic-public"),
			referencePayload("GlobalReference", "semantic-internal"),
		},
		"endpoints": []any{
			endpointPayload("SUBMODEL-3.0", "https://public.example/standalone-submodel"),
			endpointPayload("SUBMODEL-3.0", "https://admin.example/standalone-submodel"),
		},
	}
}

func rowMatchingSimpleSMEPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":        fixture.simpleSMEsubmodelID,
		"idShort":   "RowMatchSimpleSME",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":              "a",
				"modelType":            "SubmodelElementList",
				"typeValueListElement": "Property",
				"valueTypeListElement": "xs:string",
				"value": []any{
					rowMatchingPropertyPayload("visible", "semantic-public"),
					rowMatchingPropertyPayload("hidden", "semantic-internal"),
				},
			},
			map[string]any{
				"idShort":   "other",
				"modelType": "Property",
				"valueType": "xs:string",
				"value":     "enabled",
			},
		},
	}
}

func rowMatchingNestedSMEPayload(fixture rowMatchingIntegrationFixture) map[string]any {
	return map[string]any{
		"id":        fixture.nestedSMEsubmodelID,
		"idShort":   "RowMatchNestedSME",
		"modelType": "Submodel",
		"submodelElements": []any{
			map[string]any{
				"idShort":              "a",
				"modelType":            "SubmodelElementList",
				"typeValueListElement": "SubmodelElementCollection",
				"value": []any{
					map[string]any{
						"idShort":   "with-visible-child",
						"modelType": "SubmodelElementCollection",
						"value": []any{
							map[string]any{
								"idShort":              "b",
								"modelType":            "SubmodelElementList",
								"typeValueListElement": "Property",
								"valueTypeListElement": "xs:string",
								"value": []any{
									rowMatchingPropertyPayload("visible", "semantic-public"),
									rowMatchingPropertyPayload("hidden", "semantic-internal"),
								},
							},
						},
					},
					map[string]any{
						"idShort":   "without-visible-child",
						"modelType": "SubmodelElementCollection",
						"value": []any{
							map[string]any{
								"idShort":              "b",
								"modelType":            "SubmodelElementList",
								"typeValueListElement": "Property",
								"valueTypeListElement": "xs:string",
								"value": []any{
									rowMatchingPropertyPayload("blocked", "semantic-internal"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func rowMatchingPropertyPayload(value string, semanticValue string) map[string]any {
	return map[string]any{
		"modelType":  "Property",
		"valueType":  "xs:string",
		"value":      value,
		"semanticId": referencePayload("GlobalReference", semanticValue),
		"supplementalSemanticIds": []any{
			referencePayload("GlobalReference", semanticValue),
		},
	}
}

func specificAssetIDPayload(name string, value string, subject string) map[string]any {
	return map[string]any{
		"name":              name,
		"value":             value,
		"externalSubjectId": referencePayload("GlobalReference", subject),
	}
}

func referencePayload(keyType string, value string) map[string]any {
	return map[string]any{
		"type": "ExternalReference",
		"keys": []any{
			map[string]any{
				"type":  keyType,
				"value": value,
			},
		},
	}
}

func endpointPayload(interfaceName string, href string) map[string]any {
	return map[string]any{
		"interface": interfaceName,
		"protocolInformation": map[string]any{
			"href": href,
		},
	}
}

func postRowMatchingFixture(t *testing.T, endpoint string, payload map[string]any) {
	t.Helper()

	status, body, _ := doAASEnvRequest(
		t,
		aasEnvNoRedirectClient,
		http.MethodPost,
		aasEnvBaseURL+endpoint,
		payload,
	)
	require.Equalf(t, http.StatusCreated, status, "POST %s failed: %s", endpoint, string(body))
}

func querySingleRowMatchingResource(
	t *testing.T,
	testCase rowMatchingIntegrationCase,
	includeFilter bool,
) map[string]any {
	t.Helper()

	query := map[string]any{
		"$condition": equalStringCondition(testCase.rootField, testCase.rootValue),
	}
	queryDescription := "unfiltered sanity"
	if includeFilter {
		query["$filters"] = []any{
			map[string]any{
				"$fragment":  testCase.fragment,
				"$condition": testCase.condition,
			},
		}
		queryDescription = "filtered production"
	}
	status, body, _ := doAASEnvRequest(
		t,
		aasEnvNoRedirectClient,
		http.MethodPost,
		aasEnvBaseURL+testCase.endpoint,
		query,
	)
	require.Equalf(t, http.StatusOK, status, "%s query failed: %s", queryDescription, string(body))

	var response struct {
		Result []map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Lenf(
		t,
		response.Result,
		1,
		"%s query must select exactly the seeded root resource",
		queryDescription,
	)
	return response.Result[0]
}

func querySingleRowMatchingResourceWithFilters(
	t *testing.T,
	testCase rowMatchingIntegrationCase,
	filters []any,
) map[string]any {
	t.Helper()

	query := map[string]any{
		"$condition": equalStringCondition(testCase.rootField, testCase.rootValue),
		"$filters":   filters,
	}
	status, body, _ := doAASEnvRequest(
		t,
		aasEnvNoRedirectClient,
		http.MethodPost,
		aasEnvBaseURL+testCase.endpoint,
		query,
	)
	require.Equalf(t, http.StatusOK, status, "filtered production query failed: %s", string(body))

	var response struct {
		Result []map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Result, 1, "filtered production query must select exactly the seeded root resource")
	return response.Result[0]
}

func equalStringCondition(field string, value string) map[string]any {
	return map[string]any{
		"$eq": []any{
			map[string]any{"$field": field},
			map[string]any{"$strVal": value},
		},
	}
}

func andCondition(conditions ...map[string]any) map[string]any {
	items := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		items = append(items, condition)
	}
	return map[string]any{"$and": items}
}

func orCondition(conditions ...map[string]any) map[string]any {
	items := make([]any, 0, len(conditions))
	for _, condition := range conditions {
		items = append(items, condition)
	}
	return map[string]any{"$or": items}
}

func withRowMatchingRoot(root rowMatchingIntegrationCase, testCase rowMatchingIntegrationCase) rowMatchingIntegrationCase {
	testCase.endpoint = root.endpoint
	testCase.rootField = root.rootField
	testCase.rootValue = root.rootValue
	return testCase
}

func collectStringValues(resource map[string]any, path string) []string {
	nodes := []any{resource}
	for _, segment := range strings.Split(path, ".") {
		nodes = descendRowMatchingNodes(nodes, segment)
	}

	values := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if value, ok := node.(string); ok {
			values = append(values, value)
		}
	}
	return values
}

func descendRowMatchingNodes(nodes []any, segment string) []any {
	isArray := strings.HasSuffix(segment, "[]")
	key := strings.TrimSuffix(segment, "[]")
	result := make([]any, 0)

	for _, node := range nodes {
		object, ok := node.(map[string]any)
		if !ok {
			continue
		}
		value, found := object[key]
		if !found {
			continue
		}
		if !isArray {
			result = append(result, value)
			continue
		}
		items, ok := value.([]any)
		if ok {
			result = append(result, items...)
		}
	}
	return result
}
