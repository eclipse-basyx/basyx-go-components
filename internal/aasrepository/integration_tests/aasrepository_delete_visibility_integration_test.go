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
	"github.com/FriedJannik/aas-go-sdk/types"
	aasapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasp "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	smp "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestNestedDeleteHidesInaccessibleReference(t *testing.T) {
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	aasdb, err := aasp.NewAssetAdministrationShellDatabaseFromDB(db, "off")
	require.NoError(t, err)
	smdb, err := smp.NewSubmodelDatabaseFromDB(db, nil, "off")
	require.NoError(t, err)
	old := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(old) })
	cfg := &common.Config{}
	ctx := common.ContextWithConfig(t.Context(), cfg)
	aas := types.NewAssetAdministrationShell(fmt.Sprintf("urn:example:delete-hidden:%d", time.Now().UnixNano()), types.NewAssetInformation(types.AssetKindInstance))
	idShort := "secret"
	aas.SetIDShort(&idShort)
	aas.SetSubmodels([]types.IReference{types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, "linked-sm")})})
	require.NoError(t, aasdb.CreateAssetAdministrationShell(ctx, aas))
	t.Cleanup(func() {
		_ = aasdb.DeleteAssetAdministrationShellByID(common.ContextWithConfig(context.WithoutCancel(t.Context()), &common.Config{}), aas.ID())
	})
	cfg.ABAC.Enabled = true
	var expression grammar.LogicalExpression
	require.NoError(t, json.Unmarshal([]byte(`{"$eq":[{"$field":"$aas#idShort"},{"$strVal":"visible"}]}`), &expression))
	ctx = auth.WithQueryFilter(ctx, &auth.QueryFilter{Formula: &expression, FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{grammar.RightsEnumDELETE: expression}})
	_, err = aasdb.GetAssetAdministrationShellByID(ctx, aas.ID())
	require.True(t, common.IsErrNotFound(err))
	svc := aasapi.NewAssetAdministrationShellRepositoryAPIAPIService(ctx, aasdb, smdb, false)
	linked, err := svc.DeleteSubmodelByIdAasRepository(ctx, common.EncodeString(aas.ID()), common.EncodeString("linked-sm"))
	require.NoError(t, err)
	absent, err := svc.DeleteSubmodelByIdAasRepository(ctx, common.EncodeString(aas.ID()), common.EncodeString("absent-sm"))
	require.NoError(t, err)
	require.Equal(t, 404, linked.Code, "hidden reference should remain indistinguishable")
	require.Equal(t, 404, absent.Code)
}

func TestNestedDeleteUsesDeleteVisibilityWithoutRequiringRead(t *testing.T) {
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	aasdb, err := aasp.NewAssetAdministrationShellDatabaseFromDB(db, "off")
	require.NoError(t, err)
	smdb, err := smp.NewSubmodelDatabaseFromDB(db, nil, "off")
	require.NoError(t, err)
	old := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(old) })
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	aasID := fmt.Sprintf("urn:example:delete-write-only:%d", time.Now().UnixNano())
	smID := aasID + ":sm"
	aas := types.NewAssetAdministrationShell(aasID, types.NewAssetInformation(types.AssetKindInstance))
	aas.SetSubmodels([]types.IReference{types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, smID)})})
	require.NoError(t, aasdb.CreateAssetAdministrationShell(ctx, aas))
	require.NoError(t, smdb.CreateSubmodel(ctx, types.NewSubmodel(smID)))
	t.Cleanup(func() {
		_ = aasdb.DeleteAssetAdministrationShellByID(context.WithoutCancel(ctx), aasID)
		_ = smdb.DeleteSubmodel(context.WithoutCancel(ctx), smID)
	})
	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	allowed, denied := true, false
	expression := grammar.LogicalExpression{Boolean: &allowed}
	deleteCtx := auth.WithQueryFilter(common.ContextWithConfig(ctx, cfg), &auth.QueryFilter{
		Formula: &expression,
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumDELETE: expression,
			grammar.RightsEnumREAD:   {Boolean: &denied},
		},
	})
	svc := aasapi.NewAssetAdministrationShellRepositoryAPIAPIService(deleteCtx, aasdb, smdb, false)
	response, err := svc.DeleteSubmodelByIdAasRepository(deleteCtx, common.EncodeString(aasID), common.EncodeString(smID))
	require.NoError(t, err)
	require.Equal(t, 204, response.Code)
}
