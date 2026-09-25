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
	"github.com/stretchr/testify/require"
)

type recoveryProjection struct {
	tuples    map[rebac.Tuple]bool
	failAfter int
	writes    int
}

func (p *recoveryProjection) ReadTuples(context.Context, string) ([]rebac.Tuple, string, error) {
	var result []rebac.Tuple
	for tuple := range p.tuples {
		result = append(result, tuple)
	}
	return result, "", nil
}
func (*recoveryProjection) ReadChanges(context.Context, string, int) (rebac.ChangesPage, error) {
	return rebac.ChangesPage{}, nil
}
func (p *recoveryProjection) Write(_ context.Context, writes, deletes []rebac.Tuple) error {
	if p.failAfter > 0 && p.writes >= p.failAfter {
		return errors.New("test projection unavailable")
	}
	for _, tuple := range deletes {
		delete(p.tuples, tuple)
	}
	for _, tuple := range writes {
		p.tuples[tuple] = true
	}
	p.writes++
	return nil
}

func TestReconciliationRepairsDesiredGraphAndRetainsBarrierAfterFailure(t *testing.T) {
	db := testDatabase(t)
	state := &rebac.StateStore{DB: db, Scope: "recover-" + uuid.NewString()}
	require.NoError(t, state.Initialize(t.Context(), "immutable"))
	active, err := state.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.NoError(t, err)
	defer active.Close()
	expected := seedGeneratedVisibility(t, state)
	retired := seedRetiredGeneration(t, state)
	rogue := rebac.Tuple{User: "user:rogue", Relation: "owner", Object: expected.Object}
	unrelated := rebac.Tuple{User: "user:other", Relation: "viewer", Object: "aas:another-scope"}
	projection := &recoveryProjection{tuples: map[rebac.Tuple]bool{rogue: true, unrelated: true, retired: true}, failAfter: 1}
	var phases []string
	state.RecoveryAudit = func(_ context.Context, _ *sql.Tx, phase string, _ map[string]string) error {
		phases = append(phases, phase)
		return nil
	}
	require.Error(t, state.Reconcile(t.Context(), projection))
	require.ErrorIs(t, state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }), rebac.ErrPending)
	projection.failAfter = 0
	require.NoError(t, state.Reconcile(t.Context(), projection))
	require.NoError(t, state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }))
	require.True(t, projection.tuples[expected])
	require.False(t, projection.tuples[rogue])
	require.False(t, projection.tuples[retired])
	require.True(t, projection.tuples[unrelated])
	require.Contains(t, phases, "failed")
	require.Contains(t, phases, "applied")
}

func TestReconciliationAuditFailureKeepsProtectedRequestsUnavailable(t *testing.T) {
	db := testDatabase(t)
	state := &rebac.StateStore{DB: db, Scope: "recover-audit-" + uuid.NewString()}
	require.NoError(t, state.Initialize(t.Context(), "immutable"))
	active, err := state.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.NoError(t, err)
	defer active.Close()
	state.RecoveryAudit = func(_ context.Context, _ *sql.Tx, phase string, _ map[string]string) error {
		if phase == "applied" {
			return errors.New("test durable audit unavailable")
		}
		return nil
	}
	projection := &recoveryProjection{tuples: map[rebac.Tuple]bool{}}
	require.Error(t, state.Reconcile(t.Context(), projection))
	require.ErrorIs(t, state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }), rebac.ErrPending)
	state.RecoveryAudit = nil
	require.NoError(t, state.Reconcile(t.Context(), projection))
	require.NoError(t, state.WithApplied(t.Context(), func(*sql.Tx) error { return nil }))
}

func seedRetiredGeneration(t *testing.T, state *rebac.StateStore) rebac.Tuple {
	t.Helper()
	tx, err := state.DB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	resource, err := state.CreateResource(t.Context(), tx, "aas", "urn:retired", nil)
	require.NoError(t, err)
	_, err = state.RetireResource(t.Context(), tx, resource)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	object, err := rebac.ResourceObject(state.Scope, resource.Kind, resource.UUID)
	require.NoError(t, err)
	return rebac.Tuple{User: "user:retired-owner", Relation: "owner", Object: object}
}
