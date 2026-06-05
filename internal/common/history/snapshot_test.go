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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotArrayItemMutations(t *testing.T) {
	snapshot := map[string]any{}

	require.NoError(t, AppendSnapshotArrayItem(snapshot, "items", map[string]any{"id": "first"}))
	require.NoError(t, AppendSnapshotArrayItem(snapshot, "items", map[string]any{"id": "second"}))
	require.NoError(t, ReplaceSnapshotArrayItem(snapshot, "items", snapshotItemMatchesID("first"), map[string]any{"id": "renamed"}))
	require.NoError(t, RemoveSnapshotArrayItem(snapshot, "items", snapshotItemMatchesID("second")))

	require.Equal(t, []any{map[string]any{"id": "renamed"}}, snapshot["items"])
}

func TestRemoveSnapshotArrayItemDeletesEmptyField(t *testing.T) {
	snapshot := map[string]any{
		"items": []any{map[string]any{"id": "only"}},
	}

	require.NoError(t, RemoveSnapshotArrayItem(snapshot, "items", snapshotItemMatchesID("only")))
	_, exists := snapshot["items"]
	require.False(t, exists)
}

func snapshotItemMatchesID(id string) SnapshotArrayItemMatcher {
	return func(item map[string]any) bool {
		return item["id"] == id
	}
}
