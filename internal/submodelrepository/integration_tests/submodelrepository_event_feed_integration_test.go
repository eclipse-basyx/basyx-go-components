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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubmodelRepositoryEventFeedDisabledByDefault(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, path := range []string{"/events", "/.well-known/event-feed.json"} {
		resp, err := client.Get(submodelRepositoryBaseURL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestSubmodelRepositoryEventFeedIgnoresNoOpPuts(t *testing.T) {
	baseURL := submodelRepositoryEventFeedBaseURL
	smID := fmt.Sprintf("urn:example:event-feed:noop:sm:%d", time.Now().UnixNano())
	encodedSMID := base64.RawURLEncoding.EncodeToString([]byte(smID))
	client := &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, baseURL+"/submodels/"+encodedSMID, nil)
		if err != nil {
			t.Logf("cleanup request: %v", err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
			return
		}
		_ = resp.Body.Close()
	})

	submodel := func(idShort string) string {
		return fmt.Sprintf(`{
			"id": %q,
			"idShort": %q,
			"modelType": "Submodel",
			"kind": "Instance",
			"submodelElements": []
		}`, smID, idShort)
	}

	postReq, err := http.NewRequest(http.MethodPost, baseURL+"/submodels", bytes.NewReader([]byte(submodel("NoOpPutITSM"))))
	require.NoError(t, err)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := client.Do(postReq)
	require.NoError(t, err)
	_ = postResp.Body.Close()
	require.Equal(t, http.StatusCreated, postResp.StatusCode)

	putSubmodel := func(idShort string) {
		t.Helper()
		req, reqErr := http.NewRequest(http.MethodPut, baseURL+"/submodels/"+encodedSMID, bytes.NewReader([]byte(submodel(idShort))))
		require.NoError(t, reqErr)
		req.Header.Set("Content-Type", "application/json")
		resp, doErr := client.Do(req)
		require.NoError(t, doErr)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
	}

	for range 2 {
		putSubmodel("NoOpPutITSM")
	}

	// publish_seq assignment runs on a background ticker, so a just-written
	// event isn't necessarily visible through the feed instantly - poll
	// until it shows up rather than asserting on the first response.
	created, updated := waitForSubmodelFeedEventCounts(t, client, baseURL, smID, 1, 0, 5*time.Second)
	require.Equal(t, 1, created, "expected exactly one submodel.created event")
	require.Equal(t, 0, updated, "identical PUTs must not emit submodel.updated events")

	putSubmodel("NoOpPutITSMChanged")

	created, updated = waitForSubmodelFeedEventCounts(t, client, baseURL, smID, 1, 1, 5*time.Second)
	require.Equal(t, 1, created, "expected exactly one submodel.created event")
	require.Equal(t, 1, updated, "a content change must still emit exactly one submodel.updated event")
}

// waitForSubmodelFeedEventCounts polls the event feed until at least
// wantCreated created and wantUpdated updated events for subject are
// visible, or timeout elapses, returning whatever counts it last observed.
// Necessary because publish_seq assignment (see docu/user/event_feed.md)
// runs on a background interval, so a just-written event is not guaranteed
// to be visible through the feed immediately.
func waitForSubmodelFeedEventCounts(t *testing.T, client *http.Client, baseURL, subject string, wantCreated, wantUpdated int, timeout time.Duration) (created, updated int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		created, updated = countSubmodelFeedEventTypes(t, client, baseURL, subject)
		if created >= wantCreated && updated >= wantUpdated {
			return created, updated
		}
		if time.Now().After(deadline) {
			return created, updated
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func countSubmodelFeedEventTypes(t *testing.T, client *http.Client, baseURL string, subject string) (created int, updated int) {
	t.Helper()
	eventsURL := baseURL + "/events?" + url.Values{
		"limit":  []string{"100"},
		"filter": []string{"rsql:event.subject=='" + subject + "'"},
	}.Encode()

	resp, err := client.Get(eventsURL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var feed struct {
		Records []struct {
			Type    string `json:"type"`
			Subject string `json:"subject"`
		} `json:"records"`
	}
	require.NoError(t, json.Unmarshal(body, &feed))
	for _, record := range feed.Records {
		if record.Subject != subject {
			continue
		}
		switch record.Type {
		case "io.admin-shell.submodel.created.v1":
			created++
		case "io.admin-shell.submodel.updated.v1":
			updated++
		}
	}
	return created, updated
}

func TestSubmodelRepositoryEventFeedCreateAndRead(t *testing.T) {
	baseURL := submodelRepositoryEventFeedBaseURL
	smID := fmt.Sprintf("urn:example:event-feed:sm:%d", time.Now().UnixNano())
	encodedSMID := base64.RawURLEncoding.EncodeToString([]byte(smID))
	client := &http.Client{Timeout: 10 * time.Second}
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, baseURL+"/submodels/"+encodedSMID, nil)
		if err != nil {
			t.Logf("cleanup request: %v", err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", resp.StatusCode)
		}
	})

	capsResp, err := client.Get(baseURL + "/.well-known/event-feed.json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, capsResp.StatusCode)
	_ = capsResp.Body.Close()

	createBody := fmt.Sprintf(`{
		"id": %q,
		"idShort": "EventFeedITSM",
		"modelType": "Submodel",
		"kind": "Instance",
		"submodelElements": []
	}`, smID)
	postReq, err := http.NewRequest(http.MethodPost, baseURL+"/submodels", bytes.NewReader([]byte(createBody)))
	require.NoError(t, err)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := client.Do(postReq)
	require.NoError(t, err)
	_ = postResp.Body.Close()
	require.Equal(t, http.StatusCreated, postResp.StatusCode)

	// publish_seq assignment runs on a background ticker, so poll until the
	// created event is visible rather than asserting on the first response.
	created, _ := waitForSubmodelFeedEventCounts(t, client, baseURL, smID, 1, 0, 5*time.Second)
	require.Equal(t, 1, created, "missing submodel.created event for %s", smID)
}
