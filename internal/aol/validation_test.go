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

package aol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateValidationReportsLocationsAndRejectsUnknownTemplate(t *testing.T) {
	minimum, maximum := 1, 1
	template := Template{Slug: "motor", Version: "2.0.0", Root: Node{ModelType: "Submodel", SemanticID: "urn:template", Children: []Node{{IDShort: "Power", ModelType: "Property", ValueType: "xs:double", SemanticID: "urn:power", Cardinality: Cardinality{Min: &minimum, Max: &maximum}}}}}
	draft := Draft{Shell: map[string]any{"id": "urn:aas", "modelType": "AssetAdministrationShell", "assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": "urn:asset"}}, Submodels: []map[string]any{{"id": "urn:sm", "modelType": "Submodel", "semanticId": reference("urn:template"), "submodelElements": []any{}}}, Templates: map[string]string{"urn:sm": "motor"}}
	validator := Validator{Templates: map[string]Template{"motor": template}}
	report := validator.Validate(draft)
	require.False(t, report.Valid)
	require.Contains(t, report.Issues[0].Location, "submodels")
	draft.Submodels[0]["submodelElements"] = []any{map[string]any{"idShort": "Power", "modelType": "Property", "valueType": "xs:string", "value": "12", "semanticId": reference("urn:power")}}
	report = validator.Validate(draft)
	require.False(t, report.Valid)
	draft.Submodels[0]["submodelElements"].([]any)[0].(map[string]any)["valueType"] = "xs:double"
	require.True(t, validator.Validate(draft).Valid)
	draft.Templates["urn:sm"] = "unknown"
	require.False(t, validator.Validate(draft).Valid)
}
func reference(id string) map[string]any {
	return map[string]any{"type": "ExternalReference", "keys": []any{map[string]any{"type": "GlobalReference", "value": id}}}
}

func TestDraftRejectsDanglingShellReferencesAndDuplicateIdentifiers(t *testing.T) {
	v := Validator{}
	draft := Draft{Shell: map[string]any{"id": "urn:aas", "modelType": "AssetAdministrationShell", "assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": "urn:asset"}, "submodels": []any{reference("urn:absent")}}}
	require.False(t, v.Validate(draft).Valid)
}

func TestListTemplateValidatesDeclaredTypeSemanticAndCardinality(t *testing.T) {
	minimum, maximum := 1, 2
	node := Node{ModelType: "SubmodelElementList", ValueType: "xs:double", ListElementType: "Property", ListSemanticID: "urn:item", Item: &Node{ModelType: "Property", ValueType: "xs:double", SemanticID: "urn:item", Cardinality: Cardinality{Min: &minimum, Max: &maximum}}}
	valid := func() map[string]any {
		return map[string]any{"modelType": "SubmodelElementList", "typeValueListElement": "Property", "valueTypeListElement": "xs:double", "semanticIdListElement": reference("urn:item"), "value": []any{map[string]any{"modelType": "Property", "valueType": "xs:double", "value": "12"}}}
	}
	report := Report{}
	validateNode(&report, node, valid(), "/list")
	require.Empty(t, report.Issues, "list items can inherit the list semantic ID")
	for _, field := range []string{"typeValueListElement", "valueTypeListElement", "semanticIdListElement", "value"} {
		t.Run(field, func(t *testing.T) {
			value := valid()
			value[field] = "wrong"
			if field == "value" {
				value[field] = []any{}
			}
			report := Report{}
			validateNode(&report, node, value, "/list")
			require.NotEmpty(t, report.Issues)
		})
	}
}
