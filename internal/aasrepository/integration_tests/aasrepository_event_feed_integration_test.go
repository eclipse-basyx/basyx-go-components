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
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
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

func TestAASRepositoryEventFeedIgnoresNoOpPuts(t *testing.T) {
	received := testenv.SubscribeMQTT(t, mqttBrokerURL, "basyx/#")
	kafkaEvents := testenv.SubscribeKafka(t)
	amqpEvents := testenv.SubscribeAMQP(t)
	baseURL := aasRepositoryEventFeedBaseURL
	aasID := fmt.Sprintf("urn:example:event-feed:noop:aas:%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	t.Cleanup(func() {
		if _, err := deleteResponseStatus(baseURL + "/shells/" + encodedAASID); err != nil {
			t.Logf("cleanup delete failed: %v", err)
		}
	})

	shell := func(idShort string) string {
		return fmt.Sprintf(`{
			"id": %q,
			"idShort": %q,
			"modelType": "AssetAdministrationShell",
			"assetInformation": {"assetKind": "Instance", "globalAssetId": "urn:example:event-feed:noop:asset"}
		}`, aasID, idShort)
	}

	status, err := postResponseStatus(baseURL+"/shells", shell("NoOpPutITAAS"))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	for range 2 {
		putStatus, putErr := putResponseStatus(baseURL+"/shells/"+encodedAASID, shell("NoOpPutITAAS"))
		require.NoError(t, putErr)
		require.Equal(t, http.StatusNoContent, putStatus)
	}

	// publish_seq assignment runs on a background ticker, so a just-written
	// event isn't necessarily visible through the feed instantly - poll
	// until it shows up rather than asserting on the first response.
	created, updated := waitForAASFeedEventCounts(t, baseURL, aasID, 1, 0, 5*time.Second)
	require.Equal(t, 1, created, "expected exactly one aas.created event")
	require.Equal(t, 0, updated, "identical PUTs must not emit aas.updated events")

	putStatus, err := putResponseStatus(baseURL+"/shells/"+encodedAASID, shell("NoOpPutITAASChanged"))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, putStatus)

	created, updated = waitForAASFeedEventCounts(t, baseURL, aasID, 1, 1, 5*time.Second)
	require.Equal(t, 1, created, "expected exactly one aas.created event")
	require.Equal(t, 1, updated, "a content change must still emit exactly one aas.updated event")
	event1 := testenv.AwaitMQTT(t, received, aasID, "io.admin-shell.aas.created.v1").Event
	kafkaEvents.AssertEvent(t, event1, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event1)
	event2 := testenv.AwaitMQTT(t, received, "urn:example:event-feed:noop:asset", "io.admin-shell.asset.created.v1").Event
	kafkaEvents.AssertEvent(t, event2, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event2)
	event3 := testenv.AwaitMQTT(t, received, aasID, "io.admin-shell.aas.updated.v1").Event
	kafkaEvents.AssertEvent(t, event3, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event3)
	event4 := testenv.AwaitMQTT(t, received, "urn:example:event-feed:noop:asset", "io.admin-shell.asset.updated.v1").Event
	kafkaEvents.AssertEvent(t, event4, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event4)
	status, err = deleteResponseStatus(baseURL + "/shells/" + encodedAASID)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)
	event5 := testenv.AwaitMQTT(t, received, aasID, "io.admin-shell.aas.deleted.v1").Event
	kafkaEvents.AssertEvent(t, event5, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event5)
	event6 := testenv.AwaitMQTT(t, received, "urn:example:event-feed:noop:asset", "io.admin-shell.asset.deleted.v1").Event
	kafkaEvents.AssertEvent(t, event6, "aas_history:"+aasID)
	amqpEvents.AssertEvent(t, event6)
}

// waitForAASFeedEventCounts polls the event feed until at least wantCreated
// created and wantUpdated updated events for subject are visible, or
// timeout elapses, returning whatever counts it last observed. Necessary
// because publish_seq assignment (see docu/user/event_feed.md) runs on a
// background interval, so a just-written event is not guaranteed to be
// visible through the feed immediately.
func waitForAASFeedEventCounts(t *testing.T, baseURL, subject string, wantCreated, wantUpdated int, timeout time.Duration) (created, updated int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		created, updated = countAASFeedEventTypes(t, baseURL, subject)
		if created >= wantCreated && updated >= wantUpdated {
			return created, updated
		}
		if time.Now().After(deadline) {
			return created, updated
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func countAASFeedEventTypes(t *testing.T, baseURL string, subject string) (created int, updated int) {
	t.Helper()
	eventsURL := baseURL + "/events?" + url.Values{
		"limit":  []string{"100"},
		"filter": []string{"rsql:event.subject=='" + subject + "'"},
	}.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
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
		case "io.admin-shell.aas.created.v1":
			created++
		case "io.admin-shell.aas.updated.v1":
			updated++
		}
	}
	return created, updated
}

func putResponseStatus(endpoint string, body string) (int, error) {
	req, err := http.NewRequest(http.MethodPut, endpoint, strings.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
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

	// publish_seq assignment runs on a background ticker, so poll until the
	// created event is visible rather than asserting on the first response.
	created, _ := waitForAASFeedEventCounts(t, baseURL, aasID, 1, 0, 5*time.Second)
	require.Equal(t, 1, created, "missing aas.created event for %s", aasID)

	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var plan string
	require.NoError(t, db.QueryRow(`EXPLAIN SELECT seq FROM feed_events WHERE event_type = $1 ORDER BY seq ASC`, "io.admin-shell.aas.created.v1").Scan(&plan))
	require.NotEmpty(t, plan)
}
