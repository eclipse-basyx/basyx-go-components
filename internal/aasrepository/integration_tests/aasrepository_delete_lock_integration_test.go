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
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestEvidenceDeleteLocksAASBeforeLoadingSnapshot(t *testing.T) {
	aasID := "https://example.com/ids/aas/evidence-delete-lock"
	status, err := createAASForThumbnailTest(aasRepositoryBaseURL, aasID)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	t.Cleanup(func() {
		request, requestErr := http.NewRequest(
			http.MethodDelete,
			aasRepositoryBaseURL+"/shells/"+base64.RawURLEncoding.EncodeToString([]byte(aasID)),
			nil,
		)
		if requestErr != nil {
			return
		}
		response, requestErr := (&http.Client{Timeout: 10 * time.Second}).Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
		}
	})

	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repository, err := persistence.NewAssetAdministrationShellDatabaseFromDB(db, "off")
	require.NoError(t, err)

	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff, EvidenceEnabled: true})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	blocker, err := db.Begin()
	require.NoError(t, err)
	blockerFinished := false
	t.Cleanup(func() {
		if !blockerFinished {
			_ = blocker.Rollback()
		}
	})

	lockQuery, lockArgs, err := goqu.Dialect("postgres").
		From("aas").
		Select("id").
		Where(goqu.C("aas_id").Eq(aasID)).
		ForUpdate(goqu.Wait).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	var aasDBID int64
	require.NoError(t, blocker.QueryRow(lockQuery, lockArgs...).Scan(&aasDBID))

	updateQuery, updateArgs, err := goqu.Dialect("postgres").
		Update("aas").
		Set(goqu.Record{"category": "committed-before-delete"}).
		Where(goqu.C("id").Eq(aasDBID)).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	_, err = blocker.Exec(updateQuery, updateArgs...)
	require.NoError(t, err)

	deleteResult := make(chan error, 1)
	deleteCtx := common.ContextWithConfig(context.Background(), &common.Config{})
	go func() {
		deleteResult <- repository.DeleteAssetAdministrationShellByID(deleteCtx, aasID)
	}()

	require.Eventually(t, func() bool {
		return waitingForAASDeleteRowLock(db)
	}, 5*time.Second, 20*time.Millisecond)

	require.NoError(t, blocker.Commit())
	blockerFinished = true

	select {
	case deleteErr := <-deleteResult:
		require.Error(t, deleteErr)
		require.Contains(t, deleteErr.Error(), "HISTORY-EVIDENCE-MUTATION-NILSTORE")
	case <-time.After(5 * time.Second):
		t.Fatal("evidence-enabled delete did not finish after the row lock was released")
	}
}

func waitingForAASDeleteRowLock(db *sql.DB) bool {
	query, args, err := goqu.Dialect("postgres").
		From("pg_stat_activity").
		Select(goqu.COUNT("*")).
		Where(
			goqu.C("wait_event_type").Eq("Lock"),
			goqu.C("query").ILike("SELECT \"id\" FROM \"aas\"%FOR UPDATE%"),
		).
		Prepared(true).
		ToSQL()
	if err != nil {
		return false
	}
	var count int
	if err = db.QueryRow(query, args...).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
