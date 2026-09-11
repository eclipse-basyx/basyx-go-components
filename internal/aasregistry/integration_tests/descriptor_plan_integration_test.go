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
	"net/http"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestDescriptorReadGenericPlanIsScopedAndReused(t *testing.T) {
	const aasID = "urn:basyx:integration:descriptor-plan"
	_, status, _, err := postJSONResponse(aasRegistryBaseURL+"/shell-descriptors", `{"id":"urn:basyx:integration:descriptor-plan","assetKind":"Instance","idShort":"PlanReuse"}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	t.Cleanup(func() { cleanupAASDescriptor(t, aasRegistryBaseURL, aasID) })
	db, err := sql.Open("pgx", aasRegistryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	initialMode := readIntegrationPlanMode(ctx, t, db)
	for _, mode := range []string{"auto", "force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			verifyDescriptorGenericPlan(ctx, t, db, aasID, mode)
			require.Equal(t, initialMode, readIntegrationPlanMode(ctx, t, db))
		})
	}
}

func verifyDescriptorGenericPlan(ctx context.Context, t *testing.T, db *sql.DB, aasID, mode string) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	query, args, err := goqu.Dialect("postgres").Select(goqu.Func("set_config", "plan_cache_mode", mode, true)).Prepared(true).ToSQL()
	require.NoError(t, err)
	var configured string
	require.NoError(t, tx.QueryRowContext(ctx, query, args...).Scan(&configured))
	for range 2 {
		descriptor, err := common.WithPostgreSQLGenericPlanTx(ctx, tx, func() (model.AssetAdministrationShellDescriptor, error) {
			require.Equal(t, "force_generic_plan", readIntegrationPlanMode(ctx, t, tx))
			return descriptors.GetAssetAdministrationShellDescriptorByIDTx(ctx, tx, aasID)
		})
		require.NoError(t, err)
		require.Equal(t, aasID, descriptor.Id)
		require.Equal(t, "PlanReuse", descriptor.IdShort)
		require.Equal(t, mode, readIntegrationPlanMode(ctx, t, tx))
	}
	query, args, err = goqu.Dialect("postgres").From("pg_prepared_statements").
		Select(goqu.SUM("generic_plans"), goqu.SUM("custom_plans")).
		Where(goqu.C("statement").Like("SELECT jsonb_strip_nulls%")).Prepared(true).ToSQL()
	require.NoError(t, err)
	var genericPlans, customPlans int64
	require.NoError(t, tx.QueryRowContext(ctx, query, args...).Scan(&genericPlans, &customPlans))
	require.GreaterOrEqual(t, genericPlans, int64(2))
	require.Zero(t, customPlans)
	require.NoError(t, tx.Commit())
}

func readIntegrationPlanMode(ctx context.Context, t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) string {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Select(goqu.Func("current_setting", "plan_cache_mode")).Prepared(true).ToSQL()
	require.NoError(t, err)
	var mode string
	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&mode))
	return mode
}
