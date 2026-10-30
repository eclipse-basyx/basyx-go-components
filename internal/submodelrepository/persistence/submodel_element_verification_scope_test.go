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
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestVerificationScope(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		path          string
		element       types.ISubmodelElement
		expectedPath  string
		expectedLevel string
	}{
		{name: "list member", path: "Readings[0]", element: types.NewProperty(types.DataTypeDefXSDInt), expectedPath: "Readings", expectedLevel: ""},
		{name: "list", path: "Readings", element: types.NewSubmodelElementList(types.AASSubmodelElementsProperty), expectedPath: "Readings", expectedLevel: ""},
		{name: "collection", path: "Housing", element: types.NewSubmodelElementCollection(), expectedPath: "Housing", expectedLevel: "core"},
		{name: "property", path: "Housing.Color", element: types.NewProperty(types.DataTypeDefXSDString), expectedPath: "Housing.Color", expectedLevel: "core"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path, level := verificationScope(testCase.path, testCase.element)
			require.Equal(t, testCase.expectedPath, path)
			require.Equal(t, testCase.expectedLevel, level)
		})
	}
}
