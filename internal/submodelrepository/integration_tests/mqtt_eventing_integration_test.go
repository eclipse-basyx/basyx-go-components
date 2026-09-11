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

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

var (
	submodelRepositoryMQTTContainer = os.Getenv("BASYX_IT_MQTT_CONTAINER")
	submodelRepositoryMQTTURL       = mqttTestEnv("BASYX_IT_MQTT_URL", "mqtt://127.0.0.1:61883")
	submodelRepositoryMQTTAuthURL   = mqttTestEnv("BASYX_IT_MQTT_AUTH_URL", "mqtt://127.0.0.1:61884")
	submodelRepositoryMQTTTLSURL    = mqttTestEnv("BASYX_IT_MQTT_TLS_URL", "tls://127.0.0.1:61885")
	submodelRepositoryMQTTOnlyURL   = mqttTestEnv("BASYX_IT_MQTT_API_URL", "http://127.0.0.1:6034")
)

func mqttTestEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

type mqttReceived = testenv.MQTTMessage

func mqttSubscribe(t *testing.T, topic string) <-chan mqttReceived {
	return testenv.SubscribeMQTT(t, submodelRepositoryMQTTURL, topic)
}
func awaitMQTT(t *testing.T, received <-chan mqttReceived, subject, kind string) mqttReceived {
	return testenv.AwaitMQTT(t, received, subject, kind)
}
func mqttRequest(t *testing.T, method, target string, body []byte, status int) []byte {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, status, response.StatusCode, string(data))
	return data
}
func TestMQTTMatchesFeedAndSupportsMQTTOnly(t *testing.T) {
	received := mqttSubscribe(t, "basyx/#")
	for _, base := range []string{submodelRepositoryEventFeedBaseURL, submodelRepositoryMQTTOnlyURL} {
		id := fmt.Sprintf("urn:mqtt:sm:%d", time.Now().UnixNano())
		endpoint := base + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(id))
		payload := []byte(fmt.Sprintf(`{"id":%q,"modelType":"Submodel","submodelElements":[]}`, id))
		mqttRequest(t, http.MethodPost, base+"/submodels", payload, http.StatusCreated)
		t.Cleanup(func() { _, _ = sendReconciliationRequest(t, http.MethodDelete, endpoint, nil) })
		created := awaitMQTT(t, received, id, events.TypeSubmodelCreated)
		require.Equal(t, "application/cloudevents+json", created.ContentType)
		require.Equal(t, "application/json", created.Event.DataContentType)
		require.Equal(t, "basyx/submodelrepository/submodel/created", created.Topic)
		require.False(t, created.Retained)
		if base == submodelRepositoryEventFeedBaseURL {
			assertMQTTFeedParity(t, base, created.Event)
		} else {
			mqttRequest(t, http.MethodGet, base+"/events", nil, http.StatusNotFound)
			mqttRequest(t, http.MethodGet, base+events.SchemaPath+"/metamodel-submodelChangeEvent.v1.schema.json", nil, http.StatusOK)
		}
		mqttRequest(t, http.MethodPut, endpoint, payload, http.StatusNoContent)
		mqttRequest(t, http.MethodGet, endpoint, nil, http.StatusOK)
		property := []byte(`{"modelType":"Property","idShort":"temperature","valueType":"xs:string","value":"20"}`)
		mqttRequest(t, http.MethodPost, endpoint+"/submodel-elements", property, http.StatusCreated)
		updated := awaitMQTT(t, received, id, events.TypeSubmodelUpdated)
		if base == submodelRepositoryEventFeedBaseURL {
			assertMQTTFeedParity(t, base, updated.Event)
		}
		mqttRequest(t, http.MethodDelete, endpoint, nil, http.StatusNoContent)
		deleted := awaitMQTT(t, received, id, events.TypeSubmodelDeleted)
		require.NotEqual(t, updated.Event.ID, deleted.Event.ID)
		if base == submodelRepositoryEventFeedBaseURL {
			assertMQTTFeedParity(t, base, deleted.Event)
		}
	}
}
func assertMQTTFeedParity(t *testing.T, base string, event events.FeedRecord) {
	t.Helper()
	var found *events.FeedRecord
	require.Eventually(t, func() bool {
		raw := mqttRequest(t, http.MethodGet, base+"/events?filter="+url.QueryEscape("rsql:event.subject=='"+event.Subject+"'"), nil, http.StatusOK)
		var page struct {
			Records []events.FeedRecord `json:"records"`
		}
		require.NoError(t, json.Unmarshal(raw, &page))
		for _, record := range page.Records {
			if record.ID == event.ID {
				found = &record
				return true
			}
		}
		return false
	}, 10*time.Second, 100*time.Millisecond)
	require.Equal(t, event, *found)
}

type outboxTestPublisher func(context.Context, json.RawMessage, []byte) error

func (p outboxTestPublisher) Publish(ctx context.Context, routing json.RawMessage, envelope []byte) error {
	return p(ctx, routing, envelope)
}
func mqttTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}
func enqueueTestEvent(t *testing.T, db *sql.DB, sink, key string, commit bool) events.FeedEvent {
	t.Helper()
	event, err := events.NewBuilder(events.DefaultConfig()).SubmodelUpdated("urn:"+key, "", nil)
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, eventoutbox.NewRepository(db).Enqueue(t.Context(), tx, sink, key, event, json.RawMessage(`"test/topic"`)))
	if commit {
		require.NoError(t, tx.Commit())
	} else {
		require.NoError(t, tx.Rollback())
	}
	return event
}
func pendingCount(t *testing.T, repository *eventoutbox.Repository, sink string) int64 {
	t.Helper()
	count, _, _, err := repository.Stats(t.Context(), sink)
	require.NoError(t, err)
	return count
}
func TestMQTTOutboxRollbackRetryRestartAndSinkIsolation(t *testing.T) {
	db := mqttTestDB(t)
	repository := eventoutbox.NewRepository(db)
	sink := fmt.Sprintf("test-%d", time.Now().UnixNano())
	enqueueTestEvent(t, db, sink, "rolled-back", false)
	require.Zero(t, pendingCount(t, repository, sink))
	event := enqueueTestEvent(t, db, sink, "ordered", true)
	enqueueTestEvent(t, db, sink+"-other", "ordered", true)
	attempts := []string{}
	publish := outboxTestPublisher(func(_ context.Context, _ json.RawMessage, raw []byte) error {
		var record events.FeedRecord
		require.NoError(t, json.Unmarshal(raw, &record))
		attempts = append(attempts, record.ID)
		if len(attempts) == 1 {
			return errors.New("TEST-PUBLISH-LOSTACK")
		}
		return nil
	})
	found, err := repository.DeliverOne(t.Context(), sink, "before-restart", publish)
	require.True(t, found)
	require.Error(t, err)
	require.EqualValues(t, 1, pendingCount(t, repository, sink))
	require.EqualValues(t, 1, pendingCount(t, repository, sink+"-other"))
	repository = eventoutbox.NewRepository(db)
	require.Eventually(t, func() bool {
		_, err = repository.DeliverOne(t.Context(), sink, "after-restart", publish)
		require.NoError(t, err)
		return pendingCount(t, repository, sink) == 0
	}, 3*time.Second, 100*time.Millisecond)
	require.Equal(t, []string{event.ID, event.ID}, attempts)
	require.EqualValues(t, 1, pendingCount(t, repository, sink+"-other"))
	_, err = repository.DeliverOne(t.Context(), sink+"-other", "other", publish)
	require.NoError(t, err)
}
func TestMQTTOutboxConcurrentWorkersPreserveEntityOrder(t *testing.T) {
	db := mqttTestDB(t)
	repository := eventoutbox.NewRepository(db)
	sink := fmt.Sprintf("concurrent-%d", time.Now().UnixNano())
	first := enqueueTestEvent(t, db, sink, "entity", true)
	second := enqueueTestEvent(t, db, sink, "entity", true)
	unrelated := enqueueTestEvent(t, db, sink, "unrelated", true)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	result := make(chan error, 1)
	go func() {
		_, err := repository.DeliverOne(t.Context(), sink, "one", outboxTestPublisher(func(ctx context.Context, _ json.RawMessage, raw []byte) error {
			var record events.FeedRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				return err
			}
			if record.ID != first.ID {
				return errors.New("TEST-ORDER-FIRST")
			}
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}))
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not claim first event")
	}
	var received string
	publish := outboxTestPublisher(func(_ context.Context, _ json.RawMessage, raw []byte) error {
		var record events.FeedRecord
		err := json.Unmarshal(raw, &record)
		received = record.ID
		return err
	})
	found, err := repository.DeliverOne(t.Context(), sink, "two", publish)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, unrelated.ID, received)
	found, err = repository.DeliverOne(t.Context(), sink, "two", publish)
	require.NoError(t, err)
	require.False(t, found)
	once.Do(func() { close(release) })
	require.NoError(t, <-result)
	found, err = repository.DeliverOne(t.Context(), sink, "two", publish)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, second.ID, received)
}
func TestMQTTPublisherAuthenticationTLSAndQoS(t *testing.T) {
	cases := []mqtt.Config{
		{Broker: submodelRepositoryMQTTURL, QoS: 0},
		{Broker: submodelRepositoryMQTTAuthURL, QoS: 1, Username: "integration", Password: "integration-password"},
		{Broker: submodelRepositoryMQTTTLSURL, QoS: 2, CAFile: "docker_compose/mqtt-ca.pem", CertificateFile: "docker_compose/mqtt-client.pem", KeyFile: "docker_compose/mqtt-client-key.pem"},
	}
	for _, cfg := range cases {
		cfg.ClientID = fmt.Sprintf("test-publisher-%d", time.Now().UnixNano())
		cfg.SinkID = "test"
		publisher, err := mqtt.NewPublisher(t.Context(), cfg)
		require.NoError(t, err)
		require.Eventually(t, publisher.Connected, 15*time.Second, 100*time.Millisecond)
		event, err := events.NewBuilder(events.DefaultConfig()).AASCreated("urn:test:authentication", "", nil)
		require.NoError(t, err)
		record, err := events.Record(event, false)
		require.NoError(t, err)
		raw, err := json.Marshal(record)
		require.NoError(t, err)
		deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		require.NoError(t, publisher.Publish(deadline, json.RawMessage(`"test/authentication"`), raw))
		cancel()
		require.NoError(t, publisher.Stop(t.Context()))
	}
}
func TestMQTTOutboxPendingEventsSurviveFeedRetention(t *testing.T) {
	db := mqttTestDB(t)
	repository := eventoutbox.NewRepository(db)
	sink := fmt.Sprintf("retention-%d", time.Now().UnixNano())
	event, err := events.NewBuilder(events.DefaultConfig()).SubmodelUpdated("urn:retention", "", nil)
	require.NoError(t, err)
	event.Time = event.Time.Add(-48 * time.Hour)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, repository.Enqueue(t.Context(), tx, sink, "retention", event, json.RawMessage(`"test/retention"`)))
	feedRepository := eventfeed.NewRepository(db, time.Hour)
	_, err = feedRepository.SaveTx(t.Context(), tx, event)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	cfg := eventfeed.DefaultConfig()
	cfg.Enabled = true
	cfg.MaxAge = time.Hour
	cfg.HardDeleteGrace = 0
	_, err = eventfeed.NewService(feedRepository, cfg).RunRetention(t.Context())
	require.NoError(t, err)
	_, found, err := feedRepository.FindByID(t.Context(), event.ID)
	require.NoError(t, err)
	require.False(t, found)
	require.EqualValues(t, 1, pendingCount(t, repository, sink))
	_, err = repository.DeliverOne(t.Context(), sink, "worker", outboxTestPublisher(func(context.Context, json.RawMessage, []byte) error { return nil }))
	require.NoError(t, err)
}

func TestMQTTBrokerOutageKeepsWritesAvailable(t *testing.T) {
	received := mqttSubscribe(t, "basyx/#")
	mqttBrokerCommand(t, "pause")
	paused := true
	t.Cleanup(func() {
		if paused {
			mqttBrokerCommand(t, "unpause")
		}
	})
	id := fmt.Sprintf("urn:mqtt:outage:%d", time.Now().UnixNano())
	base := submodelRepositoryMQTTOnlyURL
	endpoint := base + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(id))
	t.Cleanup(func() { _, _ = sendReconciliationRequest(t, http.MethodDelete, endpoint, nil) })
	started := time.Now()
	mqttRequest(t, http.MethodPost, base+"/submodels", []byte(fmt.Sprintf(`{"id":%q,"modelType":"Submodel","submodelElements":[]}`, id)), http.StatusCreated)
	require.Less(t, time.Since(started), 5*time.Second)
	mqttBrokerCommand(t, "unpause")
	paused = false
	awaitMQTT(t, received, id, events.TypeSubmodelCreated)
}
func mqttBrokerCommand(t *testing.T, command string) {
	t.Helper()
	require.Contains(t, []string{"pause", "unpause"}, command)
	require.NotEmpty(t, submodelRepositoryMQTTContainer)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
	defer cancel()
	// #nosec G204 -- the command is allow-listed and the container belongs to this test fixture.
	output, err := exec.CommandContext(ctx, "docker", command, submodelRepositoryMQTTContainer).CombinedOutput()
	require.NoError(t, err, string(output))
}
func TestMQTTOutboxWorkerCancellationReleasesClaim(t *testing.T) {
	db := mqttTestDB(t)
	repository := eventoutbox.NewRepository(db)
	sink := fmt.Sprintf("cancel-%d", time.Now().UnixNano())
	event := enqueueTestEvent(t, db, sink, "entity", true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	found, err := repository.DeliverOne(ctx, sink, "cancelled", outboxTestPublisher(func(context.Context, json.RawMessage, []byte) error { cancel(); return nil }))
	require.True(t, found)
	require.Error(t, err)
	require.EqualValues(t, 1, pendingCount(t, repository, sink))
	found, err = repository.DeliverOne(t.Context(), sink, "replacement", outboxTestPublisher(func(_ context.Context, _ json.RawMessage, raw []byte) error {
		var record events.FeedRecord
		require.NoError(t, json.Unmarshal(raw, &record))
		require.Equal(t, event.ID, record.ID)
		return nil
	}))
	require.True(t, found)
	require.NoError(t, err)
	require.Zero(t, pendingCount(t, repository, sink))
}
func TestMQTTRetainedDelivery(t *testing.T) {
	cfg := mqtt.Config{Broker: submodelRepositoryMQTTURL, ClientID: fmt.Sprintf("retain-%d", time.Now().UnixNano()), SinkID: "test", QoS: 1, Retained: true}
	publisher, err := mqtt.NewPublisher(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, publisher.Stop(ctx))
	})
	require.Eventually(t, publisher.Connected, 10*time.Second, 100*time.Millisecond)
	event, err := events.NewBuilder(events.DefaultConfig()).AASUpdated("urn:"+cfg.ClientID, "", nil)
	require.NoError(t, err)
	record, err := events.Record(event, false)
	require.NoError(t, err)
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	topic := "test/retained/" + cfg.ClientID
	routing, err := json.Marshal(topic)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, publisher.Publish(ctx, routing, raw))
	received := mqttSubscribe(t, topic)
	message := awaitMQTT(t, received, event.Subject, event.Type)
	require.True(t, message.Retained)
	require.Equal(t, record, message.Event)
	require.NoError(t, publisher.Publish(ctx, routing, nil))
}
