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
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanonicalJSONHashIsStableForObjectKeyOrder(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"b": []any{"x", "y"},
		"a": map[string]any{"z": float64(1), "c": true},
	}
	right := map[string]any{
		"a": map[string]any{"c": true, "z": float64(1)},
		"b": []any{"x", "y"},
	}

	leftHash, err := CanonicalJSONHash(left)
	require.NoError(t, err)
	rightHash, err := CanonicalJSONHash(right)
	require.NoError(t, err)

	require.Equal(t, leftHash, rightHash)
}

func TestCanonicalJSONHashIsStableAcrossJSONRoundTripForTypedValues(t *testing.T) {
	t.Parallel()

	type typedInner struct {
		Second string `json:"second"`
		First  string `json:"first"`
	}
	type typedOuter struct {
		Metadata typedInner   `json:"metadata"`
		Items    []typedInner `json:"items"`
	}

	original := map[string]any{
		"id": "aas-1",
		"typed": typedOuter{
			Metadata: typedInner{Second: "B", First: "A"},
			Items:    []typedInner{{Second: "D", First: "C"}},
		},
	}

	originalHash, err := CanonicalJSONHash(original)
	require.NoError(t, err)

	raw, err := json.Marshal(original)
	require.NoError(t, err)

	var restored map[string]any
	err = json.Unmarshal(raw, &restored)
	require.NoError(t, err)

	restoredHash, err := CanonicalJSONHash(restored)
	require.NoError(t, err)

	require.Equal(t, originalHash, restoredHash)
}

func TestDatabaseTimestampRoundsToPostgreSQLMicrosecondPrecision(t *testing.T) {
	t.Parallel()

	value := time.Date(2026, 5, 28, 12, 0, 0, 123456789, time.FixedZone("CET", 3600))

	normalized := databaseTimestamp(value)

	require.Equal(t, time.Date(2026, 5, 28, 11, 0, 0, 123457000, time.UTC), normalized)
}

func TestComputeHistoryRowHashIncludesPreviousHash(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		Deleted:      false,
		ContentHash:  "content",
		PreviousHash: "previous-a",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.PreviousHash = "previous-b"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}

func TestComputeHistoryRowHashIncludesAuditMetadata(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		ContentHash:  "content",
		ActorSubject: "subject-a",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.ActorSubject = "subject-b"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}

func TestComputeHistoryRowHashIncludesAuthorizationAndSourceMetadata(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:          TableAAS,
		Identifier:          "aas-1",
		ChangeType:          ChangeUpdated,
		Timestamp:           time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		ContentHash:         "content",
		AuthorizationResult: "allow",
		PolicyID:            "policy-a",
		MatchedRuleID:       "rule-a",
		SourceIP:            "192.0.2.10",
		UserAgent:           "client-a",
		Operation:           "PutAssetAdministrationShellById",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.AuthorizationResult = "deny"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}

func TestComputeHistoryRowHashIncludesPayloadMetadata(t *testing.T) {
	t.Parallel()

	event := ChangeEvent{
		EntityType:   TableAAS,
		Identifier:   "aas-1",
		ChangeType:   ChangeUpdated,
		Timestamp:    time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
		PayloadType:  PayloadTypeSnapshot,
		ContentHash:  "content",
		PayloadHash:  "payload-a",
		PreviousHash: "previous",
	}
	hashA, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	event.PayloadHash = "payload-b"
	hashB, err := ComputeHistoryRowHash(event)
	require.NoError(t, err)

	require.NotEqual(t, hashA, hashB)
}
