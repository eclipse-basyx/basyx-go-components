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
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	jsonPatchOpAdd     = "add"
	jsonPatchOpRemove  = "remove"
	jsonPatchOpReplace = "replace"
)

// BuildJSONPatch returns a deterministic RFC 6902 JSON Patch from one JSON value to another.
//
// Object keys are processed in sorted order and array changes are emitted in an
// order that can be applied directly to the original value. The resulting patch
// is intended for compact history payloads, not for preserving user-authored
// patch document formatting.
//
// Parameters:
//   - from: Original JSON-compatible value.
//   - to: Target JSON-compatible value.
//
// Returns:
//   - []map[string]any: Ordered RFC 6902 operation objects.
//   - error: Error when values cannot be compared, cloned, or encoded.
//
// Example:
//
//	patch, err := BuildJSONPatch(previousSnapshot, currentSnapshot)
//	if err != nil {
//		return nil, err
//	}
//	return patch, nil
func BuildJSONPatch(from any, to any) ([]map[string]any, error) {
	operations := make([]map[string]any, 0)
	if err := appendJSONPatchOperations(&operations, "", from, to); err != nil {
		return nil, err
	}
	return operations, nil
}

// ApplyJSONPatch applies an RFC 6902 JSON Patch to a snapshot and returns the reconstructed snapshot.
//
// The base snapshot is cloned before applying operations, so callers can safely
// reuse their input map. The returned root must remain a JSON object because
// history snapshots represent complete identifiable entities.
//
// Parameters:
//   - base: Snapshot to clone and patch.
//   - operations: RFC 6902 operation objects produced by BuildJSONPatch or read
//     from a diff payload.
//
// Returns:
//   - map[string]any: Reconstructed snapshot after all operations.
//   - error: Error when operations are invalid or the reconstructed root is not
//     a JSON object.
//
// Example:
//
//	restored, err := ApplyJSONPatch(checkpoint, patch)
//	if err != nil {
//		return nil, err
//	}
//	return restored, nil
func ApplyJSONPatch(base map[string]any, operations []map[string]any) (map[string]any, error) {
	clonedBase, err := cloneJSONValue(base)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEBASE " + err.Error())
	}
	document := clonedBase
	for _, operation := range operations {
		document, err = applyJSONPatchOperation(document, operation)
		if err != nil {
			return nil, err
		}
	}
	result, ok := document.(map[string]any)
	if !ok {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-ROOTTYPE reconstructed snapshot root must be an object")
	}
	return result, nil
}

func appendJSONPatchOperations(operations *[]map[string]any, path string, from any, to any) error {
	equal, err := jsonValuesEqual(from, to)
	if err != nil {
		return common.NewInternalServerError("HISTORY-JSONPATCH-COMPARE " + err.Error())
	}
	if equal {
		return nil
	}

	fromObject, fromIsObject := from.(map[string]any)
	toObject, toIsObject := to.(map[string]any)
	if fromIsObject && toIsObject {
		return appendObjectPatchOperations(operations, path, fromObject, toObject)
	}

	fromArray, fromIsArray := from.([]any)
	toArray, toIsArray := to.([]any)
	if fromIsArray && toIsArray {
		return appendArrayPatchOperations(operations, path, fromArray, toArray)
	}

	value, err := cloneJSONValue(to)
	if err != nil {
		return common.NewInternalServerError("HISTORY-JSONPATCH-CLONEREPLACE " + err.Error())
	}
	*operations = append(*operations, map[string]any{
		"op":    jsonPatchOpReplace,
		"path":  path,
		"value": value,
	})
	return nil
}

func appendObjectPatchOperations(operations *[]map[string]any, path string, from map[string]any, to map[string]any) error {
	for _, key := range sortedMissingKeys(from, to) {
		*operations = append(*operations, map[string]any{
			"op":   jsonPatchOpRemove,
			"path": appendJSONPointerToken(path, key),
		})
	}

	for _, key := range sortedSharedKeys(from, to) {
		if err := appendJSONPatchOperations(operations, appendJSONPointerToken(path, key), from[key], to[key]); err != nil {
			return err
		}
	}

	for _, key := range sortedMissingKeys(to, from) {
		value, err := cloneJSONValue(to[key])
		if err != nil {
			return common.NewInternalServerError("HISTORY-JSONPATCH-CLONEADD " + err.Error())
		}
		*operations = append(*operations, map[string]any{
			"op":    jsonPatchOpAdd,
			"path":  appendJSONPointerToken(path, key),
			"value": value,
		})
	}
	return nil
}

func appendArrayPatchOperations(operations *[]map[string]any, path string, from []any, to []any) error {
	commonLength := len(from)
	if len(to) < commonLength {
		commonLength = len(to)
	}
	for index := 0; index < commonLength; index++ {
		if err := appendJSONPatchOperations(operations, appendJSONPointerToken(path, strconv.Itoa(index)), from[index], to[index]); err != nil {
			return err
		}
	}
	for index := len(from) - 1; index >= len(to); index-- {
		*operations = append(*operations, map[string]any{
			"op":   jsonPatchOpRemove,
			"path": appendJSONPointerToken(path, strconv.Itoa(index)),
		})
	}
	for index := len(from); index < len(to); index++ {
		value, err := cloneJSONValue(to[index])
		if err != nil {
			return common.NewInternalServerError("HISTORY-JSONPATCH-CLONEARRAYADD " + err.Error())
		}
		*operations = append(*operations, map[string]any{
			"op":    jsonPatchOpAdd,
			"path":  appendJSONPointerToken(path, strconv.Itoa(index)),
			"value": value,
		})
	}
	return nil
}

func sortedMissingKeys(source map[string]any, other map[string]any) []string {
	keys := make([]string, 0)
	for key := range source {
		if _, exists := other[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func sortedSharedKeys(left map[string]any, right map[string]any) []string {
	keys := make([]string, 0)
	for key := range left {
		if _, exists := right[key]; exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func applyJSONPatchOperation(document any, operation map[string]any) (any, error) {
	op, ok := operation["op"].(string)
	if !ok || strings.TrimSpace(op) == "" {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-OP missing JSON Patch operation")
	}
	path, ok := operation["path"].(string)
	if !ok {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-PATH missing JSON Patch path")
	}
	value, hasValue := operation["value"]
	if op != jsonPatchOpRemove && !hasValue {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-VALUE missing JSON Patch value")
	}
	tokens, err := parseJSONPointer(path)
	if err != nil {
		return nil, err
	}
	return applyJSONPatchAt(document, tokens, op, value)
}

func applyJSONPatchAt(document any, tokens []string, op string, value any) (any, error) {
	if len(tokens) == 0 {
		switch op {
		case jsonPatchOpAdd, jsonPatchOpReplace:
			cloned, err := cloneJSONValue(value)
			if err != nil {
				return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEROOT " + err.Error())
			}
			return cloned, nil
		case jsonPatchOpRemove:
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-REMOVEROOT removing snapshot root is not supported")
		default:
			return nil, unsupportedJSONPatchOperation(op)
		}
	}

	switch typed := document.(type) {
	case map[string]any:
		return applyJSONPatchToObject(typed, tokens, op, value)
	case []any:
		return applyJSONPatchToArray(typed, tokens, op, value)
	default:
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-PARENTTYPE JSON Patch parent is neither object nor array")
	}
}

func applyJSONPatchToObject(object map[string]any, tokens []string, op string, value any) (any, error) {
	key := tokens[0]
	if len(tokens) == 1 {
		return applyJSONPatchObjectLeaf(object, key, op, value)
	}
	child, exists := object[key]
	if !exists {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-MISSINGOBJECTPATH JSON Patch object path is missing")
	}
	updatedChild, err := applyJSONPatchAt(child, tokens[1:], op, value)
	if err != nil {
		return nil, err
	}
	object[key] = updatedChild
	return object, nil
}

func applyJSONPatchObjectLeaf(object map[string]any, key string, op string, value any) (any, error) {
	switch op {
	case jsonPatchOpAdd:
		cloned, err := cloneJSONValue(value)
		if err != nil {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEOBJECTADD " + err.Error())
		}
		object[key] = cloned
	case jsonPatchOpReplace:
		if _, exists := object[key]; !exists {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-REPLACEOBJECTMISSING JSON Patch replace target is missing")
		}
		cloned, err := cloneJSONValue(value)
		if err != nil {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEOBJECTREPLACE " + err.Error())
		}
		object[key] = cloned
	case jsonPatchOpRemove:
		if _, exists := object[key]; !exists {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-REMOVEOBJECTMISSING JSON Patch remove target is missing")
		}
		delete(object, key)
	default:
		return nil, unsupportedJSONPatchOperation(op)
	}
	return object, nil
}

func applyJSONPatchToArray(array []any, tokens []string, op string, value any) (any, error) {
	if len(tokens) == 0 {
		return array, nil
	}
	if len(tokens) == 1 {
		return applyJSONPatchArrayLeaf(array, tokens[0], op, value)
	}
	index, err := parseJSONArrayIndex(tokens[0], len(array), false)
	if err != nil {
		return nil, err
	}
	updatedChild, err := applyJSONPatchAt(array[index], tokens[1:], op, value)
	if err != nil {
		return nil, err
	}
	array[index] = updatedChild
	return array, nil
}

func applyJSONPatchArrayLeaf(array []any, token string, op string, value any) (any, error) {
	switch op {
	case jsonPatchOpAdd:
		index, err := parseJSONArrayIndex(token, len(array), true)
		if err != nil {
			return nil, err
		}
		cloned, cloneErr := cloneJSONValue(value)
		if cloneErr != nil {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEARRAYADD " + cloneErr.Error())
		}
		array = append(array, nil)
		copy(array[index+1:], array[index:])
		array[index] = cloned
	case jsonPatchOpReplace:
		index, err := parseJSONArrayIndex(token, len(array), false)
		if err != nil {
			return nil, err
		}
		cloned, cloneErr := cloneJSONValue(value)
		if cloneErr != nil {
			return nil, common.NewInternalServerError("HISTORY-JSONPATCH-CLONEARRAYREPLACE " + cloneErr.Error())
		}
		array[index] = cloned
	case jsonPatchOpRemove:
		index, err := parseJSONArrayIndex(token, len(array), false)
		if err != nil {
			return nil, err
		}
		array = append(array[:index], array[index+1:]...)
	default:
		return nil, unsupportedJSONPatchOperation(op)
	}
	return array, nil
}

func parseJSONArrayIndex(token string, length int, allowAppend bool) (int, error) {
	if allowAppend && token == "-" {
		return length, nil
	}
	index, err := strconv.Atoi(token)
	if err != nil || index < 0 || index > length || (!allowAppend && index == length) {
		return 0, common.NewInternalServerError("HISTORY-JSONPATCH-ARRAYINDEX invalid JSON Patch array index")
	}
	return index, nil
}

func unsupportedJSONPatchOperation(op string) error {
	return common.NewInternalServerError("HISTORY-JSONPATCH-UNSUPPORTED unsupported JSON Patch operation '" + op + "'")
}

func parseJSONPointer(path string) ([]string, error) {
	if path == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, common.NewInternalServerError("HISTORY-JSONPATCH-BADPOINTER JSON Patch path must be a JSON Pointer")
	}
	rawTokens := strings.Split(path[1:], "/")
	tokens := make([]string, 0, len(rawTokens))
	for _, rawToken := range rawTokens {
		token, err := unescapeJSONPointerToken(rawToken)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func appendJSONPointerToken(path string, token string) string {
	escaped := escapeJSONPointerToken(token)
	if path == "" {
		return "/" + escaped
	}
	return path + "/" + escaped
}

func escapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

func unescapeJSONPointerToken(token string) (string, error) {
	for index := 0; index < len(token); index++ {
		if token[index] == '~' && (index == len(token)-1 || (token[index+1] != '0' && token[index+1] != '1')) {
			return "", common.NewInternalServerError("HISTORY-JSONPATCH-BADESCAPE invalid JSON Pointer escape")
		}
	}
	token = strings.ReplaceAll(token, "~1", "/")
	return strings.ReplaceAll(token, "~0", "~"), nil
}

func jsonValuesEqual(left any, right any) (bool, error) {
	leftJSON, err := CanonicalJSON(left)
	if err != nil {
		return false, err
	}
	rightJSON, err := CanonicalJSON(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func cloneJSONValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json value: %w", err)
	}
	cloned, err := decodeNormalizedJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal json value: %w", err)
	}
	return cloned, nil
}
