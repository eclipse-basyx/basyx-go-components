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

package auth

import (
	"encoding/json"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/stretchr/testify/require"
)

type rowMatchingMode string

const (
	rowMatchingModeRowLocal         rowMatchingMode = "row-local"
	rowMatchingModeCollectionScoped rowMatchingMode = "collection-scoped"
)

type rowMatchingMockFields map[string][]string

type rowMatchingMockRow struct {
	id     string
	fields rowMatchingMockFields
}

type rowMatchingMockFieldRow struct {
	id     string
	values []string
}

type rowMatchingDataCase struct {
	name                  string
	fragment              grammar.FragmentStringPattern
	condition             string
	expectedMode          rowMatchingMode
	explanation           string
	sharedFields          rowMatchingMockFields
	candidateRows         []rowMatchingMockRow
	expectedVisibleRowIDs []string
}

func TestAutomaticArrayFragmentRowMatchingAASDescriptorData(t *testing.T) {
	runRowMatchingDataMatrix(t, []rowMatchingDataCase{
		{
			name:         "specific asset ID condition on current row",
			fragment:     "$aasdesc#specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].name"},{"$strVal":"public-id"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the specificAssetId whose own name is public remains.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].name",
				rowMatchingMockFieldRow{id: "public-specific-id", values: []string{"public-id"}},
				rowMatchingMockFieldRow{id: "customer-specific-id", values: []string{"customer-id"}},
				rowMatchingMockFieldRow{id: "internal-specific-id", values: []string{"internal-id"}},
			),
			expectedVisibleRowIDs: []string{"public-specific-id"},
		},
		{
			name:         "specific asset ID condition below fragment",
			fragment:     "$aasdesc#specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"},{"$strVal":"PUBLIC"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "A nested key filters its owning specificAssetId; a row passes when any of its own keys is PUBLIC.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value",
				rowMatchingMockFieldRow{id: "public-subject-id", values: []string{"PUBLIC"}},
				rowMatchingMockFieldRow{id: "private-subject-id", values: []string{"PRIVATE"}},
				rowMatchingMockFieldRow{id: "mixed-subject-id", values: []string{"PRIVATE", "PUBLIC"}},
			),
			expectedVisibleRowIDs: []string{"public-subject-id", "mixed-subject-id"},
		},
		{
			name:         "specific asset ID condition on descriptor ancestor",
			fragment:     "$aasdesc#specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#idShort"},{"$strVal":"PublicShell"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The shared descriptor ancestor is public, so every selected specificAssetId remains.",
			sharedFields: rowMatchingMockFields{
				"$aasdesc#idShort": {"PublicShell"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-specific-id"},
				{id: "second-specific-id"},
			},
			expectedVisibleRowIDs: []string{"first-specific-id", "second-specific-id"},
		},
		{
			name:         "specific asset ID condition on matching unrelated endpoint",
			fragment:     "$aasdesc#specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#endpoints[].protocolinformation.href"},{"$strVal":"https://public.example"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The unrelated endpoint condition is shared and true, so it keeps every specificAssetId candidate.",
			sharedFields: rowMatchingMockFields{
				"$aasdesc#endpoints[].protocolinformation.href": {"https://public.example", "https://admin.example"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-specific-id"},
				{id: "second-specific-id"},
			},
			expectedVisibleRowIDs: []string{"first-specific-id", "second-specific-id"},
		},
		{
			name:         "specific asset ID condition on nonmatching unrelated endpoint",
			fragment:     "$aasdesc#specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#endpoints[].protocolinformation.href"},{"$strVal":"https://public.example"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The unrelated endpoint condition is shared and false, so it removes every specificAssetId candidate.",
			sharedFields: rowMatchingMockFields{
				"$aasdesc#endpoints[].protocolinformation.href": {"https://admin.example"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-specific-id"},
				{id: "second-specific-id"},
			},
			expectedVisibleRowIDs: []string{},
		},
		{
			name:     "specific asset ID mixed local and unrelated condition",
			fragment: "$aasdesc#specificAssetIds[]",
			condition: `{"$and":[
				{"$eq":[{"$field":"$aasdesc#specificAssetIds[].name"},{"$strVal":"public-id"}]},
				{"$or":[
					{"$eq":[{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"},{"$strVal":"PUBLIC"}]},
					{"$eq":[{"$field":"$aasdesc#endpoints[].protocolinformation.href"},{"$strVal":"https://public.example"}]}
				]}
			]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Local fields cannot be borrowed from another row; the unrelated endpoint may provide the OR fallback for each public row.",
			sharedFields: rowMatchingMockFields{
				"$aasdesc#endpoints[].protocolinformation.href": {"https://public.example"},
			},
			candidateRows: []rowMatchingMockRow{
				{
					id: "public-id-with-public-key",
					fields: rowMatchingMockFields{
						"$aasdesc#specificAssetIds[].name":                           {"public-id"},
						"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value": {"PUBLIC"},
					},
				},
				{
					id: "public-id-using-endpoint-fallback",
					fields: rowMatchingMockFields{
						"$aasdesc#specificAssetIds[].name":                           {"public-id"},
						"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value": {"PRIVATE"},
					},
				},
				{
					id: "private-id-with-public-key",
					fields: rowMatchingMockFields{
						"$aasdesc#specificAssetIds[].name":                           {"private-id"},
						"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value": {"PUBLIC"},
					},
				},
			},
			expectedVisibleRowIDs: []string{"public-id-with-public-key", "public-id-using-endpoint-fallback"},
		},
		{
			name:         "external subject key condition on current row",
			fragment:     "$aasdesc#specificAssetIds[].externalSubjectId.keys[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"},{"$strVal":"PUBLIC"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The deepest terminal array filters individual externalSubjectId keys.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value",
				rowMatchingMockFieldRow{id: "public-key", values: []string{"PUBLIC"}},
				rowMatchingMockFieldRow{id: "private-key", values: []string{"PRIVATE"}},
			),
			expectedVisibleRowIDs: []string{"public-key"},
		},
		{
			name:         "descriptor endpoint condition on current row",
			fragment:     "$aasdesc#endpoints[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#endpoints[].protocolinformation.href"},{"$strVal":"https://public.example"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the endpoint row whose own href is public remains.",
			candidateRows: mockRowsForField(
				"$aasdesc#endpoints[].protocolinformation.href",
				rowMatchingMockFieldRow{id: "public-http-endpoint", values: []string{"https://public.example"}},
				rowMatchingMockFieldRow{id: "admin-http-endpoint", values: []string{"https://admin.example"}},
			),
			expectedVisibleRowIDs: []string{"public-http-endpoint"},
		},
		{
			name:         "submodel descriptor condition on current row",
			fragment:     "$aasdesc#submodelDescriptors[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#submodelDescriptors[].idShort"},{"$strVal":"Nameplate"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the Nameplate descriptor remains in submodelDescriptors.",
			candidateRows: mockRowsForField(
				"$aasdesc#submodelDescriptors[].idShort",
				rowMatchingMockFieldRow{id: "nameplate-descriptor", values: []string{"Nameplate"}},
				rowMatchingMockFieldRow{id: "technical-data-descriptor", values: []string{"TechnicalData"}},
				rowMatchingMockFieldRow{id: "documentation-descriptor", values: []string{"Documentation"}},
			),
			expectedVisibleRowIDs: []string{"nameplate-descriptor"},
		},
		{
			name:         "submodel descriptor semantic ID key condition on current row",
			fragment:     "$aasdesc#submodelDescriptors[].semanticId.keys[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#submodelDescriptors[].semanticId.keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the semanticId key row containing the public semantic identifier remains.",
			candidateRows: mockRowsForField(
				"$aasdesc#submodelDescriptors[].semanticId.keys[].value",
				rowMatchingMockFieldRow{id: "public-semantic-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-semantic-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-semantic-key"},
		},
		{
			name:         "submodel descriptor supplemental semantic ID condition below fragment",
			fragment:     "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "A nested key filters its owning supplementalSemanticId reference.",
			candidateRows: mockRowsForField(
				"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-supplemental-reference", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-supplemental-reference", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-supplemental-reference"},
		},
		{
			name:         "submodel descriptor supplemental semantic ID key condition on current row",
			fragment:     "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The final keys[] selects individual supplemental semantic key rows.",
			candidateRows: mockRowsForField(
				"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-supplemental-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-supplemental-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-supplemental-key"},
		},
		{
			name:         "submodel descriptor endpoint condition on current row",
			fragment:     "$aasdesc#submodelDescriptors[].endpoints[]",
			condition:    `{"$eq":[{"$field":"$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href"},{"$strVal":"https://public.example/submodels/nameplate"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the public endpoint row remains below its submodel descriptor.",
			candidateRows: mockRowsForField(
				"$aasdesc#submodelDescriptors[].endpoints[].protocolinformation.href",
				rowMatchingMockFieldRow{id: "public-submodel-endpoint", values: []string{"https://public.example/submodels/nameplate"}},
				rowMatchingMockFieldRow{id: "admin-submodel-endpoint", values: []string{"https://admin.example/submodels/nameplate"}},
			),
			expectedVisibleRowIDs: []string{"public-submodel-endpoint"},
		},
	})
}

func TestAutomaticArrayFragmentRowMatchingAASAndSubmodelData(t *testing.T) {
	runRowMatchingDataMatrix(t, []rowMatchingDataCase{
		{
			name:         "AAS submodel references retain only the permitted reference",
			fragment:     "$aas#submodels[]",
			condition:    `{"$eq":[{"$field":"$aas#submodels[].keys[].value"},{"$strVal":"urn:example:submodel:visible"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Each AAS submodel reference is evaluated using its own key values.",
			candidateRows: mockRowsForField(
				"$aas#submodels[].keys[].value",
				rowMatchingMockFieldRow{id: "visible-reference", values: []string{"urn:example:submodel:visible"}},
				rowMatchingMockFieldRow{id: "maintenance-reference", values: []string{"urn:example:submodel:maintenance"}},
				rowMatchingMockFieldRow{id: "diagnostics-reference", values: []string{"urn:example:submodel:diagnostics"}},
			),
			expectedVisibleRowIDs: []string{"visible-reference"},
		},
		{
			name:         "AAS submodel reference keys retain only the permitted key",
			fragment:     "$aas#submodels[].keys[]",
			condition:    `{"$eq":[{"$field":"$aas#submodels[].keys[].value"},{"$strVal":"urn:example:submodel:visible"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "When keys[] is the fragment, each key row is filtered independently.",
			candidateRows: mockRowsForField(
				"$aas#submodels[].keys[].value",
				rowMatchingMockFieldRow{id: "visible-reference-key", values: []string{"urn:example:submodel:visible"}},
				rowMatchingMockFieldRow{id: "hidden-reference-key", values: []string{"urn:example:submodel:hidden"}},
			),
			expectedVisibleRowIDs: []string{"visible-reference-key"},
		},
		{
			name:         "AAS specific asset ID condition on current row",
			fragment:     "$aas#assetInformation.specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aas#assetInformation.specificAssetIds[].name"},{"$strVal":"public-id"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the AAS specificAssetId row with the public name remains.",
			candidateRows: mockRowsForField(
				"$aas#assetInformation.specificAssetIds[].name",
				rowMatchingMockFieldRow{id: "public-aas-specific-id", values: []string{"public-id"}},
				rowMatchingMockFieldRow{id: "internal-aas-specific-id", values: []string{"internal-id"}},
			),
			expectedVisibleRowIDs: []string{"public-aas-specific-id"},
		},
		{
			name:         "AAS specific asset ID condition below fragment",
			fragment:     "$aas#assetInformation.specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[].value"},{"$strVal":"PUBLIC"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The externalSubjectId key is evaluated on its owning AAS specificAssetId row.",
			candidateRows: mockRowsForField(
				"$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[].value",
				rowMatchingMockFieldRow{id: "public-aas-subject-id", values: []string{"PUBLIC"}},
				rowMatchingMockFieldRow{id: "private-aas-subject-id", values: []string{"PRIVATE"}},
			),
			expectedVisibleRowIDs: []string{"public-aas-subject-id"},
		},
		{
			name:         "AAS external subject key condition on current row",
			fragment:     "$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[]",
			condition:    `{"$eq":[{"$field":"$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[].value"},{"$strVal":"PUBLIC"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The terminal keys[] filters the AAS externalSubjectId key rows independently.",
			candidateRows: mockRowsForField(
				"$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[].value",
				rowMatchingMockFieldRow{id: "public-aas-subject-key", values: []string{"PUBLIC"}},
				rowMatchingMockFieldRow{id: "private-aas-subject-key", values: []string{"PRIVATE"}},
			),
			expectedVisibleRowIDs: []string{"public-aas-subject-key"},
		},
		{
			name:         "AAS specific asset ID condition on unrelated submodel reference",
			fragment:     "$aas#assetInformation.specificAssetIds[]",
			condition:    `{"$eq":[{"$field":"$aas#submodels[].keys[].value"},{"$strVal":"urn:example:submodel:visible"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "A matching AAS submodel reference is shared by all selected specificAssetId rows.",
			sharedFields: rowMatchingMockFields{
				"$aas#submodels[].keys[].value": {"urn:example:submodel:visible"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-aas-specific-id"},
				{id: "second-aas-specific-id"},
			},
			expectedVisibleRowIDs: []string{"first-aas-specific-id", "second-aas-specific-id"},
		},
		{
			name:         "submodel semantic ID key condition on current row",
			fragment:     "$sm#semanticId.keys[]",
			condition:    `{"$eq":[{"$field":"$sm#semanticId.keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the public semanticId key remains.",
			candidateRows: mockRowsForField(
				"$sm#semanticId.keys[].value",
				rowMatchingMockFieldRow{id: "public-submodel-semantic-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-submodel-semantic-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-submodel-semantic-key"},
		},
		{
			name:         "submodel supplemental semantic ID condition below fragment",
			fragment:     "$sm#supplementalSemanticIds[]",
			condition:    `{"$eq":[{"$field":"$sm#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "A nested key filters its owning supplementalSemanticId reference.",
			candidateRows: mockRowsForField(
				"$sm#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-submodel-supplemental-reference", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-submodel-supplemental-reference", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-submodel-supplemental-reference"},
		},
		{
			name:         "submodel supplemental semantic ID key condition on current row",
			fragment:     "$sm#supplementalSemanticIds[].keys[]",
			condition:    `{"$eq":[{"$field":"$sm#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the public supplemental semantic key row remains.",
			candidateRows: mockRowsForField(
				"$sm#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-submodel-supplemental-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-submodel-supplemental-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-submodel-supplemental-key"},
		},
		{
			name:         "submodel descriptor semantic ID key condition on current row",
			fragment:     "$smdesc#semanticId.keys[]",
			condition:    `{"$eq":[{"$field":"$smdesc#semanticId.keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The submodel descriptor semanticId keys are filtered independently.",
			candidateRows: mockRowsForField(
				"$smdesc#semanticId.keys[].value",
				rowMatchingMockFieldRow{id: "public-sm-descriptor-semantic-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sm-descriptor-semantic-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sm-descriptor-semantic-key"},
		},
		{
			name:         "submodel descriptor supplemental semantic ID condition below fragment",
			fragment:     "$smdesc#supplementalSemanticIds[]",
			condition:    `{"$eq":[{"$field":"$smdesc#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The nested public key keeps only its owning submodel descriptor supplemental reference.",
			candidateRows: mockRowsForField(
				"$smdesc#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-sm-descriptor-supplemental-reference", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sm-descriptor-supplemental-reference", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sm-descriptor-supplemental-reference"},
		},
		{
			name:         "submodel descriptor supplemental semantic ID key condition on current row",
			fragment:     "$smdesc#supplementalSemanticIds[].keys[]",
			condition:    `{"$eq":[{"$field":"$smdesc#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The final keys[] filters individual submodel descriptor supplemental keys.",
			candidateRows: mockRowsForField(
				"$smdesc#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-sm-descriptor-supplemental-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sm-descriptor-supplemental-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sm-descriptor-supplemental-key"},
		},
		{
			name:         "submodel descriptor endpoint condition on current row",
			fragment:     "$smdesc#endpoints[]",
			condition:    `{"$eq":[{"$field":"$smdesc#endpoints[].protocolinformation.href"},{"$strVal":"https://public.example/submodels/nameplate"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only the public submodel descriptor endpoint remains.",
			candidateRows: mockRowsForField(
				"$smdesc#endpoints[].protocolinformation.href",
				rowMatchingMockFieldRow{id: "public-sm-descriptor-endpoint", values: []string{"https://public.example/submodels/nameplate"}},
				rowMatchingMockFieldRow{id: "admin-sm-descriptor-endpoint", values: []string{"https://admin.example/submodels/nameplate"}},
			),
			expectedVisibleRowIDs: []string{"public-sm-descriptor-endpoint"},
		},
	})
}

func TestAutomaticArrayFragmentRowMatchingSMEData(t *testing.T) {
	runRowMatchingDataMatrix(t, []rowMatchingDataCase{
		{
			name:         "SME list item condition on current row",
			fragment:     "$sme.a[]",
			condition:    `{"$eq":[{"$field":"$sme.a[]#value"},{"$strVal":"visible"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Only a[] elements whose own value is visible remain.",
			candidateRows: mockRowsForField(
				"$sme.a[]#value",
				rowMatchingMockFieldRow{id: "first-visible-a", values: []string{"visible"}},
				rowMatchingMockFieldRow{id: "hidden-a", values: []string{"hidden"}},
				rowMatchingMockFieldRow{id: "second-visible-a", values: []string{"visible"}},
			),
			expectedVisibleRowIDs: []string{"first-visible-a", "second-visible-a"},
		},
		{
			name:         "nested SME list item condition on current row",
			fragment:     "$sme.a[].b[]",
			condition:    `{"$eq":[{"$field":"$sme.a[].b[]#value"},{"$strVal":"visible"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The final b[] identifies the current row even though a[] occurs earlier.",
			candidateRows: mockRowsForField(
				"$sme.a[].b[]#value",
				rowMatchingMockFieldRow{id: "first-a-visible-b", values: []string{"visible"}},
				rowMatchingMockFieldRow{id: "first-a-hidden-b", values: []string{"hidden"}},
				rowMatchingMockFieldRow{id: "second-a-visible-b", values: []string{"visible"}},
			),
			expectedVisibleRowIDs: []string{"first-a-visible-b", "second-a-visible-b"},
		},
		{
			name:         "SME semantic ID key condition on current row",
			fragment:     "$sme.a[]#semanticId.keys[]",
			condition:    `{"$eq":[{"$field":"$sme.a[]#semanticId.keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The terminal semanticId keys[] filters individual key rows below a[].",
			candidateRows: mockRowsForField(
				"$sme.a[]#semanticId.keys[].value",
				rowMatchingMockFieldRow{id: "public-sme-semantic-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sme-semantic-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sme-semantic-key"},
		},
		{
			name:         "SME supplemental semantic ID condition below fragment",
			fragment:     "$sme.a[]#supplementalSemanticIds[]",
			condition:    `{"$eq":[{"$field":"$sme.a[]#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "A nested key filters its owning SME supplementalSemanticId reference.",
			candidateRows: mockRowsForField(
				"$sme.a[]#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-sme-supplemental-reference", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sme-supplemental-reference", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sme-supplemental-reference"},
		},
		{
			name:         "SME supplemental semantic ID key condition on current row",
			fragment:     "$sme.a[]#supplementalSemanticIds[].keys[]",
			condition:    `{"$eq":[{"$field":"$sme.a[]#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The final keys[] filters individual supplemental semantic key rows.",
			candidateRows: mockRowsForField(
				"$sme.a[]#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-sme-supplemental-key", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-sme-supplemental-key", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-sme-supplemental-key"},
		},
		{
			name:         "SME list condition on matching sibling path",
			fragment:     "$sme.a[]",
			condition:    `{"$eq":[{"$field":"$sme.other#value"},{"$strVal":"enabled"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The sibling SME is shared and enabled, so every a[] row remains.",
			sharedFields: rowMatchingMockFields{
				"$sme.other#value": {"enabled"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-a"},
				{id: "second-a"},
			},
			expectedVisibleRowIDs: []string{"first-a", "second-a"},
		},
		{
			name:         "SME list condition on nonmatching sibling path",
			fragment:     "$sme.a[]",
			condition:    `{"$eq":[{"$field":"$sme.other#value"},{"$strVal":"enabled"}]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "The sibling SME is shared and disabled, so no a[] row remains.",
			sharedFields: rowMatchingMockFields{
				"$sme.other#value": {"disabled"},
			},
			candidateRows: []rowMatchingMockRow{
				{id: "first-a"},
				{id: "second-a"},
			},
			expectedVisibleRowIDs: []string{},
		},
		{
			name:     "nested SME list with complex local and sibling condition",
			fragment: "$sme.a[].b[]",
			condition: `{"$and":[
				{"$eq":[{"$field":"$sme.a[].b[]#value"},{"$strVal":"visible"}]},
				{"$or":[
					{"$eq":[{"$field":"$sme.a[].b[]#semanticId.keys[].value"},{"$strVal":"semantic-public"}]},
					{"$eq":[{"$field":"$sme.other#value"},{"$strVal":"enabled"}]}
				]},
				{"$not":{"$eq":[{"$field":"$sme.blocked#value"},{"$strVal":"true"}]}}
			]}`,
			expectedMode: rowMatchingModeRowLocal,
			explanation:  "Nested AND, OR, and NOT keep local value and semantic checks on each b[] row while sibling checks stay shared.",
			sharedFields: rowMatchingMockFields{
				"$sme.other#value":   {"disabled"},
				"$sme.blocked#value": {"false"},
			},
			candidateRows: []rowMatchingMockRow{
				{
					id: "visible-public-b",
					fields: rowMatchingMockFields{
						"$sme.a[].b[]#value":                   {"visible"},
						"$sme.a[].b[]#semanticId.keys[].value": {"semantic-public"},
					},
				},
				{
					id: "visible-internal-b",
					fields: rowMatchingMockFields{
						"$sme.a[].b[]#value":                   {"visible"},
						"$sme.a[].b[]#semanticId.keys[].value": {"semantic-internal"},
					},
				},
				{
					id: "hidden-public-b",
					fields: rowMatchingMockFields{
						"$sme.a[].b[]#value":                   {"hidden"},
						"$sme.a[].b[]#semanticId.keys[].value": {"semantic-public"},
					},
				},
			},
			expectedVisibleRowIDs: []string{"visible-public-b"},
		},
	})
}

func TestFragmentsNotEndingInWildcardArrayRemainCollectionScopedData(t *testing.T) {
	runRowMatchingDataMatrix(t, []rowMatchingDataCase{
		{
			name:         "specificAssetIds object without array suffix",
			fragment:     "$aasdesc#specificAssetIds",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].name"},{"$strVal":"public-id"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "One public value permits the collection payload; the private row is not individually removed.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].name",
				rowMatchingMockFieldRow{id: "public-specific-id", values: []string{"public-id"}},
				rowMatchingMockFieldRow{id: "private-specific-id", values: []string{"private-id"}},
			),
			expectedVisibleRowIDs: []string{"public-specific-id", "private-specific-id"},
		},
		{
			name:         "externalSubjectId object after earlier wildcard",
			fragment:     "$aasdesc#specificAssetIds[].externalSubjectId",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.type"},{"$strVal":"ExternalReference"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "The fragment ends in an object, so one matching object permits the payload without row-local removal.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].externalSubjectId.type",
				rowMatchingMockFieldRow{id: "external-reference-object", values: []string{"ExternalReference"}},
				rowMatchingMockFieldRow{id: "model-reference-object", values: []string{"ModelReference"}},
			),
			expectedVisibleRowIDs: []string{"external-reference-object", "model-reference-object"},
		},
		{
			name:         "specific asset ID fixed index",
			fragment:     "$aasdesc#specificAssetIds[0]",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[0].name"},{"$strVal":"first-id"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "A fixed [0] ending is not the automatic wildcard row-filter contract.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[0].name",
				rowMatchingMockFieldRow{id: "first-specific-id", values: []string{"first-id"}},
				rowMatchingMockFieldRow{id: "second-specific-id", values: []string{"second-id"}},
			),
			expectedVisibleRowIDs: []string{"first-specific-id", "second-specific-id"},
		},
		{
			name:         "external subject key fixed index",
			fragment:     "$aasdesc#specificAssetIds[].externalSubjectId.keys[0]",
			condition:    `{"$eq":[{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[0].value"},{"$strVal":"first-key"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "Only an exact [] suffix activates automatic row-local filtering; [0] remains scoped.",
			candidateRows: mockRowsForField(
				"$aasdesc#specificAssetIds[].externalSubjectId.keys[0].value",
				rowMatchingMockFieldRow{id: "matching-key-payload", values: []string{"first-key"}},
				rowMatchingMockFieldRow{id: "other-key-payload", values: []string{"other-key"}},
			),
			expectedVisibleRowIDs: []string{"matching-key-payload", "other-key-payload"},
		},
		{
			name:         "SME object path without array suffix",
			fragment:     "$sme.a",
			condition:    `{"$eq":[{"$field":"$sme.a#value"},{"$strVal":"visible"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "An SME object fragment does not filter candidate payload rows independently.",
			candidateRows: mockRowsForField(
				"$sme.a#value",
				rowMatchingMockFieldRow{id: "visible-a-payload", values: []string{"visible"}},
				rowMatchingMockFieldRow{id: "hidden-a-payload", values: []string{"hidden"}},
			),
			expectedVisibleRowIDs: []string{"visible-a-payload", "hidden-a-payload"},
		},
		{
			name:         "SME fixed list index",
			fragment:     "$sme.a[0]",
			condition:    `{"$eq":[{"$field":"$sme.a[0]#value"},{"$strVal":"visible"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "A fixed SME list index does not opt into wildcard row matching.",
			candidateRows: mockRowsForField(
				"$sme.a[0]#value",
				rowMatchingMockFieldRow{id: "visible-indexed-payload", values: []string{"visible"}},
				rowMatchingMockFieldRow{id: "hidden-indexed-payload", values: []string{"hidden"}},
			),
			expectedVisibleRowIDs: []string{"visible-indexed-payload", "hidden-indexed-payload"},
		},
		{
			name:         "SME scalar after earlier wildcard",
			fragment:     "$sme.a[]#value",
			condition:    `{"$eq":[{"$field":"$sme.a[]#value"},{"$strVal":"visible"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "The fragment ends in value, so the earlier a[] does not activate row-local filtering.",
			candidateRows: mockRowsForField(
				"$sme.a[]#value",
				rowMatchingMockFieldRow{id: "visible-value-payload", values: []string{"visible"}},
				rowMatchingMockFieldRow{id: "hidden-value-payload", values: []string{"hidden"}},
			),
			expectedVisibleRowIDs: []string{"visible-value-payload", "hidden-value-payload"},
		},
		{
			name:         "SME supplemental semantic ID object after earlier wildcard",
			fragment:     "$sme.a[]#supplementalSemanticIds",
			condition:    `{"$eq":[{"$field":"$sme.a[]#supplementalSemanticIds[].keys[].value"},{"$strVal":"semantic-public"}]}`,
			expectedMode: rowMatchingModeCollectionScoped,
			explanation:  "The fragment ends in an object name, so a matching nested key permits the payload without row-local removal.",
			candidateRows: mockRowsForField(
				"$sme.a[]#supplementalSemanticIds[].keys[].value",
				rowMatchingMockFieldRow{id: "public-semantic-payload", values: []string{"semantic-public"}},
				rowMatchingMockFieldRow{id: "internal-semantic-payload", values: []string{"semantic-internal"}},
			),
			expectedVisibleRowIDs: []string{"public-semantic-payload", "internal-semantic-payload"},
		},
	})
}

func TestLegacyMATCHPropertyIsRejected(t *testing.T) {
	tests := []struct {
		name      string
		matchJSON string
	}{
		{name: "MATCH true", matchJSON: "true"},
		{name: "MATCH false", matchJSON: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := `{
				"FRAGMENT":"$aasdesc#specificAssetIds[]",
				"CONDITION":{"$boolean":true},
				"MATCH":` + tt.matchJSON + `
			}`

			var filter grammar.AccessPermissionRuleFILTER
			err := json.Unmarshal([]byte(document), &filter)

			require.Error(t, err)
			require.ErrorContains(t, err, "MATCH")
			require.ErrorContains(t, err, "no longer supported")
			require.ErrorContains(t, err, "ending in []")
		})
	}
}

func TestFilterWithoutLegacyMATCHIsAccepted(t *testing.T) {
	document := `{
		"FRAGMENT":"$aasdesc#specificAssetIds[]",
		"CONDITION":{"$boolean":true}
	}`

	var filter grammar.AccessPermissionRuleFILTER
	err := json.Unmarshal([]byte(document), &filter)

	require.NoError(t, err)
	require.Nil(t, filter.MATCH)
}

func TestRowMatchingFragmentSyntaxMatrix(t *testing.T) {
	tests := []struct {
		name     string
		fragment string
	}{
		{name: "AAS specific asset ID row", fragment: "$aas#assetInformation.specificAssetIds[]"},
		{name: "AAS external subject key row", fragment: "$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[]"},
		{name: "AAS submodel reference row", fragment: "$aas#submodels[]"},
		{name: "AAS submodel reference key row", fragment: "$aas#submodels[].keys[]"},
		{name: "specific asset IDs without array suffix", fragment: "$aasdesc#specificAssetIds"},
		{name: "descriptor specific asset ID row", fragment: "$aasdesc#specificAssetIds[]"},
		{name: "specific asset ID external subject object", fragment: "$aasdesc#specificAssetIds[].externalSubjectId"},
		{name: "descriptor external subject key row", fragment: "$aasdesc#specificAssetIds[].externalSubjectId.keys[]"},
		{name: "descriptor endpoint row", fragment: "$aasdesc#endpoints[]"},
		{name: "AAS submodel descriptor row", fragment: "$aasdesc#submodelDescriptors[]"},
		{name: "AAS submodel descriptor semantic ID key row", fragment: "$aasdesc#submodelDescriptors[].semanticId.keys[]"},
		{name: "AAS submodel descriptor supplemental semantic ID row", fragment: "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]"},
		{name: "AAS submodel descriptor supplemental semantic ID key row", fragment: "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]"},
		{name: "AAS submodel descriptor endpoint row", fragment: "$aasdesc#submodelDescriptors[].endpoints[]"},
		{name: "submodel semantic ID key row", fragment: "$sm#semanticId.keys[]"},
		{name: "submodel supplemental semantic ID row", fragment: "$sm#supplementalSemanticIds[]"},
		{name: "submodel supplemental semantic ID key row", fragment: "$sm#supplementalSemanticIds[].keys[]"},
		{name: "submodel descriptor semantic ID key row", fragment: "$smdesc#semanticId.keys[]"},
		{name: "submodel descriptor supplemental semantic ID row", fragment: "$smdesc#supplementalSemanticIds[]"},
		{name: "submodel descriptor supplemental semantic ID key row", fragment: "$smdesc#supplementalSemanticIds[].keys[]"},
		{name: "submodel descriptor endpoint row", fragment: "$smdesc#endpoints[]"},
		{name: "SME list row", fragment: "$sme.a[]"},
		{name: "nested SME list row", fragment: "$sme.a[].b[]"},
		{name: "SME semantic ID key row", fragment: "$sme.a[]#semanticId.keys[]"},
		{name: "SME supplemental semantic ID row", fragment: "$sme.a[]#supplementalSemanticIds[]"},
		{name: "SME supplemental semantic ID key row", fragment: "$sme.a[]#supplementalSemanticIds[].keys[]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encodedFragment, err := json.Marshal(tt.fragment)
			require.NoError(t, err)

			var fragment grammar.FragmentStringPattern
			err = json.Unmarshal(encodedFragment, &fragment)

			require.NoErrorf(t, err, "fragment %q is part of the row-matching contract", tt.fragment)
			require.Equal(t, tt.fragment, string(fragment))
		})
	}
}

func runRowMatchingDataMatrix(t *testing.T, tests []rowMatchingDataCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualMode := rowMatchingModeCollectionScoped
			if fragmentEndsWithWildcardArraySegment(tt.fragment) {
				actualMode = rowMatchingModeRowLocal
			}

			t.Logf("fragment: %s", tt.fragment)
			t.Logf("condition: %s", tt.condition)
			t.Logf("expected mode: %s", tt.expectedMode)
			t.Log(tt.explanation)
			logRowMatchingMockData(t, "shared data", tt.sharedFields)
			for _, row := range tt.candidateRows {
				logRowMatchingMockData(t, "candidate "+row.id, row.fields)
			}

			require.Equalf(
				t,
				tt.expectedMode,
				actualMode,
				"%s; fragment=%s",
				tt.explanation,
				tt.fragment,
			)

			condition := parseRowMatchingCondition(t, tt.condition)
			actualVisibleRowIDs := filterRowMatchingMockData(
				t,
				actualMode,
				condition,
				tt.sharedFields,
				tt.candidateRows,
			)

			t.Logf("expected visible rows: %v", tt.expectedVisibleRowIDs)
			t.Logf("actual visible rows:   %v", actualVisibleRowIDs)
			require.Equalf(
				t,
				tt.expectedVisibleRowIDs,
				actualVisibleRowIDs,
				"%s; fragment=%s condition=%s",
				tt.explanation,
				tt.fragment,
				tt.condition,
			)
		})
	}
}

func mockRowsForField(field string, rows ...rowMatchingMockFieldRow) []rowMatchingMockRow {
	result := make([]rowMatchingMockRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, rowMatchingMockRow{
			id: row.id,
			fields: rowMatchingMockFields{
				field: row.values,
			},
		})
	}
	return result
}

func parseRowMatchingCondition(t *testing.T, conditionJSON string) grammar.LogicalExpression {
	t.Helper()

	var condition grammar.LogicalExpression
	err := json.Unmarshal([]byte(conditionJSON), &condition)
	require.NoErrorf(t, err, "invalid test condition: %s", conditionJSON)
	return condition
}

func filterRowMatchingMockData(
	t *testing.T,
	mode rowMatchingMode,
	condition grammar.LogicalExpression,
	sharedFields rowMatchingMockFields,
	candidateRows []rowMatchingMockRow,
) []string {
	t.Helper()

	if mode == rowMatchingModeCollectionScoped {
		return filterCollectionScopedMockData(t, condition, sharedFields, candidateRows)
	}

	visibleRowIDs := make([]string, 0, len(candidateRows))
	for _, row := range candidateRows {
		if evaluateRowMatchingMockCondition(t, condition, sharedFields, row.fields) {
			visibleRowIDs = append(visibleRowIDs, row.id)
		}
	}
	return visibleRowIDs
}

func filterCollectionScopedMockData(
	t *testing.T,
	condition grammar.LogicalExpression,
	sharedFields rowMatchingMockFields,
	candidateRows []rowMatchingMockRow,
) []string {
	t.Helper()

	collectionFields := mergeRowMatchingMockFields(sharedFields, candidateRows)
	if !evaluateRowMatchingMockCondition(t, condition, collectionFields, nil) {
		return []string{}
	}

	visibleRowIDs := make([]string, 0, len(candidateRows))
	for _, row := range candidateRows {
		visibleRowIDs = append(visibleRowIDs, row.id)
	}
	return visibleRowIDs
}

func mergeRowMatchingMockFields(
	sharedFields rowMatchingMockFields,
	candidateRows []rowMatchingMockRow,
) rowMatchingMockFields {
	merged := make(rowMatchingMockFields, len(sharedFields))
	for field, values := range sharedFields {
		merged[field] = append([]string(nil), values...)
	}
	for _, row := range candidateRows {
		for field, values := range row.fields {
			merged[field] = append(merged[field], values...)
		}
	}
	return merged
}

func evaluateRowMatchingMockCondition(
	t *testing.T,
	condition grammar.LogicalExpression,
	sharedFields rowMatchingMockFields,
	rowFields rowMatchingMockFields,
) bool {
	t.Helper()

	switch {
	case condition.Boolean != nil:
		return *condition.Boolean
	case len(condition.Eq) > 0:
		return rowMatchingMockValuesEqual(t, condition.Eq, sharedFields, rowFields)
	case len(condition.Ne) > 0:
		return !rowMatchingMockValuesEqual(t, condition.Ne, sharedFields, rowFields)
	case len(condition.And) > 0:
		return allRowMatchingMockConditionsMatch(t, condition.And, sharedFields, rowFields)
	case len(condition.Or) > 0:
		return anyRowMatchingMockConditionMatches(t, condition.Or, sharedFields, rowFields)
	case condition.Not != nil:
		return !evaluateRowMatchingMockCondition(t, *condition.Not, sharedFields, rowFields)
	default:
		require.FailNow(t, "unsupported mock condition", "condition=%+v", condition)
		return false
	}
}

func allRowMatchingMockConditionsMatch(
	t *testing.T,
	conditions []grammar.LogicalExpression,
	sharedFields rowMatchingMockFields,
	rowFields rowMatchingMockFields,
) bool {
	t.Helper()

	for _, condition := range conditions {
		if !evaluateRowMatchingMockCondition(t, condition, sharedFields, rowFields) {
			return false
		}
	}
	return true
}

func anyRowMatchingMockConditionMatches(
	t *testing.T,
	conditions []grammar.LogicalExpression,
	sharedFields rowMatchingMockFields,
	rowFields rowMatchingMockFields,
) bool {
	t.Helper()

	for _, condition := range conditions {
		if evaluateRowMatchingMockCondition(t, condition, sharedFields, rowFields) {
			return true
		}
	}
	return false
}

func rowMatchingMockValuesEqual(
	t *testing.T,
	operands grammar.ComparisonItems,
	sharedFields rowMatchingMockFields,
	rowFields rowMatchingMockFields,
) bool {
	t.Helper()
	require.Len(t, operands, 2)

	left := rowMatchingMockOperandValues(t, operands[0], sharedFields, rowFields)
	right := rowMatchingMockOperandValues(t, operands[1], sharedFields, rowFields)
	for _, leftValue := range left {
		for _, rightValue := range right {
			if leftValue == rightValue {
				return true
			}
		}
	}
	return false
}

func rowMatchingMockOperandValues(
	t *testing.T,
	operand grammar.Value,
	sharedFields rowMatchingMockFields,
	rowFields rowMatchingMockFields,
) []string {
	t.Helper()

	if operand.Field != nil {
		field := string(*operand.Field)
		if values, found := rowFields[field]; found {
			return values
		}
		return sharedFields[field]
	}
	if operand.StrVal != nil {
		return []string{string(*operand.StrVal)}
	}

	require.FailNow(t, "unsupported mock operand", "operand=%+v", operand)
	return nil
}

func logRowMatchingMockData(t *testing.T, label string, fields rowMatchingMockFields) {
	t.Helper()

	if len(fields) == 0 {
		t.Logf("%s: {}", label)
		return
	}
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	t.Logf("%s: %s", label, encoded)
}
