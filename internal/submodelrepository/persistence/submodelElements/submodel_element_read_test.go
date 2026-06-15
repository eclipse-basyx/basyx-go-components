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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedPathWithListIndex(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeSubmodelElementList, "test.test[0]")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 4)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "test", keys[1].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[2].Type())
	require.Equal(t, "test", keys[2].Value())
	require.Equal(t, types.KeyTypesSubmodelElementList, keys[3].Type())
	require.Equal(t, "0", keys[3].Value())
}

func TestBuildSubmodelElementReferenceBuildsKeyChainForNestedDotPath(t *testing.T) {
	t.Parallel()

	reference, err := buildSubmodelElementReference("sm-1", types.ModelTypeProperty, "parent.child")
	require.NoError(t, err)

	keys := reference.Keys()
	require.Len(t, keys, 3)

	require.Equal(t, types.KeyTypesSubmodel, keys[0].Type())
	require.Equal(t, "sm-1", keys[0].Value())
	require.Equal(t, types.KeyTypesSubmodelElementCollection, keys[1].Type())
	require.Equal(t, "parent", keys[1].Value())
	require.Equal(t, types.KeyTypesProperty, keys[2].Type())
	require.Equal(t, "child", keys[2].Value())
}

func TestEscapeSQLLikePatternEscapesWildcardCharacters(t *testing.T) {
	t.Parallel()

	require.Equal(t, "A!_B", escapeSQLLikePattern("A_B"))
	require.Equal(t, "A!%B", escapeSQLLikePattern("A%B"))
	require.Equal(t, "A!!B", escapeSQLLikePattern("A!B"))
	require.Equal(t, "A!!B!_C!%", escapeSQLLikePattern("A!B_C%"))
}
