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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateOptionalIDShortAcceptsMetamodelIDShorts(t *testing.T) {
	t.Parallel()
	for _, idShort := range []string{"ab", "A1", "a_", "a-b", "Ab_c-D9", strings.Repeat("a", 128)} {
		require.NoError(t, ValidateOptionalIDShort("TEST-SCOPE", "Element", &idShort), idShort)
	}
}

func TestValidateOptionalIDShortAcceptsOmittedIDShort(t *testing.T) {
	t.Parallel()
	empty := ""
	require.NoError(t, ValidateOptionalIDShort("TEST-SCOPE", "Element", nil))
	require.NoError(t, ValidateOptionalIDShort("TEST-SCOPE", "Element", &empty))
}

func TestValidateOptionalIDShortRejectsOtherIDShorts(t *testing.T) {
	t.Parallel()
	invalidIDShorts := []string{"a", "1a", "_a", "-a", "a-", "a;b", "a b", " ab", "a.b", "a/b", "a%3Bb", "a[0]", "a:b", "äb"}
	for _, idShort := range invalidIDShorts {
		err := ValidateOptionalIDShort("TEST-SCOPE", "Element", &idShort)
		require.True(t, IsErrBadRequest(err), "%q: %v", idShort, err)
		require.ErrorContains(t, err, "TEST-SCOPE-INVALIDIDSHORT Element has the invalid idShort '"+idShort+"'")
	}
}

func TestNewErrInvalidIDShortTruncatesOverlongIDShort(t *testing.T) {
	t.Parallel()
	idShort := strings.Repeat("a", 127) + "ä" + strings.Repeat("b", 1000)
	err := ValidateOptionalIDShort("TEST-SCOPE", "Element", &idShort)
	require.True(t, IsErrBadRequest(err), "%v", err)
	require.ErrorContains(t, err, "has the invalid idShort '"+strings.Repeat("a", 127)+"...'")
	require.NotContains(t, err.Error(), "ab")
}
