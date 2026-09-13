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
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestOperationRequestFromValueOnlyUsesDeclarationsAndPreservesTemplates(t *testing.T) {
	t.Parallel()

	operation := types.NewOperation()
	first := propertyOperationVariable("first", "xs:int", "template-first")
	second := propertyOperationVariable("second", "xs:int", "template-second")
	inoutput := propertyOperationVariable("accumulator", "xs:int", "template-inoutput")
	operation.SetInputVariables([]types.IOperationVariable{first, second})
	operation.SetInoutputVariables([]types.IOperationVariable{inoutput})

	request, err := operationRequestFromValueOnly(operation, gen.OperationRequestValueOnly{
		InputArguments:        map[string]any{"second": "2", "first": "1"},
		InoutputArguments:     map[string]any{"accumulator": "3"},
		ClientTimeoutDuration: "PT5S",
	})
	require.NoError(t, err)
	require.Equal(t, "PT5S", request.ClientTimeoutDuration)
	require.Equal(t, []string{"first", "second"}, operationVariableNames(t, request.InputArguments))
	require.Equal(t, []string{"1", "2"}, operationVariablePropertyValues(t, request.InputArguments))
	require.Equal(t, []string{"accumulator"}, operationVariableNames(t, request.InoutputArguments))
	require.Equal(t, []string{"3"}, operationVariablePropertyValues(t, request.InoutputArguments))
	require.Equal(t, "template-first", *first.Value().(*types.Property).Value())
	require.Equal(t, "template-second", *second.Value().(*types.Property).Value())
	require.Equal(t, "template-inoutput", *inoutput.Value().(*types.Property).Value())
}

func TestOperationRequestFromValueOnlyForwardsOnlySuppliedArguments(t *testing.T) {
	t.Parallel()

	operation := types.NewOperation()
	operation.SetInputVariables([]types.IOperationVariable{
		propertyOperationVariable("first", "xs:int", "0"),
		propertyOperationVariable("second", "xs:int", "0"),
	})

	request, err := operationRequestFromValueOnly(operation, gen.OperationRequestValueOnly{
		InputArguments: map[string]any{"second": "2"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"second"}, operationVariableNames(t, request.InputArguments))
}

func TestOperationRequestFromValueOnlyRejectsUnknownMalformedAndAmbiguousArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		declarations []types.IOperationVariable
		arguments    map[string]any
		errorCode    string
	}{
		{
			name:         "unknown",
			declarations: []types.IOperationVariable{propertyOperationVariable("known", "xs:string", "")},
			arguments:    map[string]any{"unknown": "value"},
			errorCode:    "SMREPO-OPVALREQ-UNKNOWN-inputArguments",
		},
		{
			name:         "malformed property",
			declarations: []types.IOperationVariable{propertyOperationVariable("known", "xs:int", "0")},
			arguments:    map[string]any{"known": map[string]any{"invalid": 1}},
			errorCode:    "SMREPO-OPVALREQ-PROPERTYSHAPE",
		},
		{
			name: "ambiguous declaration",
			declarations: []types.IOperationVariable{
				propertyOperationVariable("duplicate", "xs:string", ""),
				propertyOperationVariable("duplicate", "xs:string", ""),
			},
			arguments: map[string]any{"duplicate": "value"},
			errorCode: "SMREPO-OPVALREQ-AMBIGUOUS-inputArguments",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			operation := types.NewOperation()
			operation.SetInputVariables(test.declarations)
			_, err := operationRequestFromValueOnly(operation, gen.OperationRequestValueOnly{InputArguments: test.arguments})
			require.ErrorContains(t, err, test.errorCode)
		})
	}
}

func TestOperationRequestFromValueOnlyUsesDeclaredStructuredType(t *testing.T) {
	t.Parallel()

	property := propertyElement("nested", "xs:string", "template")
	collection := types.NewSubmodelElementCollection()
	collection.SetIDShort(stringPointer("structure"))
	collection.SetValue([]types.ISubmodelElement{property})
	mlp := types.NewMultiLanguageProperty()
	mlp.SetIDShort(stringPointer("label"))
	operation := types.NewOperation()
	operation.SetInputVariables([]types.IOperationVariable{
		types.NewOperationVariable(collection),
		types.NewOperationVariable(mlp),
	})

	request, err := operationRequestFromValueOnly(operation, gen.OperationRequestValueOnly{
		InputArguments: map[string]any{
			"structure": map[string]any{"nested": "updated"},
			"label":     []map[string]string{{"en": "Updated"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, request.InputArguments, 2)
	convertedCollection := request.InputArguments[0].Value().(*types.SubmodelElementCollection)
	require.Equal(t, "updated", *convertedCollection.Value()[0].(*types.Property).Value())
	convertedMLP := request.InputArguments[1].Value().(*types.MultiLanguageProperty)
	require.Len(t, convertedMLP.Value(), 1)
	require.Equal(t, "en", convertedMLP.Value()[0].Language())
	require.Equal(t, "Updated", convertedMLP.Value()[0].Text())
	require.Equal(t, "template", *property.Value())
}

func TestOperationResultToValueOnlyAcceptsArrayAndEnvelopeForms(t *testing.T) {
	t.Parallel()

	arrayResult, err := operationResultToValueOnly([]any{operationVariableJSON("sum", "8")})
	require.NoError(t, err)
	require.Equal(t, "8", valueOnlyPropertyResult(t, arrayResult, "outputArguments", "sum"))

	messages := []any{map[string]any{"code": "DELEGATE-FAILED", "messageType": "Error", "text": "failed"}}
	envelopeResult, err := operationResultToValueOnly(map[string]any{
		"executionState":    "Failed",
		"success":           false,
		"messages":          messages,
		"outputArguments":   []any{operationVariableJSON("sum", "8")},
		"inoutputArguments": []any{operationVariableJSON("accumulator", "3")},
	})
	require.NoError(t, err)
	require.Equal(t, "Failed", envelopeResult["executionState"])
	require.Equal(t, false, envelopeResult["success"])
	require.Equal(t, messages, envelopeResult["messages"])
	require.Equal(t, "8", valueOnlyPropertyResult(t, envelopeResult, "outputArguments", "sum"))
	require.Equal(t, "3", valueOnlyPropertyResult(t, envelopeResult, "inoutputArguments", "accumulator"))
}

func TestOperationResultToValueOnlyRejectsMissingDuplicateAndUnrepresentableArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []any
		errorCode string
	}{
		{
			name: "missing name",
			arguments: []any{map[string]any{"value": map[string]any{
				"modelType": "Property", "valueType": "xs:string", "value": "value",
			}}},
			errorCode: "SMREPO-OPVALREQ-NAMELESS-outputArguments",
		},
		{
			name:      "duplicate name",
			arguments: []any{operationVariableJSON("duplicate", "first"), operationVariableJSON("duplicate", "second")},
			errorCode: "SMREPO-OPVALRES-DUPLICATE-outputArguments",
		},
		{
			name: "unrepresentable",
			arguments: []any{map[string]any{"value": map[string]any{
				"modelType": "Capability", "idShort": "capability",
			}}},
			errorCode: "SMREPO-OPVALRES-UNREPRESENTABLE-outputArguments-capability",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := operationResultToValueOnly(test.arguments)
			require.ErrorContains(t, err, test.errorCode)
		})
	}
}

func propertyOperationVariable(idShort string, valueType string, value string) types.IOperationVariable {
	return types.NewOperationVariable(propertyElement(idShort, valueType, value))
}

func propertyElement(idShort string, valueType string, value string) *types.Property {
	dataType := types.DataTypeDefXSDString
	if valueType == "xs:int" {
		dataType = types.DataTypeDefXSDInt
	}
	property := types.NewProperty(dataType)
	property.SetIDShort(stringPointer(idShort))
	property.SetValue(stringPointer(value))
	return property
}

func operationVariableNames(t *testing.T, variables []types.IOperationVariable) []string {
	t.Helper()
	names := make([]string, 0, len(variables))
	for _, variable := range variables {
		names = append(names, *variable.Value().IDShort())
	}
	return names
}

func operationVariablePropertyValues(t *testing.T, variables []types.IOperationVariable) []string {
	t.Helper()
	values := make([]string, 0, len(variables))
	for _, variable := range variables {
		property, ok := variable.Value().(*types.Property)
		require.True(t, ok)
		values = append(values, *property.Value())
	}
	return values
}

func operationVariableJSON(idShort string, value string) map[string]any {
	return map[string]any{"value": map[string]any{
		"modelType": "Property",
		"idShort":   idShort,
		"valueType": "xs:string",
		"value":     value,
	}}
}

func valueOnlyPropertyResult(t *testing.T, result map[string]any, group string, idShort string) string {
	t.Helper()
	arguments, ok := result[group].(map[string]any)
	require.True(t, ok)
	serialized, err := json.Marshal(arguments[idShort])
	require.NoError(t, err)
	var value string
	require.NoError(t, json.Unmarshal(serialized, &value))
	return value
}

func TestOperationValueOnlyScalarRoundTrip(t *testing.T) {
	tests := []struct{ name, declaration, value string }{
		{"integer", `{"modelType":"Property","valueType":"xs:int"}`, `3`},
		{"zero exponent integer", `{"modelType":"Property","valueType":"xs:int"}`, `0e-2`},
		{"exponent integer", `{"modelType":"Property","valueType":"xs:int"}`, `3e2`},
		{"integral decimal", `{"modelType":"Property","valueType":"xs:int"}`, `3.0`},
		{"boolean", `{"modelType":"Property","valueType":"xs:boolean"}`, `true`},
		{"large integer", `{"modelType":"Property","valueType":"xs:unsignedLong"}`, `18446744073709551615`},
		{"decimal", `{"modelType":"Property","valueType":"xs:decimal"}`, `1234567890.1234567890123456789`},
		{"range", `{"modelType":"Range","valueType":"xs:int"}`, `{"min":1,"max":10}`},
		{"nested", `{"modelType":"SubmodelElementCollection","value":[{"modelType":"Property","idShort":"enabled","valueType":"xs:boolean"},{"modelType":"SubmodelElementList","idShort":"numbers","typeValueListElement":"Property","valueTypeListElement":"xs:int","value":[{"modelType":"Property","valueType":"xs:int"}]}]}`, `{"enabled":false,"numbers":[3]}`},
		{"empty collection", `{"modelType":"SubmodelElementCollection"}`, `{}`},
		{"empty list", `{"modelType":"SubmodelElementList","typeValueListElement":"Property","valueTypeListElement":"xs:int"}`, `[]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			declaration := valueOnlyTestDeclaration(t, test.declaration)
			before, err := jsonization.ToJsonable(declaration.Value())
			require.NoError(t, err)
			var supplied gen.OperationRequestValueOnly
			require.NoError(t, json.Unmarshal([]byte(`{"inputArguments":{"arg":`+test.value+`}}`), &supplied))
			arguments, err := operationVariablesFromValueOnly([]types.IOperationVariable{declaration}, supplied.InputArguments, "inputArguments")
			require.NoError(t, err)
			result, err := operationResultToValueOnly(arguments)
			require.NoError(t, err)
			actual, err := json.Marshal(result["outputArguments"].(map[string]any)["arg"])
			require.NoError(t, err)
			require.JSONEq(t, test.value, string(actual))
			if test.name == "large integer" || test.name == "decimal" {
				require.Equal(t, test.value, string(actual))
				require.Equal(t, test.value, *arguments[0].Value().(*types.Property).Value())
			}
			after, err := jsonization.ToJsonable(declaration.Value())
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestOperationValueOnlyRejectsInvalidScalarTypes(t *testing.T) {
	for _, test := range []struct{ name, declaration, value string }{
		{"fractional integer", `{"modelType":"Property","valueType":"xs:int"}`, `1.5`},
		{"integer overflow", `{"modelType":"Property","valueType":"xs:int"}`, `2147483648`},
		{"boolean for integer", `{"modelType":"Property","valueType":"xs:int"}`, `true`},
		{"number for string", `{"modelType":"Property","valueType":"xs:string"}`, `3`},
		{"number for boolean", `{"modelType":"Property","valueType":"xs:boolean"}`, `1`},
		{"invalid lexical integer", `{"modelType":"Property","valueType":"xs:int"}`, `"invalid"`},
		{"unknown empty child", `{"modelType":"SubmodelElementCollection"}`, `{"unknown":3}`},
		{"undeclared list item", `{"modelType":"SubmodelElementList","typeValueListElement":"Property"}`, `[3]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var supplied gen.OperationRequestValueOnly
			require.NoError(t, json.Unmarshal([]byte(`{"inputArguments":{"arg":`+test.value+`}}`), &supplied))
			_, err := operationVariablesFromValueOnly([]types.IOperationVariable{valueOnlyTestDeclaration(t, test.declaration)}, supplied.InputArguments, "inputArguments")
			require.Error(t, err)
			require.Contains(t, err.Error(), "SMREPO-OPVALREQ-")
		})
	}
}

func valueOnlyTestDeclaration(t *testing.T, declaration string) types.IOperationVariable {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(declaration), &raw))
	raw["idShort"] = "arg"
	element, err := jsonization.SubmodelElementFromJsonable(raw)
	require.NoError(t, err)
	return types.NewOperationVariable(element)
}

func TestOperationValueOnlyEmptyNamedFields(t *testing.T) {
	for _, declaration := range []string{
		`{"modelType":"Entity","entityType":"CoManagedEntity"}`,
		`{"modelType":"AnnotatedRelationshipElement","first":{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"first"}]},"second":{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"second"}]}}`,
	} {
		variable := valueOnlyTestDeclaration(t, declaration)
		raw, err := jsonization.ToJsonable(variable.Value())
		require.NoError(t, err)
		supplied := map[string]any{"statements": map[string]any{}}
		if raw["modelType"] == "AnnotatedRelationshipElement" {
			supplied = map[string]any{"first": raw["first"], "second": raw["second"], "annotations": map[string]any{}}
		}
		_, err = operationVariableWithValueOnly(variable, supplied)
		require.NoError(t, err)
	}
}

func TestOperationValueOnlyRejectsInvalidDelegatedScalar(t *testing.T) {
	for _, declaration := range []string{
		`{"modelType":"Property","valueType":"xs:int","value":"not-an-int"}`,
		`{"modelType":"Property","valueType":"xs:double","value":"INF"}`,
	} {
		_, err := operationResultToValueOnly([]types.IOperationVariable{valueOnlyTestDeclaration(t, declaration)})
		require.ErrorContains(t, err, "SMREPO-OPVALRES-")
	}
}
