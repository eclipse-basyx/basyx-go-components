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
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
)

func TestQueueChangedDoesNotCreateBarrierForIdenticalTuple(t *testing.T) {
	db := testDatabase(t)
	state := initializedLifecycleState(t, db, "unchanged")
	tuple := rebac.Tuple{User: "user:alice", Relation: "viewer", Object: "aas:machine"}

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if revision, err := state.QueueChanged(t.Context(), tx, []rebac.Tuple{tuple}, nil); err != nil || revision != 1 {
		t.Fatalf("initial change: revision=%d error=%v", revision, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if done, err := state.ProjectNext(t.Context(), &lifecycleWriter{}); err != nil || !done {
		t.Fatalf("initial projection: done=%v error=%v", done, err)
	}

	tx, err = db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if revision, err := state.QueueChanged(t.Context(), tx, []rebac.Tuple{tuple}, nil); err != nil || revision != 1 {
		t.Fatalf("unchanged tuple: revision=%d error=%v", revision, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }); err != nil {
		t.Fatalf("unchanged tuple left projection barrier: %v", err)
	}
}

func TestEnqueueCoalescesCurrentTransactionAndRejectsIndependentPending(t *testing.T) {
	db := testDatabase(t)
	state := initializedLifecycleState(t, db, "coalesce")
	tuple := rebac.Tuple{User: "user:alice", Relation: "viewer", Object: "aas:machine"}

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if revision, err := state.Enqueue(t.Context(), tx, []rebac.Tuple{tuple}, nil); err != nil || revision != 1 {
		t.Fatalf("write: revision=%d error=%v", revision, err)
	}
	if revision, err := state.Enqueue(t.Context(), tx, nil, []rebac.Tuple{tuple}); err != nil || revision != 1 {
		t.Fatalf("coalesced delete: revision=%d error=%v", revision, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	independent, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.Enqueue(t.Context(), independent, []rebac.Tuple{tuple}, nil); !errors.Is(err, rebac.ErrPending) {
		t.Fatalf("independent pending mutation accepted: %v", err)
	}
	if err = independent.Rollback(); err != nil {
		t.Fatal(err)
	}

	writer := &lifecycleWriter{}
	if done, err := state.ProjectNext(t.Context(), writer); err != nil || !done {
		t.Fatalf("coalesced projection: done=%v error=%v", done, err)
	}
	if len(writer.writes) != 0 || len(writer.deletes) != 1 || writer.deletes[0] != tuple {
		t.Fatalf("final operation was not the delete: writes=%v deletes=%v", writer.writes, writer.deletes)
	}
}

func TestRetireAndRecreateDoesNotTransferRelationships(t *testing.T) {
	db := testDatabase(t)
	state := initializedLifecycleState(t, db, "recreate")

	resource, object := createOwnedLifecycleResource(t, db, state)

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	deletes, err := state.RetireResource(t.Context(), tx, resource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.QueueChanged(t.Context(), tx, nil, deletes); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if done, err := state.ProjectNext(t.Context(), &lifecycleWriter{}); err != nil || !done {
		t.Fatalf("retirement projection: done=%v error=%v", done, err)
	}

	tx, err = db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	recreated, writes, err := state.EnsureResource(t.Context(), tx, "aas", "machine", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if recreated.UUID == resource.UUID {
		t.Fatal("recreated resource reused retired generation")
	}
	if len(writes) != 0 {
		t.Fatalf("unexpected inherited creation grants: %v", writes)
	}
	assertRecreatedRelationships(t, db, state, recreated, object)
}

func assertRecreatedRelationships(t *testing.T, db *sql.DB, state *rebac.StateStore, recreated rebac.StoredResource, object string) {
	t.Helper()
	recreatedObject, err := rebac.ResourceObject(state.Scope, recreated.Kind, recreated.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if recreatedObject == object {
		t.Fatal("recreated resource reused retired object")
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	relationships, err := state.Relationships(t.Context(), tx, recreatedObject, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if len(relationships) != 0 {
		t.Fatalf("retired resource grant transferred to recreation: %v", relationships)
	}
}

func initializedLifecycleState(t *testing.T, db *sql.DB, name string) *rebac.StateStore {
	t.Helper()
	state := &rebac.StateStore{DB: db, Scope: name + "-" + uuid.NewString()}
	if err := state.Initialize(t.Context(), "config"); err != nil {
		t.Fatal(err)
	}
	return state
}

type lifecycleWriter struct {
	writes  []rebac.Tuple
	deletes []rebac.Tuple
}

func (w *lifecycleWriter) Write(_ context.Context, writes, deletes []rebac.Tuple) error {
	w.writes = writes
	w.deletes = deletes
	return nil
}

func createOwnedLifecycleResource(t *testing.T, db *sql.DB, state *rebac.StateStore) (rebac.StoredResource, string) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := state.CreateResource(t.Context(), tx, "aas", "machine", nil)
	if err != nil {
		t.Fatal(err)
	}
	object, err := rebac.ResourceObject(state.Scope, resource.Kind, resource.UUID)
	if err != nil {
		t.Fatal(err)
	}
	grant := rebac.Tuple{User: "user:alice", Relation: "owner", Object: object}
	if _, err = state.QueueChanged(t.Context(), tx, []rebac.Tuple{grant}, nil); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if done, err := state.ProjectNext(t.Context(), &lifecycleWriter{}); err != nil || !done {
		t.Fatalf("grant projection: done=%v error=%v", done, err)
	}

	return resource, object
}
