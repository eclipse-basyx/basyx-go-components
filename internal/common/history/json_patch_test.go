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
	"testing"

	"github.com/stretchr/testify/require"
)

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
