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
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIntegrationActivationRejectsLiveDisagreementAndFencesOldInstances(t *testing.T) {
	db := testDatabase(t)
	scope := "activation-" + uuid.NewString()
	first := &rebac.StateStore{DB: db, Scope: scope}
	require.NoError(t, first.Initialize(t.Context(), "immutable"))
	enabled := rebac.IntegrationState{AASRegistry: true, SubmodelRegistry: true, Discovery: true}
	active, err := first.ActivateIntegrations(t.Context(), enabled)
	require.NoError(t, err)
	defer active.Close()
	second := &rebac.StateStore{DB: db, Scope: scope}
	companion, err := second.ActivateIntegrations(t.Context(), enabled)
	require.NoError(t, err)
	defer companion.Close()
	replacement := &rebac.StateStore{DB: db, Scope: scope}
	_, err = replacement.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.ErrorIs(t, err, rebac.ErrScopeConflict)
	active.Close()
	_, err = replacement.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.ErrorIs(t, err, rebac.ErrScopeConflict)
	companion.Close()
	changed, err := replacement.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.NoError(t, err)
	defer changed.Close()
	require.ErrorIs(t, first.WithApplied(t.Context(), func(*sql.Tx) error { return nil }), rebac.ErrScopeConflict)
	require.NoError(t, replacement.WithApplied(t.Context(), func(*sql.Tx) error { return nil }))
	require.ErrorIs(t, replacement.Initialize(t.Context(), "changed-model"), rebac.ErrScopeConflict)
}

func seedGeneratedVisibility(t *testing.T, state *rebac.StateStore) rebac.Tuple {
	t.Helper()
	tx, err := state.DB.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	source, err := state.CreateResource(t.Context(), tx, "aas", "urn:source", nil)
	require.NoError(t, err)
	target, err := state.CreateResource(t.Context(), tx, "aas_descriptor", "urn:generated", nil)
	require.NoError(t, err)
	statement, args, err := goqu.Dialect("postgres").Insert("rebac_generated").Rows(goqu.Record{"scope": state.Scope, "source_uuid": source.UUID, "target_uuid": target.UUID, "integration": "aas_registry"}).Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), statement, args...)
	require.NoError(t, err)
	sourceObject, err := rebac.ResourceObject(state.Scope, source.Kind, source.UUID)
	require.NoError(t, err)
	targetObject, err := rebac.ResourceObject(state.Scope, target.Kind, target.UUID)
	require.NoError(t, err)
	tuple := rebac.Tuple{User: sourceObject, Relation: "source", Object: targetObject}
	_, err = state.QueueChanged(t.Context(), tx, []rebac.Tuple{tuple, {User: "user:independent", Relation: "viewer", Object: targetObject}}, nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	_, err = state.ProjectNext(t.Context(), &lifecycleWriter{})
	require.NoError(t, err)
	return tuple
}

func TestIntegrationReconfigurationReconcilesOnlyRecordedProvenance(t *testing.T) {
	db := testDatabase(t)
	scope := "integration-" + uuid.NewString()
	first := &rebac.StateStore{DB: db, Scope: scope}
	require.NoError(t, first.Initialize(t.Context(), "immutable"))
	enabled := rebac.IntegrationState{AASRegistry: true}
	active, err := first.ActivateIntegrations(t.Context(), enabled)
	require.NoError(t, err)
	tuple := seedGeneratedVisibility(t, first)
	active.Close()
	disabled := &rebac.StateStore{DB: db, Scope: scope}
	inactive, err := disabled.ActivateIntegrations(t.Context(), rebac.IntegrationState{})
	require.NoError(t, err)
	require.ErrorIs(t, disabled.WithApplied(t.Context(), func(*sql.Tx) error { return nil }), rebac.ErrPending)
	writer := &lifecycleWriter{}
	changed, err := disabled.ProjectNext(t.Context(), writer)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []rebac.Tuple{tuple}, writer.deletes)
	require.Empty(t, writer.writes)
	require.NoError(t, disabled.WithApplied(t.Context(), func(tx *sql.Tx) error {
		tuples, readErr := disabled.Relationships(t.Context(), tx, tuple.Object, false)
		require.Len(t, tuples, 1)
		require.Equal(t, "viewer", tuples[0].Relation)
		return readErr
	}))
	inactive.Close()
	restored := &rebac.StateStore{DB: db, Scope: scope}
	restoredActive, err := restored.ActivateIntegrations(t.Context(), enabled)
	require.NoError(t, err)
	defer restoredActive.Close()
	writer = &lifecycleWriter{}
	changed, err = restored.ProjectNext(t.Context(), writer)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []rebac.Tuple{tuple}, writer.writes)
	require.Empty(t, writer.deletes)
}

func TestEveryIntegrationCombinationProjectsOnlyEnabledVisibility(t *testing.T) {
	combinations := []rebac.IntegrationState{
		{},
		{AASRegistry: true},
		{SubmodelRegistry: true},
		{Discovery: true},
		{AASRegistry: true, SubmodelRegistry: true},
		{AASRegistry: true, Discovery: true},
		{SubmodelRegistry: true, Discovery: true},
		{AASRegistry: true, SubmodelRegistry: true, Discovery: true},
	}
	for index, desired := range combinations {
		t.Run(fmt.Sprintf("combination-%d", index), func(t *testing.T) {
			db := testDatabase(t)
			state := &rebac.StateStore{DB: db, Scope: "integration-combination-" + uuid.NewString()}
			require.NoError(t, state.Initialize(t.Context(), "immutable"))
			active, err := state.ActivateIntegrations(t.Context(), desired)
			require.NoError(t, err)
			defer active.Close()

			require.Equal(t, desired.AASRegistry, integrationEnabled(t, state, "aas_registry"))
			require.Equal(t, desired.SubmodelRegistry, integrationEnabled(t, state, "submodel_registry"))
			require.Equal(t, desired.Discovery, integrationEnabled(t, state, "discovery"))
		})
	}
}

func integrationEnabled(t *testing.T, state *rebac.StateStore, integration string) bool {
	t.Helper()
	var configured map[string]bool
	require.NoError(t, state.WithApplied(t.Context(), func(tx *sql.Tx) error {
		statement, args, err := goqu.Dialect("postgres").From("rebac_scope").Select("integration_state").Where(goqu.C("scope").Eq(state.Scope)).Prepared(true).ToSQL()
		if err != nil {
			return err
		}
		var encoded string
		if err = tx.QueryRowContext(t.Context(), statement, args...).Scan(&encoded); err != nil {
			return err
		}
		return json.Unmarshal([]byte(encoded), &configured)
	}))
	return configured[integration]
}
