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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func chainedEvents(t *testing.T, count int) []AuditEvent {
	t.Helper()
	events := make([]AuditEvent, 0, count)
	previous := ""
	for index := range count {
		event := AuditEvent{
			ID: int64(index + 1), OccurredAt: time.Date(2026, 9, 26, 10, 0, index, 1000, time.UTC),
			Type: AuditGrantsChanged, Actor: UserKey(testIssuer, "alice"), Object: ResourceKey(TypeSubmodel, testSubmodelUUID),
			Details: json.RawMessage(`{"added":[{"relation":"viewer","subject":"user:bob"}],"removed":[]}`), PreviousHash: previous,
		}
		hash, err := event.computeHash()
		require.NoError(t, err)
		event.Hash = hash
		previous = hash
		events = append(events, event)
	}
	return events
}

func TestAuditHashesAreIndependentOfJSONFormatting(t *testing.T) {
	t.Parallel()

	event := chainedEvents(t, 1)[0]
	reformatted := event
	reformatted.Details = json.RawMessage("{ \"removed\": [], \"added\": [ {\"subject\": \"user:bob\", \"relation\": \"viewer\"} ] }")
	hash, err := reformatted.computeHash()
	require.NoError(t, err)
	require.Equal(t, event.Hash, hash, "PostgreSQL may store JSONB in another key order")
}

func TestAuditVerificationDetectsTamperingAndReordering(t *testing.T) {
	t.Parallel()

	events := chainedEvents(t, 3)
	verify := func(events []AuditEvent) string {
		report := AuditVerification{}
		previous := ""
		for _, event := range events {
			reason, err := verifyAuditEvent(t.Context(), nil, event, previous, &report)
			require.NoError(t, err)
			if reason != "" {
				return reason
			}
			previous = event.Hash
		}
		return ""
	}
	require.Empty(t, verify(events))

	tampered := append([]AuditEvent(nil), events...)
	tampered[1].Details = json.RawMessage(`{"added":[{"relation":"owner","subject":"user:mallory"}],"removed":[]}`)
	require.Equal(t, "the event content does not match its hash", verify(tampered))

	require.Equal(t, "the event does not follow its predecessor", verify([]AuditEvent{events[0], events[2]}), "removed events break the chain")

	renumbered := append([]AuditEvent(nil), events...)
	renumbered[2].ID = 7
	require.NotEmpty(t, verify(renumbered), "event ids are part of the hash")
}
