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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// CanonicalJSONHash returns a SHA-256 hash over CanonicalJSON(value).
//
// Use this helper when JSON-compatible values need stable digests independent
// of Go map iteration order or input object-key order. Raw JSON inputs are
// parsed before hashing so insignificant whitespace and object-key ordering do
// not affect the returned digest. JSON numbers are decoded with json.Number to
// avoid lossy float64 coercion for large integers.
//
// Parameters:
//   - value: JSON-compatible value, raw JSON bytes, or json.RawMessage to hash.
//
// Returns:
//   - string: Lowercase hexadecimal SHA-256 digest.
//   - error: Error when value cannot be normalized or encoded as canonical JSON.
//
// Example:
//
//	hash, err := common.CanonicalJSONHash(map[string]any{"id": identifier})
//	if err != nil {
//		return err
//	}
//	return hash, nil
func CanonicalJSONHash(value any) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// CanonicalJSON encodes JSON values with stable object-key ordering.
//
// The encoder normalizes typed values, raw JSON bytes, and json.RawMessage into
// JSON data, then writes objects with sorted keys. Array order and scalar values
// are preserved. The result is intended for hashes, signatures, and audit
// artifacts where semantically equivalent JSON must produce identical bytes.
//
// Parameters:
//   - value: JSON-compatible value, raw JSON bytes, or json.RawMessage to encode.
//
// Returns:
//   - []byte: Canonical JSON representation.
//   - error: Error when value cannot be normalized or encoded.
//
// Example:
//
//	canonical, err := common.CanonicalJSON(payload)
//	if err != nil {
//		return nil, err
//	}
//	return canonical, nil
func CanonicalJSON(value any) ([]byte, error) {
	normalized, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err = writeCanonicalJSON(&out, normalized); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DecodeJSONPreservingNumbers decodes one JSON document without coercing numbers to float64.
//
// The decoder rejects malformed JSON and multiple concatenated JSON documents.
// Numeric values are returned as json.Number so callers can preserve exact
// lexical number text during canonicalization or database round trips.
//
// Parameters:
//   - raw: JSON document bytes to decode.
//   - target: Pointer to the value that should receive the decoded document.
//
// Returns:
//   - error: Error when the input is invalid JSON, contains trailing JSON
//     values, or cannot be assigned to target.
//
// Example:
//
//	var payload map[string]any
//	if err := common.DecodeJSONPreservingNumbers(raw, &payload); err != nil {
//		return err
//	}
//	return payload, nil
func DecodeJSONPreservingNumbers(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values in payload")
		}
		return err
	}
	return nil
}

func normalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.RawMessage:
		return decodeNormalizedJSON(typed)
	case []byte:
		return decodeNormalizedJSON(typed)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return decodeNormalizedJSON(encoded)
	}
}

func decodeNormalizedJSON(raw []byte) (any, error) {
	var normalized any
	if err := DecodeJSONPreservingNumbers(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func writeCanonicalJSON(out *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		return writeCanonicalObject(out, typed)
	case []any:
		return writeCanonicalArray(out, typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("marshal scalar: %w", err)
		}
		return writeCanonicalBytes(out, encoded)
	}
}

func writeCanonicalObject(out *bytes.Buffer, value map[string]any) error {
	if err := writeCanonicalByte(out, '{'); err != nil {
		return err
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index > 0 {
			if err := writeCanonicalByte(out, ','); err != nil {
				return err
			}
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("marshal object key: %w", err)
		}
		if err = writeCanonicalBytes(out, keyJSON); err != nil {
			return err
		}
		if err = writeCanonicalByte(out, ':'); err != nil {
			return err
		}
		if err = writeCanonicalJSON(out, value[key]); err != nil {
			return err
		}
	}
	return writeCanonicalByte(out, '}')
}

func writeCanonicalArray(out *bytes.Buffer, value []any) error {
	if err := writeCanonicalByte(out, '['); err != nil {
		return err
	}
	for index, item := range value {
		if index > 0 {
			if err := writeCanonicalByte(out, ','); err != nil {
				return err
			}
		}
		if err := writeCanonicalJSON(out, item); err != nil {
			return err
		}
	}
	return writeCanonicalByte(out, ']')
}

func writeCanonicalBytes(out *bytes.Buffer, value []byte) error {
	if _, err := out.Write(value); err != nil {
		return fmt.Errorf("write canonical json: %w", err)
	}
	return nil
}

func writeCanonicalByte(out *bytes.Buffer, value byte) error {
	if err := out.WriteByte(value); err != nil {
		return fmt.Errorf("write canonical json byte: %w", err)
	}
	return nil
}
