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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// CanonicalJSONHash returns a SHA-256 hash over deterministic JSON.
func CanonicalJSONHash(value any) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ComputeHistoryRowHash returns the hash-chain row hash for a history event.
func ComputeHistoryRowHash(event ChangeEvent) (string, error) {
	return CanonicalJSONHash(map[string]any{
		"hashContract":        historyRowHashContract,
		"entityType":          event.EntityType,
		"identifier":          event.Identifier,
		"changeType":          event.ChangeType,
		"timestamp":           event.Timestamp.UTC().Format(time.RFC3339Nano),
		"deleted":             event.Deleted,
		"payloadType":         event.PayloadType,
		"contentHash":         event.ContentHash,
		"payloadHash":         event.PayloadHash,
		"previousHash":        event.PreviousHash,
		"requestId":           event.RequestID,
		"correlationId":       event.CorrelationID,
		"actorSubject":        event.ActorSubject,
		"actorIssuer":         event.ActorIssuer,
		"clientId":            event.ClientID,
		"authorizationResult": event.AuthorizationResult,
		"policyId":            event.PolicyID,
		"matchedRuleId":       event.MatchedRuleID,
		"sourceIp":            event.SourceIP,
		"userAgent":           event.UserAgent,
		"operation":           event.Operation,
		"endpoint":            event.Endpoint,
		"httpMethod":          event.HTTPMethod,
	})
}

func computeLegacyHistoryRowHash(event ChangeEvent) (string, error) {
	return CanonicalJSONHash(map[string]any{
		"entityType":    event.EntityType,
		"identifier":    event.Identifier,
		"changeType":    event.ChangeType,
		"timestamp":     event.Timestamp.UTC().Format(time.RFC3339Nano),
		"deleted":       event.Deleted,
		"payloadType":   event.PayloadType,
		"contentHash":   event.ContentHash,
		"payloadHash":   event.PayloadHash,
		"previousHash":  event.PreviousHash,
		"requestId":     event.RequestID,
		"correlationId": event.CorrelationID,
		"actorSubject":  event.ActorSubject,
		"actorIssuer":   event.ActorIssuer,
		"clientId":      event.ClientID,
		"endpoint":      event.Endpoint,
		"httpMethod":    event.HTTPMethod,
	})
}

func databaseTimestamp(value time.Time) time.Time {
	return value.UTC().Round(time.Microsecond)
}

// CanonicalJSON encodes JSON values with stable object-key ordering.
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
	if err := decodeJSONPreservingNumbers(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func decodeJSONPreservingNumbers(raw []byte, target any) error {
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
