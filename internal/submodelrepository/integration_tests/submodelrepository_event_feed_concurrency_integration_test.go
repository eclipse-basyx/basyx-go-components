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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeedsetup"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/mqtt"
	smpersistence "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/stretchr/testify/require"
)

func TestEventFeedConcurrentPCNAdditionsUseCommittedPredecessor(t *testing.T) {
	received := mqttSubscribe(t, "basyx/#")
	ctx, cancel := context.WithTimeout(eventFeedPersistenceContext(t), 15*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	name := fmt.Sprintf("pcn-concurrency-%d", time.Now().UnixNano())
	workerURL, err := url.Parse(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	query := workerURL.Query()
	query.Set("application_name", name)
	workerURL.RawQuery = query.Encode()
	workers, err := sql.Open("pgx", workerURL.String())
	require.NoError(t, err)
	defer func() { _ = workers.Close() }()
	backend, err := smpersistence.NewSubmodelDatabaseFromDB(workers, nil, "off")
	require.NoError(t, err)
	previous := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	defer history.Configure(previous)
	cfg := eventfeed.DefaultConfig()
	cfg.Enabled = true
	module, err := eventfeed.NewModule(workers, cfg)
	require.NoError(t, err)
	eventConfig := &common.Config{Eventing: common.EventingConfig{Enabled: true, OutboxEnabled: true, Sinks: []string{"mqtt"}, TopicPrefix: "basyx", Feed: common.EventFeedConfig{Enabled: true}, MQTT: mqtt.Config{Broker: submodelRepositoryMQTTURL, ClientID: name, SinkID: name, QoS: 1}}}
	require.NoError(t, eventfeedsetup.Start(ctx, workers, eventConfig, module))
	defer module.Stop()
	sm := types.NewSubmodel("urn:event-feed:" + name)
	sm.SetSemanticID(types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{types.NewKey(types.KeyTypesGlobalReference, eventfeed.SemanticIDPCN)}))
	records := types.NewSubmodelElementCollection()
	idShort := "Records"
	records.SetIDShort(&idShort)
	sm.SetSubmodelElements([]types.ISubmodelElement{records})
	require.NoError(t, backend.CreateSubmodel(ctx, sm))
	defer func() { _ = backend.DeleteSubmodel(ctx, sm.ID()) }()
	blocker, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	lockPCNRecordsParent(ctx, t, blocker, sm.ID())
	results := make(chan error, 2)
	for _, id := range []string{"CN1", "CN2"} {
		go func(id string) {
			results <- backend.AddSubmodelElementWithPath(ctx, sm.ID(), "Records", eventFeedPCNRecord(id))
		}(id)
	}
	waitForPCNMutationLocks(ctx, t, db, name)
	require.NoError(t, blocker.Commit())
	for range 2 {
		require.NoError(t, <-results)
	}
	require.ElementsMatch(t, []string{"CN1", "CN2"}, readPCNChanges(ctx, t, db, sm.ID()))
	notifications := []string{}
	for range 2 {
		event := awaitMQTT(t, received, sm.ID(), eventfeed.TypePCN)
		record := event.Event.Data["record"].(map[string]any)
		notifications = append(notifications, record["ManufacturerChangeID"].(string))
	}
	require.ElementsMatch(t, []string{"CN1", "CN2"}, notifications)
}

func eventFeedPersistenceContext(t *testing.T) context.Context {
	t.Helper()
	var ctx context.Context
	handler := common.ConfigMiddleware(&common.Config{})(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { ctx = request.Context() }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil).WithContext(t.Context()))
	return ctx
}

func eventFeedPCNRecord(id string) types.ISubmodelElement {
	record := types.NewSubmodelElementCollection()
	record.SetIDShort(&id)
	property := types.NewProperty(types.DataTypeDefXSDString)
	name := "ManufacturerChangeID"
	property.SetIDShort(&name)
	property.SetValue(&id)
	record.SetValue([]types.ISubmodelElement{property})
	return record
}

func lockPCNRecordsParent(ctx context.Context, t *testing.T, tx *sql.Tx, submodelID string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("sme.submodel_id")))).
		Select(goqu.I("sme.id")).
		Where(goqu.I("sm.submodel_identifier").Eq(submodelID), goqu.I("sme.idshort_path").Eq("Records")).
		ForUpdate(goqu.Wait, goqu.T("sme")).Prepared(true).ToSQL()
	require.NoError(t, err)
	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, query, args...).Scan(&id))
}

func waitForPCNMutationLocks(ctx context.Context, t *testing.T, db *sql.DB, applicationName string) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_stat_activity").Select(goqu.COUNT("*")).
		Where(goqu.C("application_name").Eq(applicationName), goqu.C("wait_event_type").Eq("Lock")).Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var count int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			t.Logf("read waiting mutations: %v", err)
			return false
		}
		return count == 2
	}, 5*time.Second, 20*time.Millisecond, "both writers must be blocked before their parent can change")
}

func readPCNChanges(ctx context.Context, t *testing.T, db *sql.DB, submodelID string) []string {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("feed_events").Select("data_full").
		Where(goqu.C("event_type").Eq(eventfeed.TypePCN), goqu.C("subject").Eq(submodelID)).Prepared(true).ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	changes := []string{}
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var event struct {
			Record map[string]string `json:"record"`
		}
		require.NoError(t, json.Unmarshal(payload, &event))
		changes = append(changes, event.Record["ManufacturerChangeID"])
	}
	require.NoError(t, rows.Err())
	return changes
}
