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

package submodelelements

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnclosingListPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path         string
		expectedPath string
		isListMember bool
	}{
		{path: "Readings[0]", expectedPath: "Readings", isListMember: true},
		{path: "Housing.Readings[2]", expectedPath: "Housing.Readings", isListMember: true},
		{path: "Matrix[1][3]", expectedPath: "Matrix[1]", isListMember: true},
		{path: "Readings[0].Value", isListMember: false},
		{path: "Temperature", isListMember: false},
	}

	for _, testCase := range testCases {
		listPath, isListMember := EnclosingListPath(testCase.path)
		require.Equal(t, testCase.isListMember, isListMember, testCase.path)
		require.Equal(t, testCase.expectedPath, listPath, testCase.path)
	}
}
