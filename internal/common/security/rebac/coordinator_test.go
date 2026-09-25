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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

const (
	testSubmodelUUID = "1c7d6b4f-7b90-4c75-8b1d-1e2e503b2f02"
	testIssuer       = "https://idp.example"
)

func testCoordinator(t *testing.T) (*Coordinator, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	coordinator := NewCoordinator(db, common.ReBACConfig{GroupClaim: "groups"}, nil)
	coordinator.MarkReady()
	return coordinator, mock
}

func request(method string, pattern string, params map[string]string, subject string, rights ...grammar.RightsEnum) auth.ReBACRequest {
	claims := auth.Claims{}
	if subject != "" {
		claims = auth.Claims{"iss": testIssuer, "sub": subject, "groups": []any{"ops"}}
	}
	return auth.ReBACRequest{
		Route:         auth.ReBACRoute{Method: method, Pattern: pattern, Params: params, Rights: rights},
		Claims:        claims,
		PendingRights: rights,
	}
}

func expectLookup(mock sqlmock.Sqlmock, table string, authUUID string) {
	rows := sqlmock.NewRows([]string{"auth_uuid"})
	if authUUID != "" {
		rows.AddRow(authUUID)
	}
	mock.ExpectQuery(`SELECT resource\.object_uuid::text FROM \(SELECT .* FROM "` + table + `" AS "object_row"`).WillReturnRows(rows)
}

func expectDecision(mock sqlmock.Sqlmock, allowed bool) {
	mock.ExpectQuery(`^SELECT (\(|EXISTS)`).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(allowed))
}

func submodelRequest(subject string) auth.ReBACRequest {
	return request(http.MethodGet, "/submodels/{submodelIdentifier}",
		map[string]string{paramSubmodel: common.EncodeString("urn:sm")}, subject, grammar.RightsEnumREAD)
}

func TestResolveGrantsOnlyThePermittedResource(t *testing.T) {
	t.Parallel()

	coordinator, mock := testCoordinator(t)
	expectLookup(mock, "submodel", testSubmodelUUID)
	expectDecision(mock, true)
	grants, err := coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveKeepsABACForUnknownUngrantedOrAnonymousCallers(t *testing.T) {
	t.Parallel()

	coordinator, mock := testCoordinator(t)
	expectLookup(mock, "submodel", "")
	grants, err := coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "unknown identifiers behave like today")

	expectLookup(mock, "submodel", testSubmodelUUID)
	expectDecision(mock, false)
	grants, err = coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "denied ReBAC keeps the ABAC result")
	require.NoError(t, mock.ExpectationsWereMet())

	grants, err = coordinator.Resolve(t.Context(), submodelRequest(""))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "anonymous callers are ABAC-only")

	starting, _ := testCoordinator(t)
	starting.ready.Store(false)
	_, err = starting.Resolve(t.Context(), submodelRequest("alice"))
	require.ErrorIs(t, err, ErrNotReady)
}

func TestListsAreGrantedAsQueriesOnlyWhenSomethingIsReadable(t *testing.T) {
	t.Parallel()

	listRequest := request(http.MethodGet, "/concept-descriptions", nil, "alice", grammar.RightsEnumREAD)
	coordinator, mock := testCoordinator(t)
	expectDecision(mock, false)
	grants, err := coordinator.Resolve(t.Context(), listRequest)
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "callers without readable objects keep the ABAC denial")

	expectDecision(mock, true)
	grants, err = coordinator.Resolve(t.Context(), listRequest)
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())
	require.NoError(t, mock.ExpectationsWereMet())

	sql, _, err := liveObjects(KindConceptDescription, []string{UserKey(testIssuer, "alice")}, PermissionRead).ToSQL()
	require.NoError(t, err)
	require.Contains(t, sql, "'admin'", "repository admins see every object of the kind")
	require.Contains(t, sql, "UNION")
}

func TestPermissionQueriesFollowTheRelationModel(t *testing.T) {
	t.Parallel()

	keys := []string{UserKey(testIssuer, "alice"), GroupKey(testIssuer, "ops")}
	render := func(t *testing.T, kind ResourceKind, permission string) string {
		t.Helper()
		sql, _, err := permittedObjects(kind, keys, permission).ToSQL()
		require.NoError(t, err)
		return sql
	}
	read := render(t, KindSubmodel, PermissionRead)
	require.Contains(t, read, "UNION", "Submodels inherit read access through approved links")
	require.Contains(t, read, `"ref_key"."value"`, "links count only while the shell references the Submodel")
	require.Contains(t, read, "'viewer', 'editor', 'owner'")
	for _, permission := range []string{PermissionManage, PermissionDelete} {
		require.NotContains(t, render(t, KindSubmodel, permission), "UNION", "links never carry %s", permission)
	}
	require.NotContains(t, render(t, KindAAS, PermissionRead), "UNION", "only Submodels inherit from links")
	require.Contains(t, render(t, KindSubmodel, PermissionExecute), "'executor', 'owner'")
	for _, key := range keys {
		require.Contains(t, read, key, "user and token groups are subjects")
	}
}

func TestElementDecisionsCoverAncestorsButNotSiblings(t *testing.T) {
	t.Parallel()

	sql, _, err := grantedElements([]string{UserKey(testIssuer, "alice")}, PermissionRead, testSubmodelUUID).
		Where(goqu.I("g.element_path").In(ElementPathChain("a.list[1].c"))).ToSQL()
	require.NoError(t, err)
	for _, ancestor := range []string{"'a'", "'a.list'", "'a.list[1]'", "'a.list[1].c'"} {
		require.Contains(t, sql, ancestor)
	}
	require.NotContains(t, sql, "'a.list[10]'")
	require.Contains(t, sql, "'element'")
}

func TestDerivedObjectsInheritEveryPermissionOfTheirSource(t *testing.T) {
	t.Parallel()

	keys := []string{UserKey(testIssuer, "alice")}
	render := func(t *testing.T, expression any) string {
		t.Helper()
		sql, _, err := dialect.Select(expression).ToSQL()
		require.NoError(t, err)
		return sql
	}
	entry := render(t, permissionCondition(KindAssetLinks, keys, PermissionUpdate, goqu.L("?::uuid", testSubmodelUUID)))
	for _, fragment := range []string{`"derivation_asset_links"`, `"derivation_aas_descriptor"`, `'asset_links'`, `'aas_descriptor'`, `'aas'`, `'editor', 'owner'`} {
		require.Contains(t, entry, fragment, "discovery entries inherit through their descriptor from the shell")
	}
	require.NotContains(t, render(t, permissionCondition(KindAAS, keys, PermissionRead, goqu.L("?::uuid", testSubmodelUUID))), derivationTable,
		"repository resources are never derived")

	list, _, err := liveObjects(KindSubmodelDescriptor, keys, PermissionRead).ToSQL()
	require.NoError(t, err)
	require.Contains(t, list, `"derived_submodel_descriptor"`)
	require.Contains(t, list, `"rebac_submodel_link"`, "descriptors follow Submodel access including approved links")
	require.Contains(t, list, `"aas_descriptor_id" IS NULL`, "embedded descriptors are no standalone objects")
}

func TestSubmodelWritesFollowReferencingShellsByIdentifier(t *testing.T) {
	t.Parallel()

	sql, _, err := shellsReferencing("urn:sm").ToSQL()
	require.NoError(t, err)
	require.Contains(t, sql, `"ref_key"."value" IN ('urn:sm')`, "the reference survives the deletion of the Submodel row")
	require.NotContains(t, sql, `"submodel"`)
	require.True(t, writes([]grammar.RightsEnum{grammar.RightsEnumDELETE}))
	require.False(t, writes([]grammar.RightsEnum{grammar.RightsEnumREAD, grammar.RightsEnumEXECUTE}))
}
