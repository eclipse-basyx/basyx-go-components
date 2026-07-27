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

package grammar

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccessPermissionRuleFilterRejectsLegacyMATCH(t *testing.T) {
	tests := []struct {
		name  string
		match bool
	}{
		{name: "MATCH true", match: true},
		{name: "MATCH false", match: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := json.Marshal(map[string]any{
				"FRAGMENT": "$aas#submodels[]",
				"CONDITION": map[string]any{
					"$boolean": true,
				},
				"MATCH": tt.match,
			})
			require.NoError(t, err)

			var filter AccessPermissionRuleFILTER
			err = json.Unmarshal(document, &filter)

			require.Error(t, err)
			require.ErrorContains(t, err, "MATCH")
			require.ErrorContains(t, err, "unsupported")
			require.ErrorContains(t, err, "ending in []")
		})
	}
}

func TestAccessPermissionRuleFilterAcceptsAutomaticArrayMatchingWithoutMATCH(t *testing.T) {
	document := []byte(`{
		"FRAGMENT": "$aas#submodels[]",
		"CONDITION": {"$boolean": true}
	}`)

	var filter AccessPermissionRuleFILTER
	err := json.Unmarshal(document, &filter)

	require.NoError(t, err)
	require.Nil(t, filter.MATCH)
}
