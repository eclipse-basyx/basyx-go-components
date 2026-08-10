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

package history

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkBuildJSONPatchReorderedSubmodel(b *testing.B) {
	const elementCount = 1200
	baseElements := make([]any, elementCount)
	targetElements := make([]any, elementCount)
	for position := range elementCount {
		baseElements[position] = map[string]any{
			"idShort": fmt.Sprintf("P%04d", position), "modelType": "Property", "valueType": "xs:string", "value": "old",
		}
		targetElements[position] = map[string]any{
			"idShort": fmt.Sprintf("P%04d", elementCount-1-position), "modelType": "Property", "valueType": "xs:string", "value": "new",
		}
	}
	base := map[string]any{"id": "sm", "submodelElements": baseElements}
	target := map[string]any{"id": "sm", "submodelElements": targetElements}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := BuildJSONPatch(base, target); err != nil {
			b.Fatal(err)
		}
	}
}

func TestJSONPatchBuildAndApplyObjectOperations(t *testing.T) {
	base := map[string]any{
		"id":     "aas-1",
		"remove": true,
		"nested": map[string]any{
			"x/y": "before",
		},
	}
	target := map[string]any{
		"id":    "aas-1",
		"added": "yes",
		"nested": map[string]any{
			"x/y":       "after",
			"tilde~key": nil,
		},
	}

	patch, err := BuildJSONPatch(base, target)
	require.NoError(t, err)
	require.Contains(t, patch, map[string]any{"op": jsonPatchOpRemove, "path": "/remove"})
	require.Contains(t, patch, map[string]any{"op": jsonPatchOpReplace, "path": "/nested/x~1y", "value": "after"})
	require.Contains(t, patch, map[string]any{"op": jsonPatchOpAdd, "path": "/nested/tilde~0key", "value": nil})

	actual, err := ApplyJSONPatch(base, patch)
	require.NoError(t, err)
	require.Equal(t, target, actual)
}

func TestJSONPatchKeepsArrayElementChangesCompact(t *testing.T) {
	base := map[string]any{
		"items": []any{
			map[string]any{"id": "a", "value": "before"},
			map[string]any{"id": "b", "value": "same"},
		},
	}
	target := map[string]any{
		"items": []any{
			map[string]any{"id": "a", "value": "after"},
			map[string]any{"id": "b", "value": "same"},
			map[string]any{"id": "c", "value": "new"},
		},
	}

	patch, err := BuildJSONPatch(base, target)
	require.NoError(t, err)
	require.Contains(t, patch, map[string]any{"op": jsonPatchOpReplace, "path": "/items/0/value", "value": "after"})
	require.Contains(t, patch, map[string]any{"op": jsonPatchOpAdd, "path": "/items/2", "value": map[string]any{"id": "c", "value": "new"}})
	for _, operation := range patch {
		require.NotEqual(t, map[string]any{"op": jsonPatchOpReplace, "path": "/items", "value": target["items"]}, operation)
	}

	actual, err := ApplyJSONPatch(base, patch)
	require.NoError(t, err)
	require.Equal(t, target, actual)
}

func TestJSONPatchEmptyDiffRoundTrip(t *testing.T) {
	base := map[string]any{"id": "aas-1", "value": []any{"same"}}

	patch, err := BuildJSONPatch(base, base)
	require.NoError(t, err)
	require.Empty(t, patch)

	actual, err := ApplyJSONPatch(base, patch)
	require.NoError(t, err)
	require.Equal(t, base, actual)
}

func TestJSONPatchDoesNotRetainTargetContainers(t *testing.T) {
	targetNested := map[string]any{"value": "before"}
	patch, err := BuildJSONPatch(map[string]any{}, map[string]any{"nested": targetNested})
	require.NoError(t, err)
	targetNested["value"] = "after"
	require.Equal(t, map[string]any{
		"op": "add", "path": "/nested", "value": map[string]any{"value": "before"},
	}, patch[0])
}

func TestJSONPatchPreservesLargeIntegerValues(t *testing.T) {
	base := map[string]any{
		"id":      "aas-1",
		"counter": json.Number("9007199254740993"),
	}
	target := map[string]any{
		"id":      "aas-1",
		"counter": json.Number("9007199254740995"),
	}

	patch, err := BuildJSONPatch(base, target)
	require.NoError(t, err)
	require.Contains(t, patch, map[string]any{
		"op":    jsonPatchOpReplace,
		"path":  "/counter",
		"value": json.Number("9007199254740995"),
	})

	actual, err := ApplyJSONPatch(base, patch)
	require.NoError(t, err)
	require.Equal(t, target, actual)
}

func TestJSONPatchRejectsInvalidPath(t *testing.T) {
	_, err := ApplyJSONPatch(map[string]any{"id": "aas-1"}, []map[string]any{
		{"op": jsonPatchOpReplace, "path": "id", "value": "new"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "HISTORY-JSONPATCH-BADPOINTER")
}
