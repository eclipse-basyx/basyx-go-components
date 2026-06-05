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

import "github.com/eclipse-basyx/basyx-go-components/internal/common"

// SnapshotArrayItemMatcher reports whether an array item is the mutation target.
type SnapshotArrayItemMatcher func(item map[string]any) bool

// AppendSnapshotArrayItem appends an item to an optional snapshot array.
func AppendSnapshotArrayItem(snapshot map[string]any, field string, item any) error {
	items, err := snapshotArrayItems(snapshot, field)
	if err != nil {
		return err
	}
	snapshot[field] = append(items, item)
	return nil
}

// ReplaceSnapshotArrayItem replaces the matching item in a snapshot array.
func ReplaceSnapshotArrayItem(snapshot map[string]any, field string, matcher SnapshotArrayItemMatcher, item any) error {
	items, index, err := matchingSnapshotArrayItem(snapshot, field, matcher)
	if err != nil {
		return err
	}
	items[index] = item
	snapshot[field] = items
	return nil
}

// RemoveSnapshotArrayItem removes the matching item from a snapshot array.
func RemoveSnapshotArrayItem(snapshot map[string]any, field string, matcher SnapshotArrayItemMatcher) error {
	items, index, err := matchingSnapshotArrayItem(snapshot, field, matcher)
	if err != nil {
		return err
	}
	items = append(items[:index], items[index+1:]...)
	if len(items) == 0 {
		delete(snapshot, field)
		return nil
	}
	snapshot[field] = items
	return nil
}

func matchingSnapshotArrayItem(snapshot map[string]any, field string, matcher SnapshotArrayItemMatcher) ([]any, int, error) {
	if matcher == nil {
		return nil, -1, common.NewInternalServerError("HISTORY-SNAPSHOTARRAY-NILMATCHER snapshot array matcher is required")
	}
	items, err := snapshotArrayItems(snapshot, field)
	if err != nil {
		return nil, -1, err
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if ok && matcher(item) {
			return items, index, nil
		}
	}
	return nil, -1, common.NewInternalServerError("HISTORY-SNAPSHOTARRAY-NOTFOUND matching item missing from snapshot field '" + field + "'")
}

func snapshotArrayItems(snapshot map[string]any, field string) ([]any, error) {
	if snapshot == nil {
		return nil, common.NewInternalServerError("HISTORY-SNAPSHOTARRAY-NILSNAPSHOT snapshot is required")
	}
	if field == "" {
		return nil, common.NewInternalServerError("HISTORY-SNAPSHOTARRAY-EMPTYFIELD snapshot array field is required")
	}
	rawItems, exists := snapshot[field]
	if !exists {
		return []any{}, nil
	}
	items, ok := rawItems.([]any)
	if !ok {
		return nil, common.NewInternalServerError("HISTORY-SNAPSHOTARRAY-INVALIDFIELD snapshot field '" + field + "' must be an array")
	}
	return items, nil
}
