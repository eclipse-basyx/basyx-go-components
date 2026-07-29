/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package main

import (
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/stretchr/testify/require"
)

func TestPersistentAsyncHandleLifecycleAcrossManagers(t *testing.T) {
	db, err := common.NewDatabaseConnection(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	managerA, err := asyncjob.NewPostgresManager(t.Context(), db, "ASYNC-IT", time.Minute)
	require.NoError(t, err)
	managerB, err := asyncjob.NewPostgresManager(t.Context(), db, "ASYNC-IT", time.Minute)
	require.NoError(t, err)

	handleID, err := managerA.Start(t.Context(), "owner-a", asyncjob.StartOptions{
		JobKind:  "integration.completed",
		Metadata: map[string]string{"source": "replica-a"},
	})
	require.NoError(t, err)
	require.NoError(t, managerA.CompletePayload(t.Context(), handleID, map[string]any{"value": "persisted"}))

	_, found, err := managerB.GetForOwner(t.Context(), handleID, "owner-b")
	require.NoError(t, err)
	require.False(t, found)

	record, found, err := managerB.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Completed", record.ExecutionState)
	require.Equal(t, "replica-a", record.Metadata["source"])
	require.Equal(t, "persisted", record.Payload.(map[string]any)["value"])
}

func TestPersistentAsyncHandleRecoversAbandonedWorkerAndCleansUp(t *testing.T) {
	db, err := common.NewDatabaseConnection(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	managerA, err := asyncjob.NewPostgresManager(t.Context(), db, "ASYNC-RECOVERY-IT", 50*time.Millisecond)
	require.NoError(t, err)
	handleID, err := managerA.Start(t.Context(), "owner-a", asyncjob.StartOptions{
		JobKind:           "integration.abandoned",
		ExecutionDeadline: time.Now().UTC().Add(-time.Second),
	})
	require.NoError(t, err)

	managerB, err := asyncjob.NewPostgresManager(t.Context(), db, "ASYNC-RECOVERY-IT", 50*time.Millisecond)
	require.NoError(t, err)
	record, found, err := managerB.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Running", record.ExecutionState)

	expireLeaseQuery, expireLeaseArgs, err := goqu.Dialect("postgres").
		Update("async_job").
		Set(goqu.Record{"lease_expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handleID)).
		ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), expireLeaseQuery, expireLeaseArgs...)
	require.NoError(t, err)

	record, found, err = managerB.GetForOwner(t.Context(), handleID, "owner-a")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "Failed", record.ExecutionState)
	require.Equal(t, 500, record.ErrorStatus)
	require.NotNil(t, record.ErrorBody)

	expireResultQuery, expireResultArgs, err := goqu.Dialect("postgres").
		Update("async_job").
		Set(goqu.Record{"expires_at": time.Now().UTC().Add(-time.Second)}).
		Where(goqu.C("handle_id").Eq(handleID)).
		ToSQL()
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), expireResultQuery, expireResultArgs...)
	require.NoError(t, err)

	_, found, err = managerB.Get(t.Context(), handleID)
	require.NoError(t, err)
	require.False(t, found)
}
