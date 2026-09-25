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

package integration_tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BASYX_REBAC_TEST_DSN")
	if dsn == "" {
		t.Skip("BASYX_REBAC_TEST_DSN is required for live PostgreSQL tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestProjectionBarrierAndRollback(t *testing.T) {
	db := testDatabase(t)
	state := &rebac.StateStore{DB: db, Scope: "test-" + uuid.NewString()}
	if err := state.Initialize(t.Context(), "config-a"); err != nil {
		t.Fatal(err)
	}
	if err := state.Initialize(t.Context(), "config-b"); !errors.Is(err, rebac.ErrScopeConflict) {
		t.Fatalf("configuration mismatch accepted: %v", err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tuple := rebac.Tuple{User: "user:alice", Relation: "viewer", Object: "aas:test"}
	if _, err = state.Enqueue(t.Context(), tx, []rebac.Tuple{tuple}, nil); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }); err != nil {
		t.Fatal("rolled back enqueue left barrier", err)
	}
	tx, err = db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.Enqueue(t.Context(), tx, []rebac.Tuple{tuple}, nil); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = state.WithApplied(t.Context(), func(*sql.Tx) error { t.Fatal("pending projection became visible"); return nil }); !errors.Is(err, rebac.ErrPending) {
		t.Fatal(err)
	}
	assertProjectionRetry(t, state, tuple)
}

func assertProjectionRetry(t *testing.T, state *rebac.StateStore, tuple rebac.Tuple) {
	t.Helper()
	failed := &testWriter{failure: errors.New("offline")}
	if _, err := state.ProjectNext(t.Context(), failed); err == nil {
		t.Fatal("write failure swallowed")
	}
	success := &testWriter{}
	if done, err := state.ProjectNext(t.Context(), success); err != nil || !done {
		t.Fatalf("retry: %v %v", done, err)
	}
	if len(success.writes) != 1 || success.writes[0] != tuple {
		t.Fatal("projection mismatch")
	}
	if err := state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if done, err := state.ProjectNext(t.Context(), success); err != nil || done {
		t.Fatalf("applied revision replayed: %v %v", done, err)
	}
}

type testWriter struct {
	failure error
	writes  []rebac.Tuple
}

func (w *testWriter) Write(_ context.Context, writes, _ []rebac.Tuple) error {
	w.writes = writes
	return w.failure
}

func TestOpenFGAModelAndHierarchy(t *testing.T) {
	endpoint := os.Getenv("BASYX_REBAC_TEST_OPENFGA_URL")
	if endpoint == "" {
		t.Skip("BASYX_REBAC_TEST_OPENFGA_URL is required for live OpenFGA tests")
	}
	store := postJSON(t, endpoint+"/stores", map[string]string{"name": "basyx-test-" + uuid.NewString()})["id"].(string)
	model, err := rebac.AuthorizationModelJSON()
	if err != nil {
		t.Fatal(err)
	}
	id := postJSON(t, endpoint+"/stores/"+store+"/authorization-models", json.RawMessage(model))["authorization_model_id"].(string)
	client, err := rebac.NewClient(rebac.Config{URL: endpoint, Token: os.Getenv("BASYX_REBAC_TEST_OPENFGA_TOKEN"), StoreID: store, ModelID: id, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.ReadModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Uses only direct relations here; hierarchy cases are added alongside the fixed model.
	writes := []rebac.Tuple{{User: "user:alice", Relation: "viewer", Object: "aas:machine"}}
	if err = client.Write(t.Context(), writes, nil); err != nil {
		t.Fatal(err)
	}
	if err = client.Write(t.Context(), writes, nil); err != nil {
		t.Fatal("idempotent write", err)
	}
	allowed, err := client.Check(t.Context(), "user:alice", "viewer", "aas:machine", nil)
	if err != nil || !allowed {
		t.Fatalf("grant: %v %v", allowed, err)
	}
	if err = client.Write(t.Context(), nil, writes); err != nil {
		t.Fatal(err)
	}
	if err = client.Write(t.Context(), nil, writes); err != nil {
		t.Fatal("idempotent delete", err)
	}
	allowed, err = client.Check(t.Context(), "user:alice", "viewer", "aas:machine", nil)
	if err != nil || allowed {
		t.Fatalf("revoke: %v %v", allowed, err)
	}
}

func postJSON(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G704 -- target is explicitly supplied by the integration-test operator.
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("BASYX_REBAC_TEST_OPENFGA_TOKEN"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	// #nosec G704 -- test-only call to the operator-configured OpenFGA instance.
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("OpenFGA status %d: %s", response.StatusCode, raw)
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
