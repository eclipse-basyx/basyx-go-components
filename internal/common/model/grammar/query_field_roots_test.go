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

package grammar

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindModelFieldByRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  string
		roots    []string
		expected ModelStringPattern
		found    bool
	}{
		{
			name:    "AAS field does not match hierarchy roots",
			payload: `{"$condition":{"$eq":[{"$field":"$aas#id"},{"$strVal":"aas"}]}}`,
			roots:   []string{"$sm", "$sme"},
		},
		{
			name:     "nested SME numeric cast matches",
			payload:  `{"$condition":{"$lt":[{"$numCast":{"$field":"$sme.Metrics.Value#value"}},{"$numVal":48}]}}`,
			roots:    []string{"$sm", "$sme"},
			expected: "$sme.Metrics.Value#value",
			found:    true,
		},
		{
			name:     "Submodel field in fragment condition matches",
			payload:  `{"$condition":{"$boolean":true},"$filters":[{"$fragment":"$aas#submodels[]","$condition":{"$eq":[{"$field":"$sm#idShort"},{"$strVal":"TechnicalData"}]}}]}`,
			roots:    []string{"$sm"},
			expected: "$sm#idShort",
			found:    true,
		},
		{
			name:     "selected Submodel field matches",
			payload:  `{"$condition":{"$boolean":true},"$select":["$sm#id"]}`,
			roots:    []string{"$sm"},
			expected: "$sm#id",
			found:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var query Query
			require.NoError(t, json.Unmarshal([]byte(test.payload), &query))

			actual, found := FindModelFieldByRoot(query, test.roots...)

			require.Equal(t, test.found, found)
			require.Equal(t, test.expected, actual)
		})
	}
}
