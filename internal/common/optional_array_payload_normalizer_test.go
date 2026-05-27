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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAASPayloadOptionalArraysRemovesNullSubmodels(t *testing.T) {
	payload := map[string]any{
		"id":        "https://example.com/aas/1",
		"submodels": nil,
	}

	NormalizeAASPayloadOptionalArrays(payload)

	_, hasSubmodels := payload["submodels"]
	assert.False(t, hasSubmodels)
}

func TestNormalizeAASPayloadOptionalArraysKeepsNonNullSubmodels(t *testing.T) {
	payload := map[string]any{
		"id":        "https://example.com/aas/2",
		"submodels": []any{},
	}

	NormalizeAASPayloadOptionalArrays(payload)

	_, hasSubmodels := payload["submodels"]
	assert.True(t, hasSubmodels)
}

func TestNormalizeSubmodelPayloadOptionalArraysRemovesNullElements(t *testing.T) {
	payload := map[string]any{
		"id":               "https://example.com/sm/1",
		"submodelElements": nil,
	}

	NormalizeSubmodelPayloadOptionalArrays(payload)

	_, hasSubmodelElements := payload["submodelElements"]
	assert.False(t, hasSubmodelElements)
}

func TestNormalizeSubmodelPayloadOptionalArraysIgnoresNonObjectPayload(t *testing.T) {
	payload := []any{map[string]any{"submodelElements": nil}}

	NormalizeSubmodelPayloadOptionalArrays(payload)

	assert.Len(t, payload, 1)
}

func TestNormalizeSubmodelPayloadOptionalArraysRemovesNestedNullFields(t *testing.T) {
	payload := map[string]any{
		"id": "https://example.com/sm/2",
		"submodelElements": []any{
			map[string]any{
				"idShort": "collection",
				"value": []any{
					map[string]any{
						"idShort": "nested",
						"value":   nil,
					},
					nil,
				},
				"semanticId": nil,
			},
		},
	}

	NormalizeSubmodelPayloadOptionalArrays(payload)

	rawElements, hasElements := payload["submodelElements"]
	assert.True(t, hasElements)

	elements, ok := rawElements.([]any)
	assert.True(t, ok)
	assert.Len(t, elements, 1)

	firstElement, ok := elements[0].(map[string]any)
	assert.True(t, ok)
	_, hasSemanticID := firstElement["semanticId"]
	assert.False(t, hasSemanticID)

	nestedRaw, hasNested := firstElement["value"]
	assert.True(t, hasNested)
	nested, ok := nestedRaw.([]any)
	assert.True(t, ok)
	assert.Len(t, nested, 1)

	nestedElement, ok := nested[0].(map[string]any)
	assert.True(t, ok)
	_, hasValue := nestedElement["value"]
	assert.False(t, hasValue)
}
