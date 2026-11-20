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

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreatedSubmodelElementKeepsMetadataOutOfJSON(t *testing.T) {
	created := CreatedSubmodelElement{Path: "Rows[2]", Value: map[string]any{"value": "2"}}
	data, err := json.Marshal(created)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":"2"}`, string(data))
}

func TestCreatedSubmodelElementPath(t *testing.T) {
	idShort := "Child"
	for _, test := range []struct {
		name, parent, committed, expected string
		idShort                           *string
	}{
		{"empty list", "Rows", "Rows[0]", "Rows[0]", nil},
		{"nested list", "Rows[1]", "Rows[1][2]", "Rows[1][2]", nil},
		{"collection", "Group", "Group.Child", "Group.Child", &idShort},
		{"legacy collection", "Group", "", "Group.Child", &idShort},
		{"legacy top level", "", "", "Child", &idShort},
		{"missing path", "Rows", "", "", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body any = map[string]any{}
			if test.committed != "" {
				body = CreatedSubmodelElement{Path: test.committed}
			}
			require.Equal(t, test.expected, CreatedSubmodelElementPath(body, test.parent, test.idShort))
		})
	}
}
