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
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// CanonicalJSONHash returns a SHA-256 hash over CanonicalJSON(value).
//
// Use this for payload and content hashes where semantically identical JSON
// must produce the same digest regardless of object key order or Go map
// iteration order.
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
//	hash, err := CanonicalJSONHash(map[string]any{"id": identifier})
//	if err != nil {
//		return err
//	}
//	event.ContentHash = hash
func CanonicalJSONHash(value any) (string, error) {
	return common.CanonicalJSONHash(value)
}

// ComputeHistoryRowHash returns the hash-chain row hash for a history event.
//
// The row hash covers the entity identity, change metadata, payload/content
// hashes, previous row hash, and audit metadata. Persisted history rows use this
// value to detect tampering and to link each row to the previous version of the
// same entity.
//
// Parameters:
//   - event: Normalized history event metadata and hash inputs.
//
// Returns:
//   - string: Lowercase hexadecimal row hash.
//   - error: Error when the event cannot be encoded as canonical JSON.
//
// Example:
//
//	rowHash, err := ComputeHistoryRowHash(event)
//	if err != nil {
//		return err
//	}
//	event.RowHash = rowHash
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
//
// The encoder normalizes raw JSON, byte slices, maps, slices, and scalar values
// before writing JSON with sorted object keys. It preserves JSON number text so
// hashes remain stable across database round trips for large integers and other
// values that should not be coerced to float64.
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
//	canonical, err := CanonicalJSON(snapshot)
//	if err != nil {
//		return nil, err
//	}
//	return canonical, nil
func CanonicalJSON(value any) ([]byte, error) {
	return common.CanonicalJSON(value)
}

func decodeJSONPreservingNumbers(raw []byte, target any) error {
	return common.DecodeJSONPreservingNumbers(raw, target)
}

func decodeNormalizedJSON(raw []byte) (any, error) {
	var normalized any
	if err := decodeJSONPreservingNumbers(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
