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

	eventsURL := baseURL + "/events?" + url.Values{
		"limit":  []string{"100"},
		"filter": []string{"rsql:event.subject=='" + smID + "'"},
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
	require.NotEmpty(t, feed.Records)
	foundCreated := false
	for _, rec := range feed.Records {
		if rec.Subject == smID && rec.Type == "io.admin-shell.submodel.created.v1" {
			foundCreated = true
			break
		}
	}
	require.True(t, foundCreated, "missing submodel.created event: %s", body)
}
