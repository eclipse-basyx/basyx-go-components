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
	"database/sql"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestRenamedPathIsDerivedFromTheStoredParent(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		path     string
		location storedElementLocation
		expected string
	}{
		{name: "top-level", path: "Old.Name", expected: "Renamed"},
		{name: "collection child", path: "Coll.Old.Name", location: parentLocation("Coll", types.ModelTypeSubmodelElementCollection), expected: "Coll.Renamed"},
		{name: "child of list item", path: "List[0].a]", location: parentLocation("List[0]", types.ModelTypeSubmodelElementCollection), expected: "List[0].Renamed"},
		{name: "list item", path: "List[1]", location: parentLocation("List", types.ModelTypeSubmodelElementList), expected: "List[1]"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.expected, testCase.location.renamedPath(testCase.path, "Renamed"))
		})
	}
}

func TestValidateStoredIDShortAllowsUnnamedListItems(t *testing.T) {
	require.NoError(t, validateStoredIDShort("SCOPE", sql.NullString{}))
	require.NoError(t, validateStoredIDShort("SCOPE", sql.NullString{String: "", Valid: true}))
	require.NoError(t, validateStoredIDShort("SCOPE", sql.NullString{String: "Valid", Valid: true}))
	require.ErrorContains(t, validateStoredIDShort("SCOPE", sql.NullString{String: "a;b", Valid: true}), "SCOPE-INVALIDIDSHORT")
}

func TestValueOnlySchemaRejectsStoredInvalidIDShorts(t *testing.T) {
	rows := []valueOnlySchemaRow{
		{idShort: sql.NullString{String: "Coll", Valid: true}},
		{idShort: sql.NullString{String: "", Valid: true}},
	}
	require.NoError(t, validateValueOnlyStoredIDShorts(rows))
	rows = append(rows, valueOnlySchemaRow{idShort: sql.NullString{String: "a;b", Valid: true}})
	require.ErrorContains(t, validateValueOnlyStoredIDShorts(rows), "SMREPO-UPDVALONLY-INVALIDIDSHORT")
}

func TestMetadataPatchRejectsInvalidIDShorts(t *testing.T) {
	stored := sql.NullString{String: "Temperature", Valid: true}
	require.NoError(t, validateMetadataIDShorts(MetadataTarget{IDShort: stored}, map[string]any{"modelType": "Property"}))
	require.NoError(t, validateMetadataIDShorts(MetadataTarget{IDShort: stored}, map[string]any{"modelType": "Property", "idShort": nil}))
	require.ErrorContains(t, validateMetadataIDShorts(MetadataTarget{IDShort: stored}, map[string]any{"modelType": "Property", "idShort": ""}), "SMREPO-PATCHSMEMETA-INVALIDIDSHORT")
	legacy := sql.NullString{String: "a;b", Valid: true}
	require.ErrorContains(t, validateMetadataIDShorts(MetadataTarget{IDShort: legacy}, map[string]any{"modelType": "Property"}), "SMREPO-PATCHSMEMETA-INVALIDIDSHORT")
}

func parentLocation(path string, modelType types.ModelType) storedElementLocation {
	return storedElementLocation{
		parentPath:      sql.NullString{String: path, Valid: true},
		parentModelType: sql.NullInt64{Int64: int64(modelType), Valid: true},
	}
}
