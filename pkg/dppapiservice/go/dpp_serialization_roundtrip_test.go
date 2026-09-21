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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestCompressedContentPreservesCollectionShapes(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"single field collection list", []any{map[string]any{"partNumber": "A-1"}, map[string]any{"partNumber": "B-2"}}},
		{"nested collection lists", []any{map[string]any{"parts": []any{map[string]any{"partNumber": "A-1"}}}}},
		{"ordinary value and contentType fields", map[string]any{"value": "material", "contentType": "classification"}},
		{"multilingual property", []any{map[string]any{"language": "de", "value": "Motor"}, map[string]any{"language": "en", "value": "Motor"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			element, err := inferElement("data", test.value)
			require.NoError(t, err)
			submodel := types.NewSubmodel("urn:example:serialization")
			submodel.SetSubmodelElements([]types.ISubmodelElement{element})
			content, err := compressedContent(submodel)
			require.NoError(t, err)
			require.Equal(t, map[string]any{"data": test.value}, content)
			single, err := compressedElementValueWithContext(element, "data", dppSerializationContext{})
			require.NoError(t, err)
			require.Equal(t, test.value, single)
		})
	}
}

func TestCompressedContentPreservesImportedEmptyTypedList(t *testing.T) {
	list := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	idShort := "measurements"
	list.SetIDShort(&idShort)
	valueType := types.DataTypeDefXSDDecimal
	list.SetValueTypeListElement(&valueType)
	list.SetValue([]types.ISubmodelElement{})
	submodel := types.NewSubmodel("urn:example:empty-list")
	submodel.SetSubmodelElements([]types.ISubmodelElement{list})
	content, err := compressedContent(submodel)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"measurements": []any{}}, content)
	expanded, err := dppElementFromAASWithContext(list, idShort, dppSerializationContext{})
	require.NoError(t, err)
	require.Equal(t, "xsd:decimal", expanded["valueDataType"])
	require.Empty(t, expanded["elements"])
}
