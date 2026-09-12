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
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/amqp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestAMQPOnlyPublishesWithSchemasAndNoFeed(t *testing.T) {
	base := "http://" + testenv.AMQPAddress("BASYX_IT_AMQP_API_PORT")
	require.NoError(t, testenv.WaitHealthyURL(base+"/health", 150*time.Second))
	received := testenv.SubscribeAMQP(t)
	id := fmt.Sprintf("urn:amqp:only:%d", time.Now().UnixNano())
	endpoint := base + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(id))
	payload := []byte(fmt.Sprintf(`{"modelType":"Submodel","id":%q,"submodelElements":[]}`, id))
	mqttRequest(t, http.MethodPost, base+"/submodels", payload, http.StatusCreated)
	event := received.AwaitSubject(t, id, events.TypeSubmodelCreated)
	require.Equal(t, "1.0", event.SpecVersion)
	require.NotEmpty(t, event.ID)
	mqttRequest(t, http.MethodGet, base+"/events", nil, http.StatusNotFound)
	mqttRequest(t, http.MethodGet, base+events.SchemaPath+"/metamodel-submodelChangeEvent.v1.schema.json", nil, http.StatusOK)
	mqttRequest(t, http.MethodPut, endpoint, payload, http.StatusNoContent)
	received.AssertNoEvent(t, id, events.TypeSubmodelUpdated)
	mqttRequest(t, http.MethodDelete, endpoint, nil, http.StatusNoContent)
	received.AwaitSubject(t, id, events.TypeSubmodelDeleted)
}

func amqpConfig() amqp.Config {
	return amqp.Config{Broker: "amqp://" + testenv.AMQPAddress("BASYX_IT_AMQP_PORT"), Address: "/exchanges/basyx.events", SinkID: fmt.Sprintf("amqp-test-%d", time.Now().UnixNano()), Username: "basyx", Password: "secret"}
}
func amqpPublisher(t *testing.T, cfg amqp.Config) *amqp.Publisher {
	t.Helper()
	p, err := amqp.NewPublisher(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		require.NoError(t, p.Stop(ctx))
	})
	return p
}
func amqpEnqueue(t *testing.T, db *sql.DB, cfg amqp.Config, key string, commit bool) events.FeedRecord {
	t.Helper()
	event, err := events.NewBuilder(events.DefaultConfig()).SubmodelUpdated("urn:"+key, "", nil)
	require.NoError(t, err)
	return amqpEnqueueEvent(t, db, cfg, key, event, commit)
}
func amqpEnqueueEvent(t *testing.T, db *sql.DB, cfg amqp.Config, key string, event events.FeedEvent, commit bool) events.FeedRecord {
	t.Helper()
	routing, err := amqp.Routing(cfg.Address)
	require.NoError(t, err)
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, eventoutbox.NewRepository(db).Enqueue(t.Context(), tx, cfg.SinkID, key, event, routing))
	if commit {
		require.NoError(t, tx.Commit())
	} else {
		require.NoError(t, tx.Rollback())
	}
	record, err := events.Record(event, false)
	require.NoError(t, err)
	return record
}

func TestAMQPAuthenticationAndTLS(t *testing.T) {
	received := testenv.SubscribeAMQP(t)
	for _, secure := range []bool{false, true} {
		cfg := amqpConfig()
		if secure {
			cfg.Broker = "amqps://" + testenv.AMQPAddress("BASYX_IT_AMQP_TLS_PORT")
			cfg.CAFile = testenv.AMQPCertificate("ca.pem")
			cfg.CertificateFile = testenv.AMQPCertificate("client.pem")
			cfg.KeyFile = testenv.AMQPCertificate("client-key.pem")
			cfg.HostName = "vhost:/"
		}
		p := amqpPublisher(t, cfg)
		event := amqpEnqueue(t, mqttTestDB(t), cfg, "auth:"+cfg.SinkID, true)
		_, err := eventoutbox.NewRepository(mqttTestDB(t)).DeliverOne(t.Context(), cfg.SinkID, p)
		require.NoError(t, err)
		received.AssertEvent(t, event)
	}
	cfg := amqpConfig()
	cfg.Password = "wrong-amqp-password"
	p := amqpPublisher(t, cfg)
	routing, err := amqp.Routing(cfg.Address)
	require.NoError(t, err)
	err = p.Publish(t.Context(), routing, []byte(`{}`))
	require.Error(t, err)
	require.NotContains(t, err.Error(), cfg.Password)
	cfg = amqpConfig()
	cfg.Broker = "amqps://" + testenv.AMQPAddress("BASYX_IT_AMQP_TLS_PORT")
	cfg.CAFile = testenv.AMQPCertificate("ca.pem")
	require.Error(t, amqpPublisher(t, cfg).Publish(t.Context(), routing, []byte(`{}`)))
}

func TestAMQPOutboxRollbackRestartAndConcurrentOrdering(t *testing.T) {
	db := mqttTestDB(t)
	cfg := amqpConfig()
	received := testenv.SubscribeAMQP(t)
	repository := eventoutbox.NewRepository(db)
	rolledBack := amqpEnqueue(t, db, cfg, "rollback", false)
	require.Zero(t, pendingCount(t, repository, cfg.SinkID))
	var records []events.FeedRecord
	for range 12 {
		records = append(records, amqpEnqueue(t, db, cfg, "ordered", true))
	}
	unrelated := amqpEnqueue(t, db, cfg, "unrelated", true)
	other := cfg
	other.SinkID += "-other"
	isolated := amqpEnqueue(t, db, other, "ordered", true)
	unavailable := cfg
	unavailable.Broker = "amqp://127.0.0.1:1"
	failed := amqpPublisher(t, unavailable)
	_, err := repository.DeliverOne(t.Context(), cfg.SinkID, failed)
	require.Error(t, err)
	require.EqualValues(t, 13, pendingCount(t, repository, cfg.SinkID))
	require.NoError(t, failed.Stop(t.Context()))
	p := amqpPublisher(t, cfg)
	worker, err := eventoutbox.Start(t.Context(), repository, cfg.SinkID, p)
	require.NoError(t, err)
	defer worker.Stop()
	require.Eventually(t, func() bool { return pendingCount(t, repository, cfg.SinkID) == 0 }, 30*time.Second, 100*time.Millisecond)
	previous := -1
	for _, event := range records {
		received.AssertEvent(t, event)
		index := slices.Index(received.Order, event.ID)
		require.Greater(t, index, previous)
		previous = index
	}
	received.AssertEvent(t, unrelated)
	received.AssertNoEvent(t, rolledBack.Subject, rolledBack.Type)
	require.EqualValues(t, 1, pendingCount(t, repository, other.SinkID))
	_, err = repository.DeliverOne(t.Context(), other.SinkID, p)
	require.NoError(t, err)
	received.AssertEvent(t, isolated)
}

func TestAMQPRejectionsKeepOutboxPending(t *testing.T) {
	db := mqttTestDB(t)
	for _, address := range []string{"/queues/missing", "/exchanges/unroutable"} {
		cfg := amqpConfig()
		cfg.Address = address
		amqpEnqueue(t, db, cfg, "unroutable", true)
		repository := eventoutbox.NewRepository(db)
		_, err := repository.DeliverOne(t.Context(), cfg.SinkID, amqpPublisher(t, cfg))
		require.Error(t, err)
		if address == "/exchanges/unroutable" {
			require.Contains(t, err.Error(), "AMQP-PUBLISH-OUTCOME")
		} else {
			require.Contains(t, err.Error(), "AMQP-PUBLISH-LINK")
		}
		require.EqualValues(t, 1, pendingCount(t, repository, cfg.SinkID))
	}
	cfg := amqpConfig()
	event, err := events.NewBuilder(events.DefaultConfig()).PCN(cfg.SinkID, nil, map[string]any{"description": strings.Repeat("x", 3<<20)})
	require.NoError(t, err)
	amqpEnqueueEvent(t, db, cfg, "large", event, true)
	amqpEnqueue(t, db, cfg, "large", true)
	repository := eventoutbox.NewRepository(db)
	_, err = repository.DeliverOne(t.Context(), cfg.SinkID, amqpPublisher(t, cfg))
	require.Error(t, err)
	require.EqualValues(t, 2, pendingCount(t, repository, cfg.SinkID))
}

func TestAMQPLargePCNDelivery(t *testing.T) {
	cfg := amqpConfig()
	db := mqttTestDB(t)
	received := testenv.SubscribeAMQP(t)
	event, err := events.NewBuilder(events.DefaultConfig()).PCN(cfg.SinkID, nil, map[string]any{"description": strings.Repeat("x", 1200000)})
	require.NoError(t, err)
	record := amqpEnqueueEvent(t, db, cfg, "large-pcn", event, true)
	_, err = eventoutbox.NewRepository(db).DeliverOne(t.Context(), cfg.SinkID, amqpPublisher(t, cfg))
	require.NoError(t, err)
	received.AssertEvent(t, record)
}

func TestAMQPBrokerOutageKeepsOtherSinksAndWritesAvailable(t *testing.T) {
	received := testenv.SubscribeAMQP(t)
	kafkaEvents := testenv.SubscribeKafka(t)
	mqttEvents := mqttSubscribe(t, "basyx/#")
	command := func(action string) { t.Helper(); amqpBrokerCommand(t, action) }
	command("pause")
	paused := true
	defer func() {
		if paused {
			command("unpause")
		}
	}()
	p := amqpPublisher(t, amqpConfig())
	routing, err := amqp.Routing("/exchanges/basyx.events")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	require.Error(t, p.Publish(ctx, routing, []byte(`{}`)))
	require.Less(t, time.Since(started), 2*time.Second)
	stop, cancelStop := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelStop()
	require.NoError(t, p.Stop(stop))
	id := fmt.Sprintf("urn:amqp:outage:%d", time.Now().UnixNano())
	base := submodelRepositoryEventFeedBaseURL
	mqttRequest(t, http.MethodPost, base+"/submodels", []byte(fmt.Sprintf(`{"id":%q,"modelType":"Submodel","submodelElements":[]}`, id)), http.StatusCreated)
	event := awaitMQTT(t, mqttEvents, id, events.TypeSubmodelCreated).Event
	assertMQTTFeedParity(t, base, event)
	kafkaEvents.AssertEvent(t, event, "submodel_history:"+id)
	command("unpause")
	paused = false
	received.AssertEvent(t, event)
	mqttRequest(t, http.MethodDelete, base+"/submodels/"+base64.RawURLEncoding.EncodeToString([]byte(id)), nil, http.StatusNoContent)
}

func TestAMQPReconnectsAfterBrokerRestartWithStoredRouting(t *testing.T) {
	cfg := amqpConfig()
	db := mqttTestDB(t)
	repository := eventoutbox.NewRepository(db)
	received := testenv.SubscribeAMQP(t)
	first := amqpEnqueue(t, db, cfg, "restart", true)
	// The stored destination must win even when the live configuration has changed.
	changed := cfg
	changed.Address = "/queues/does-not-exist"
	publisher := amqpPublisher(t, changed)
	_, err := repository.DeliverOne(t.Context(), cfg.SinkID, publisher)
	require.NoError(t, err)
	received.AssertEvent(t, first)
	amqpBrokerCommand(t, "restart")
	// Queues are non-durable test subscribers, so attach a fresh receiver after restart.
	afterRestart := testenv.SubscribeAMQP(t)
	next := amqpEnqueue(t, db, cfg, "restart", true)
	worker, err := eventoutbox.Start(t.Context(), repository, cfg.SinkID, publisher)
	require.NoError(t, err)
	defer worker.Stop()
	require.Eventually(t, func() bool { return pendingCount(t, repository, cfg.SinkID) == 0 }, 30*time.Second, 100*time.Millisecond)
	afterRestart.AssertEvent(t, next)
}

func amqpBrokerCommand(t *testing.T, action string) {
	t.Helper()
	// #nosec G204 G702 -- the container is selected by the allocated test broker port.
	output, err := exec.CommandContext(t.Context(), "docker", "ps", "--filter", "publish="+os.Getenv("BASYX_IT_AMQP_PORT"), "--format", "{{.ID}}").Output()
	require.NoError(t, err)
	container := strings.TrimSpace(string(output))
	require.NotEmpty(t, container)
	require.NotContains(t, container, "\n")
	// #nosec G204 G702 -- action is a fixed test lifecycle operation, never user input.
	require.NoError(t, exec.CommandContext(t.Context(), "docker", action, container).Run())
	if action == "restart" {
		require.Eventually(t, func() bool {
			// #nosec G204 G702 -- only the resolved test broker container is inspected.
			result, err := exec.CommandContext(t.Context(), "docker", "inspect", "--format", "{{.State.Health.Status}}", container).Output()
			return err == nil && strings.TrimSpace(string(result)) == "healthy"
		}, 45*time.Second, time.Second)
	}
}
