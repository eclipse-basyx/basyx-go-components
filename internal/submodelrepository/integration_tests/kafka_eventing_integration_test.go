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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventoutbox"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/kafka"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
)

func kafkaConfig() kafka.Config {
	return kafka.Config{Brokers: []string{testenv.KafkaAddress("BASYX_IT_KAFKA_PORT")}, Topic: "basyx.events", SinkID: fmt.Sprintf("kafka-test-%d", time.Now().UnixNano()), ClientID: "integration"}
}
func kafkaPublisher(t *testing.T, cfg kafka.Config) *kafka.Publisher {
	t.Helper()
	p, err := kafka.NewPublisher(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cancel()
		require.NoError(t, p.Stop(ctx))
	})
	return p
}
func kafkaEnqueue(t *testing.T, db *sql.DB, cfg kafka.Config, key string, commit bool) events.FeedRecord {
	t.Helper()
	event, err := events.NewBuilder(events.DefaultConfig()).SubmodelUpdated("urn:"+key, "", nil)
	require.NoError(t, err)
	return kafkaEnqueueEvent(t, db, cfg, key, event, commit)
}
func kafkaEnqueueEvent(t *testing.T, db *sql.DB, cfg kafka.Config, key string, event events.FeedEvent, commit bool) events.FeedRecord {
	t.Helper()
	routing, err := kafka.Routing(cfg.Topic, key)
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

func TestKafkaOnlyPublishesWithSchemasAndNoFeed(t *testing.T) {
	base := "http://" + testenv.KafkaAddress("BASYX_IT_KAFKA_API_PORT")
	require.NoError(t, testenv.WaitHealthyURL(base+"/health", 150*time.Second))
	received := testenv.SubscribeKafka(t)
	id := fmt.Sprintf("urn:kafka:only:%d", time.Now().UnixNano())
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

func TestKafkaAuthenticationAndTLS(t *testing.T) {
	received := testenv.SubscribeKafka(t)
	for _, mechanism := range []string{"", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		for _, tlsEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tls=%t", mechanism, tlsEnabled), func(t *testing.T) {
				cfg := kafkaAuthenticationConfig(mechanism, tlsEnabled)
				publishKafkaFixture(t, received, cfg)
			})
		}
	}
	assertKafkaAuthenticationFailure(t)
}

func assertKafkaAuthenticationFailure(t *testing.T) {
	t.Helper()
	logFile, err := os.CreateTemp(t.TempDir(), "kafka-errors")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logFile.Close()) })
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	cfg := kafkaConfig()
	cfg.Brokers = []string{testenv.KafkaAddress("BASYX_IT_KAFKA_AUTH_PORT")}
	cfg.SASLMechanism = "PLAIN"
	cfg.Username = "basyx"
	cfg.Password = "wrong"
	p := kafkaPublisher(t, cfg)
	routing, err := kafka.Routing(cfg.Topic, "invalid-auth")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	err = p.Publish(ctx, routing, []byte(`{}`))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotContains(t, err.Error(), "wrong")
	require.Eventually(t, func() bool {
		raw, readErr := os.ReadFile(logFile.Name())
		return readErr == nil && strings.Contains(string(raw), "SASL_AUTHENTICATION_FAILED")
	}, 5*time.Second, 50*time.Millisecond)
	raw, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	require.NotContains(t, string(raw), "wrong")
}

func TestKafkaLargePCNDelivery(t *testing.T) {
	db := mqttTestDB(t)
	cfg := kafkaConfig()
	received := testenv.SubscribeKafka(t)
	key := "submodel_history:" + cfg.SinkID
	event, err := events.NewBuilder(events.DefaultConfig()).PCN(cfg.SinkID, nil, map[string]any{"description": strings.Repeat("x", 1200000)})
	require.NoError(t, err)
	record, err := events.Record(event, false)
	require.NoError(t, err)
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	routing, err := kafka.Routing(cfg.Topic, key)
	require.NoError(t, err)
	t.Run("default limit reports oversized record", func(t *testing.T) {
		p := kafkaPublisher(t, cfg)
		require.ErrorIs(t, p.Publish(t.Context(), routing, raw), kerr.MessageTooLarge)
	})
	t.Run("configured limit drains large notification and its successor", func(t *testing.T) {
		require.NoError(t, json.Unmarshal([]byte(`{"producerBatchMaxBytes":2097152}`), &cfg))
		p := kafkaPublisher(t, cfg)
		large := kafkaEnqueueEvent(t, db, cfg, key, event, true)
		next := kafkaEnqueue(t, db, cfg, key, true)
		repository := eventoutbox.NewRepository(db)
		found, err := repository.DeliverOne(t.Context(), cfg.SinkID, p)
		require.NoError(t, err)
		require.True(t, found)
		require.EqualValues(t, 1, pendingCount(t, repository, cfg.SinkID))
		found, err = repository.DeliverOne(t.Context(), cfg.SinkID, p)
		require.NoError(t, err)
		require.True(t, found)
		require.Zero(t, pendingCount(t, repository, cfg.SinkID))
		firstRecord := received.AssertEvent(t, large, key)
		nextRecord := received.AssertEvent(t, next, key)
		require.Equal(t, firstRecord.Partition, nextRecord.Partition)
		require.Greater(t, nextRecord.Offset, firstRecord.Offset)
	})
}
func kafkaAuthenticationConfig(mechanism string, tlsEnabled bool) kafka.Config {
	cfg := kafkaConfig()
	listener := "BASYX_IT_KAFKA_PORT"
	if mechanism != "" {
		listener = "BASYX_IT_KAFKA_AUTH_PORT"
		cfg.SASLMechanism = mechanism
		cfg.Username = "basyx"
		cfg.Password = "secret"
	}
	if tlsEnabled {
		listener = "BASYX_IT_KAFKA_TLS_PORT"
		if mechanism != "" {
			listener = "BASYX_IT_KAFKA_AUTH_TLS_PORT"
		}
		cfg.TLSEnabled = true
		cfg.CAFile = testenv.KafkaCertificate("ca.pem")
		cfg.CertificateFile = testenv.KafkaCertificate("client.pem")
		cfg.KeyFile = testenv.KafkaCertificate("client-key.pem")
	}
	cfg.Brokers = []string{testenv.KafkaAddress(listener)}
	return cfg
}

func publishKafkaFixture(t *testing.T, received *testenv.KafkaConsumer, cfg kafka.Config) *kafka.Publisher {
	t.Helper()
	p := kafkaPublisher(t, cfg)
	event, err := events.NewBuilder(events.DefaultConfig()).SubmodelCreated(cfg.SinkID, "", nil)
	require.NoError(t, err)
	record, err := events.Record(event, false)
	require.NoError(t, err)
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	routing, err := kafka.Routing(cfg.Topic, "auth:"+cfg.SinkID)
	require.NoError(t, err)
	require.NoError(t, p.Publish(t.Context(), routing, raw))
	received.AssertEvent(t, record, "auth:"+cfg.SinkID)
	return p
}

func TestKafkaOutboxRollbackRestartAndConcurrentOrdering(t *testing.T) {
	db := mqttTestDB(t)
	cfg := kafkaConfig()
	received := testenv.SubscribeKafka(t)
	repository := eventoutbox.NewRepository(db)
	rolledBack := kafkaEnqueue(t, db, cfg, "rollback", false)
	require.Zero(t, pendingCount(t, repository, cfg.SinkID))
	var records []events.FeedRecord
	for range 12 {
		records = append(records, kafkaEnqueue(t, db, cfg, "ordered", true))
	}
	unrelated := kafkaEnqueue(t, db, cfg, "unrelated", true)
	other := cfg
	other.SinkID += "-other"
	isolated := kafkaEnqueue(t, db, other, "ordered", true)
	unavailable := cfg
	unavailable.Brokers = []string{"127.0.0.1:1"}
	failed := kafkaPublisher(t, unavailable)
	_, err := repository.DeliverOne(t.Context(), cfg.SinkID, failed)
	require.Error(t, err)
	require.EqualValues(t, 13, pendingCount(t, repository, cfg.SinkID))
	require.NoError(t, failed.Stop(t.Context()))
	p := kafkaPublisher(t, cfg)
	worker, err := eventoutbox.Start(t.Context(), eventoutbox.NewRepository(db), cfg.SinkID, p)
	require.NoError(t, err)
	defer worker.Stop()
	require.Eventually(t, func() bool { return pendingCount(t, repository, cfg.SinkID) == 0 }, 30*time.Second, 100*time.Millisecond)
	var partition int32
	var offset int64 = -1
	for i, event := range records {
		record := received.AssertEvent(t, event, "ordered")
		if i == 0 {
			partition = record.Partition
		}
		require.Equal(t, partition, record.Partition)
		require.Greater(t, record.Offset, offset)
		offset = record.Offset
	}
	received.AssertEvent(t, unrelated, "unrelated")
	received.AssertNoEvent(t, rolledBack.Subject, rolledBack.Type)
	require.EqualValues(t, 1, pendingCount(t, repository, other.SinkID))
	_, err = repository.DeliverOne(t.Context(), other.SinkID, p)
	require.NoError(t, err)
	received.AssertEvent(t, isolated, "ordered")
}

func TestKafkaBrokerOutageKeepsMQTTAndWritesAvailable(t *testing.T) {
	received := testenv.SubscribeKafka(t)
	mqttEvents := mqttSubscribe(t, "basyx/#")
	publisher := publishKafkaFixture(t, received, kafkaConfig())
	port := os.Getenv("BASYX_IT_KAFKA_PORT")
	// #nosec G204 G702 -- arguments select a test broker using its allocated local port.
	result, err := exec.CommandContext(t.Context(), "docker", "ps", "--filter", "publish="+port, "--format", "{{.ID}}").Output()
	require.NoError(t, err)
	container := strings.TrimSpace(string(result))
	require.NotEmpty(t, container)
	require.NotContains(t, container, "\n")
	command := func(action string) {
		t.Helper()
		// #nosec G204 G702 -- action is pause/unpause and container is resolved from the test broker port.
		require.NoError(t, exec.CommandContext(t.Context(), "docker", action, container).Run())
	}
	command("pause")
	paused := true
	defer func() {
		if paused {
			command("unpause")
		}
	}()
	assertKafkaCancellation(t, publisher)
	id := fmt.Sprintf("urn:kafka:outage:%d", time.Now().UnixNano())
	base := submodelRepositoryEventFeedBaseURL
	mqttRequest(t, http.MethodPost, base+"/submodels", []byte(fmt.Sprintf(`{"id":%q,"modelType":"Submodel","submodelElements":[]}`, id)), http.StatusCreated)
	event := awaitMQTT(t, mqttEvents, id, events.TypeSubmodelCreated).Event
	assertMQTTFeedParity(t, base, event)
	command("unpause")
	paused = false
	received.AssertEvent(t, event, "submodel_history:"+id)
	mqttRequest(t, http.MethodDelete, base+"/submodels/"+base64.RawURLEncoding.EncodeToString([]byte(id)), nil, http.StatusNoContent)
}

func assertKafkaCancellation(t *testing.T, publisher *kafka.Publisher) {
	t.Helper()
	routing, err := kafka.Routing("basyx.events", "cancellation")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	require.Error(t, publisher.Publish(ctx, routing, []byte(`{"specversion":"1.0","id":"cancelled","source":"/test","type":"test"}`)))
	require.Less(t, time.Since(started), 2*time.Second)
	shutdown, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()
	require.NoError(t, publisher.Stop(shutdown))
}
