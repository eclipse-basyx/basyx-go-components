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

package eventfeed

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/jsonization"
)

func TestMutationSinkRequiresTransaction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	sink := NewMutationSink(NewService(nil, cfg))
	snap := map[string]any{"id": "aas-1"}
	if err := sink.HandleMutation(context.Background(), nil, Mutation{
		Table:            "aas_history",
		Identifier:       "aas-1",
		ChangeType:       "Updated",
		PreviousSnapshot: snap,
		Snapshot:         snap,
	}); err != nil {
		t.Fatalf("nil tx: %v", err)
	}
}

func TestMutationSinkDisabled(t *testing.T) {
	sink := NewMutationSink(NewService(nil, DefaultConfig()))
	if err := sink.HandleMutation(context.Background(), nil, Mutation{
		Table:      "aas_history",
		Identifier: "aas-1",
		ChangeType: "Created",
		Snapshot:   map[string]any{"id": "aas-1"},
	}); err != nil {
		t.Fatalf("disabled: %v", err)
	}
}

// A PUT that changed no persisted content is still recorded in history, but it
// is not a content change, so it must not reach the feed.
func TestMutationSinkSkipsAcknowledgedWrites(t *testing.T) {
	for _, table := range []string{"aas_history", "submodel_history"} {
		t.Run(table, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer func() { _ = db.Close() }()

			cfg := DefaultConfig()
			cfg.Enabled = true
			svc := NewService(NewRepository(db, cfg.MaxAge), cfg)

			mock.ExpectBegin()
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("begin: %v", err)
			}

			// No statement is expected: any INSERT INTO feed_events would be an
			// unexpected query and fail ExpectationsWereMet below.
			snapshot := map[string]any{
				"id":               "entity-1",
				"assetInformation": map[string]any{"globalAssetId": "asset-1"},
			}
			if err = NewMutationSink(svc).HandleMutation(context.Background(), tx, Mutation{
				Table:            table,
				Identifier:       "entity-1",
				ChangeType:       "Updated",
				PreviousSnapshot: snapshot,
				Snapshot:         snapshot,
				Acknowledged:     true,
			}); err != nil {
				t.Fatalf("handle: %v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("acknowledged write reached the feed: %v", err)
			}
		})
	}
}

func TestMutationSinkWritesAASAndAssetInTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	repo := NewRepository(db, cfg.MaxAge)
	svc := NewService(repo, cfg)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(int64(1), fixedNow))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(int64(2), fixedNow))

	sink := NewMutationSink(svc)
	if err = sink.HandleMutation(context.Background(), tx, Mutation{
		Table:      "aas_history",
		Identifier: "aas-1",
		ChangeType: "Created",
		Snapshot: map[string]any{
			"id": "aas-1",
			"assetInformation": map[string]any{
				"globalAssetId": "asset-1",
			},
		},
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}
}

// An AAS may reference a submodel that does not exist as a stored submodel:
// it can be created moments later (the usual AASX upload order) or live in a
// different repository. Such a reference must still appear in the AAS and
// asset events, with the referredSemanticId the AAS write recorded.
func TestMutationSinkAASEventKeepsReferenceToUnknownSubmodel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	svc := NewService(NewRepository(db, cfg.MaxAge), cfg)
	fixedNow := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	// Exactly two statements may reach the database: the AAS event insert and
	// the asset event insert. Any submodel lookup would be an unexpected query
	// and fail ExpectationsWereMet below.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(int64(1), fixedNow))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(int64(2), fixedNow))

	snapshot := map[string]any{
		"id":               "aas-1",
		"assetInformation": map[string]any{"globalAssetId": "asset-1"},
		"submodels": []any{
			map[string]any{
				"type": "ModelReference",
				"keys": []any{map[string]any{"type": "Submodel", "value": "sm-not-stored"}},
				"referredSemanticId": map[string]any{
					"type": "ExternalReference",
					"keys": []any{map[string]any{"type": "GlobalReference", "value": "0173-1#01-AHE582#003"}},
				},
			},
		},
	}

	sink := NewMutationSink(svc)
	if err = sink.HandleMutation(context.Background(), tx, Mutation{
		Table:      "aas_history",
		Identifier: "aas-1",
		ChangeType: "Updated",
		Snapshot:   snapshot,
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql: %v", err)
	}

	// The reference and its referredSemanticId survive into both payloads.
	aasID, globalAssetID, submodels := events.AASFieldsFromSnapshot(snapshot)
	want := []SubmodelRef{{SubmodelID: "sm-not-stored", SemanticID: "0173-1#01-AHE582#003"}}
	if !reflect.DeepEqual(submodels, want) {
		t.Fatalf("submodels = %#v, want %#v", submodels, want)
	}
	aasEvent, err := svc.build.AASUpdated(aasID, globalAssetID, submodels)
	if err != nil {
		t.Fatalf("build aas: %v", err)
	}
	assetEvent, err := svc.build.AssetUpdated(globalAssetID, aasID, submodels)
	if err != nil {
		t.Fatalf("build asset: %v", err)
	}
	for name, payload := range map[string]string{"aas": aasEvent.DataFull, "asset": assetEvent.DataFull} {
		if !strings.Contains(payload, "sm-not-stored") {
			t.Fatalf("%s event dropped the submodel reference: %s", name, payload)
		}
		if !strings.Contains(payload, `"referredSemanticId"`) || !strings.Contains(payload, "0173-1#01-AHE582#003") {
			t.Fatalf("%s event lost referredSemanticId: %s", name, payload)
		}
	}
}

func TestMutationSinkWriteFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := DefaultConfig()
	cfg.Enabled = true
	svc := NewService(NewRepository(db, cfg.MaxAge), cfg)
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO`)).
		WillReturnError(errors.New("tx cancelled"))

	sink := NewMutationSink(svc)
	err = sink.HandleMutation(context.Background(), tx, Mutation{
		Table:      "aas_history",
		Identifier: "aas-1",
		ChangeType: "Created",
		Snapshot:   map[string]any{"id": "aas-1"},
	})
	if err == nil {
		t.Fatal("expected write failure")
	}
}

func TestSubmodelFromSnapshotRoundTripsPCNRecordsToValueOnly(t *testing.T) {
	sm := pcnSubmodelWithCollectionRecords(t, pcnRecord("Record0", "CN123456"))
	snap, err := jsonization.ToJsonable(sm)
	if err != nil {
		t.Fatalf("ToJsonable: %v", err)
	}

	restored, err := jsonization.SubmodelFromJsonable(snap)
	if err != nil {
		t.Fatalf("submodelFromSnapshot: %v", err)
	}

	values := PCNNewRecordValuesFromSubmodel(nil, restored)
	if len(values) != 1 {
		t.Fatalf("expected 1 record, got %d", len(values))
	}

	raw, err := json.Marshal(values[0])
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	var record map[string]any
	if err = json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if len(record) != 1 || record["ManufacturerChangeID"] != "CN123456" {
		t.Fatalf("expected value-only record {ManufacturerChangeID: CN123456}, got %v", record)
	}
}

func TestFeedSaveDefaultsMissingTimestamp(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Truncate(time.Microsecond)
	repo := NewRepository(db, time.Hour)
	repo.now = func() time.Time { return now }
	mock.ExpectQuery("INSERT INTO.*time").WillReturnRows(sqlmock.NewRows([]string{"seq", "time"}).AddRow(1, now))
	if err = repo.Save(t.Context(), FeedEvent{ID: "test", DataFull: "{}", DataCompact: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
