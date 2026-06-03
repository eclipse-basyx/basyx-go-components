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

//nolint:all
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestConceptDescriptionRepositoryRecentChanges(t *testing.T) {
	const changedAfter = "2029-01-01T00:00:00Z"
	baseURL := "http://localhost:6004"
	conceptDescriptionID := fmt.Sprintf("urn:example:cd:recent:%d", time.Now().UnixNano())
	encodedID := base64.RawURLEncoding.EncodeToString([]byte(conceptDescriptionID))
	t.Cleanup(func() {
		status, _, err := requestJSON(http.MethodDelete, baseURL+"/concept-descriptions/"+encodedID, nil)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
			return
		}
		if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	status, body, err := requestJSON(http.MethodPost, baseURL+"/concept-descriptions", map[string]any{
		"id":        conceptDescriptionID,
		"idShort":   "RecentConceptV1",
		"modelType": "ConceptDescription",
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": "2030-01-02T03:04:06Z",
		},
	})
	if err != nil {
		t.Fatalf("failed to create concept description: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", status, string(body))
	}

	status, body, err = requestJSON(http.MethodPut, baseURL+"/concept-descriptions/"+encodedID, map[string]any{
		"id":        conceptDescriptionID,
		"idShort":   "RecentConceptV2",
		"modelType": "ConceptDescription",
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": "2030-01-02T03:04:07Z",
		},
	})
	if err != nil {
		t.Fatalf("failed to update concept description: %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", status, string(body))
	}

	recentURL := baseURL + "/concept-descriptions/$recent-changes?limit=10&updatedFrom=" + url.QueryEscape(changedAfter)
	status, body, err = requestJSON(http.MethodGet, recentURL, nil)
	if err != nil {
		t.Fatalf("failed to read recent changes: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, string(body))
	}

	var payload map[string]any
	if err = json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode recent changes: %v", err)
	}
	requireConceptDescriptionRecentChanges(t, payload, conceptDescriptionID, 2)
}

func requireConceptDescriptionRecentChanges(t *testing.T, payload map[string]any, id string, minimumCount int) {
	t.Helper()
	result, ok := payload["result"].([]any)
	if !ok {
		t.Fatalf("expected result array, got %#v", payload["result"])
	}
	count := 0
	for _, entry := range result {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if item["id"] == id {
			if len(item) != 4 || item["type"] == "" || item["createdAt"] == "" || item["updatedAt"] == "" {
				t.Fatalf("expected concept description recent-change payload, got %#v", item)
			}
			count++
		}
	}
	if count < minimumCount {
		t.Fatalf("expected at least %d concept description recent changes for id=%s in payload: %#v", minimumCount, id, payload)
	}
}
