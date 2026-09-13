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
			arguments:    map[string]any{"known": 1},
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
