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
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestAASRepositoryEventFeedDisabledByDefault(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, path := range []string{"/events", "/.well-known/event-feed.json"} {
		resp, err := client.Get(aasRepositoryBaseURL + path)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode, path)
	}
}

func TestAASRepositoryEventFeedCreateAndRead(t *testing.T) {
	baseURL := aasRepositoryEventFeedBaseURL
	aasID := fmt.Sprintf("urn:example:event-feed:aas:%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	t.Cleanup(func() {
		status, err := deleteResponseStatus(baseURL + "/shells/" + encodedAASID)
		if err != nil {
			t.Logf("cleanup delete failed: %v", err)
		} else if status != http.StatusNoContent && status != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d", status)
		}
	})

	capsResp, err := http.Get(baseURL + "/.well-known/event-feed.json")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, capsResp.StatusCode)
	_ = capsResp.Body.Close()

	createBody := fmt.Sprintf(`{
		"id": %q,
		"idShort": "EventFeedITAAS",
		"modelType": "AssetAdministrationShell",
		"assetInformation": {"assetKind": "Instance", "globalAssetId": "urn:example:event-feed:asset"}
	}`, aasID)
	status, err := postResponseStatus(baseURL+"/shells", createBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	client := &http.Client{Timeout: 10 * time.Second}
	eventsURL := baseURL + "/events?" + url.Values{
		"limit":  []string{"100"},
		"filter": []string{"rsql:event.subject=='" + aasID + "'"},
	}.Encode()
	req, err := http.NewRequest(http.MethodGet, eventsURL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
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
		if rec.Subject == aasID && rec.Type == "io.admin-shell.aas.created.v1" {
			foundCreated = true
			break
		}
	}
	require.True(t, foundCreated, "missing aas.created event: %s", body)

	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var plan string
	require.NoError(t, db.QueryRow(`EXPLAIN SELECT seq FROM feed_events WHERE event_type = $1 ORDER BY seq ASC`, "io.admin-shell.aas.created.v1").Scan(&plan))
	require.NotEmpty(t, plan)
}
