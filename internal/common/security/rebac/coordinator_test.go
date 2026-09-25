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
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

const (
	testSubmodelUUID = "1c7d6b4f-7b90-4c75-8b1d-1e2e503b2f02"
	testIssuer       = "https://idp.example"
)

// fakeClient answers checks from a set of allowed "user|relation|object" keys.
type fakeClient struct {
	mu        sync.Mutex
	allowed   map[string]bool
	listed    []string
	err       error
	checks    []CheckItem
	writes    []Tuple
	modelJSON []byte
}

func (f *fakeClient) decide(item CheckItem) bool {
	return f.allowed[item.User+"|"+item.Relation+"|"+item.Object]
}

func (f *fakeClient) Check(_ context.Context, item CheckItem) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, item)
	return f.decide(item), f.err
}

func (f *fakeClient) BatchCheck(_ context.Context, items []CheckItem) ([]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, items...)
	if f.err != nil {
		return nil, f.err
	}
	results := make([]bool, len(items))
	for index, item := range items {
		results[index] = f.decide(item)
	}
	return results, nil
}

func (f *fakeClient) ListObjects(context.Context, string, string, string, []Tuple) ([]string, error) {
	return f.listed, f.err
}

func (f *fakeClient) Write(_ context.Context, writes []Tuple, _ []Tuple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, writes...)
	return f.err
}

func (f *fakeClient) Read(context.Context, string, string) ([]Tuple, string, error) {
	return nil, "", f.err
}

func (f *fakeClient) ReadModel(context.Context, string) ([]byte, error) {
	return f.modelJSON, f.err
}

func testCoordinator(t *testing.T, client Client) (*Coordinator, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	coordinator := NewCoordinator(CoordinatorOptions{
		DB: db, Client: client, Projector: NewProjector(db, client, "it"),
		Config: common.ReBACConfig{Scope: "it", GroupClaim: "groups", ListObjectsMaxResults: 3, MaxScanCandidates: 10},
	})
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
	mock.ExpectQuery(`SELECT auth_uuid::text FROM "` + table + `"`).WillReturnRows(rows)
}

func expectNoPendingRevocation(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`FROM "rebac_outbox"`).WillReturnRows(sqlmock.NewRows([]string{"marker"}))
}

func submodelRequest(subject string) auth.ReBACRequest {
	return request(http.MethodGet, "/submodels/{submodelIdentifier}",
		map[string]string{paramSubmodel: common.EncodeString("urn:sm")}, subject, grammar.RightsEnumREAD)
}

func TestResolveGrantsTheCheckedResourceOnly(t *testing.T) {
	t.Parallel()

	alice := UserObject(testIssuer, "alice")
	client := &fakeClient{allowed: map[string]bool{alice + "|" + RelationCanRead + "|submodel:" + testSubmodelUUID: true}}
	coordinator, mock := testCoordinator(t, client)
	expectLookup(mock, "submodel", testSubmodelUUID)
	expectNoPendingRevocation(mock)

	grants, err := coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())
	require.NoError(t, mock.ExpectationsWereMet())
	require.Len(t, client.checks, 2, "resource check and repository admin check share one round trip")
	require.Equal(t, []Tuple{{User: alice, Relation: RelationMember, Object: GroupObject(testIssuer, "ops")}}, client.checks[0].Contextual,
		"group memberships are sent as contextual tuples")
}

func TestResolveKeepsABACOnlyForUnknownOrUngrantedResources(t *testing.T) {
	t.Parallel()

	coordinator, mock := testCoordinator(t, &fakeClient{allowed: map[string]bool{}})
	expectLookup(mock, "submodel", "")
	grants, err := coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "unknown identifiers behave like today")

	expectLookup(mock, "submodel", testSubmodelUUID)
	grants, err = coordinator.Resolve(t.Context(), submodelRequest("alice"))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "denied ReBAC keeps the ABAC result")
	require.NoError(t, mock.ExpectationsWereMet())

	grants, err = coordinator.Resolve(t.Context(), submodelRequest(""))
	require.NoError(t, err)
	require.True(t, grants.IsEmpty(), "anonymous callers are ABAC-only")
}

func TestResolveFailsClosedWhenOpenFGAOrTheBarrierCannotAnswer(t *testing.T) {
	t.Parallel()

	failing, mock := testCoordinator(t, &fakeClient{err: errors.New("timeout")})
	expectLookup(mock, "submodel", testSubmodelUUID)
	_, err := failing.Resolve(t.Context(), submodelRequest("alice"))
	require.Error(t, err)

	alice := UserObject(testIssuer, "alice")
	allowing, mock := testCoordinator(t, &fakeClient{allowed: map[string]bool{alice + "|" + RelationCanRead + "|submodel:" + testSubmodelUUID: true}})
	expectLookup(mock, "submodel", testSubmodelUUID)
	mock.ExpectQuery(`FROM "rebac_outbox"`).WillReturnRows(sqlmock.NewRows([]string{"marker"}).AddRow(1))
	_, err = allowing.Resolve(t.Context(), submodelRequest("alice"))
	require.ErrorIs(t, err, ErrRevocationPending)

	starting, _ := testCoordinator(t, &fakeClient{})
	starting.ready.Store(false)
	_, err = starting.Resolve(t.Context(), submodelRequest("alice"))
	require.ErrorIs(t, err, ErrNotReady)
}

func TestElementChecksCarryTheirAncestry(t *testing.T) {
	t.Parallel()

	alice := UserObject(testIssuer, "alice")
	element := ElementObject(testSubmodelUUID, "a.b")
	client := &fakeClient{allowed: map[string]bool{alice + "|" + RelationCanRead + "|" + element: true}}
	coordinator, mock := testCoordinator(t, client)
	expectLookup(mock, "submodel", testSubmodelUUID)
	expectNoPendingRevocation(mock)

	grants, err := coordinator.Resolve(t.Context(), request(http.MethodGet, "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
		map[string]string{paramSubmodel: common.EncodeString("urn:sm"), paramPath: "a.b"}, "alice", grammar.RightsEnumREAD))
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())
	require.Subset(t, client.checks[0].Contextual, ElementAncestry(testSubmodelUUID, "a.b"))
}

func TestElementDeletionRequiresUpdateOnTheParent(t *testing.T) {
	t.Parallel()

	alice := UserObject(testIssuer, "alice")
	client := &fakeClient{allowed: map[string]bool{alice + "|" + RelationCanUpdate + "|" + ElementObject(testSubmodelUUID, "a"): true}}
	coordinator, mock := testCoordinator(t, client)
	expectLookup(mock, "submodel", testSubmodelUUID)
	expectNoPendingRevocation(mock)
	_, err := coordinator.Resolve(t.Context(), request(http.MethodDelete, "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}",
		map[string]string{paramSubmodel: common.EncodeString("urn:sm"), paramPath: "a.b"}, "alice", grammar.RightsEnumDELETE))
	require.NoError(t, err)
	require.Equal(t, ElementObject(testSubmodelUUID, "a"), client.checks[0].Object)
}

func TestListsUseListObjectsAndFallBackToVerifiedCandidates(t *testing.T) {
	t.Parallel()

	listRequest := request(http.MethodGet, "/concept-descriptions", nil, "alice", grammar.RightsEnumREAD)
	withinCap, mock := testCoordinator(t, &fakeClient{listed: []string{"concept_description:" + testSubmodelUUID}})
	expectNoPendingRevocation(mock)
	grants, err := withinCap.Resolve(t.Context(), listRequest)
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())

	alice := UserObject(testIssuer, "alice")
	candidate := "2d8e7c50-8ca1-4d86-9c2e-2f3f614c3003"
	truncated := &fakeClient{
		listed:  []string{"concept_description:a", "concept_description:b", "concept_description:c"},
		allowed: map[string]bool{alice + "|" + RelationCanRead + "|concept_description:" + candidate: true},
	}
	scanning, mock := testCoordinator(t, truncated)
	mock.ExpectQuery(`FROM "rebac_grant"`).WillReturnRows(sqlmock.NewRows([]string{"object_uuid"}).AddRow(candidate).AddRow(testSubmodelUUID))
	expectNoPendingRevocation(mock)
	grants, err = scanning.Resolve(t.Context(), listRequest)
	require.NoError(t, err)
	require.False(t, grants.IsEmpty())
	require.NoError(t, mock.ExpectationsWereMet())

	adminClient := &fakeClient{allowed: map[string]bool{alice + "|" + RelationAdmin + "|repository:concept_description": true}}
	admin, mock := testCoordinator(t, adminClient)
	expectNoPendingRevocation(mock)
	grants, err = admin.Resolve(t.Context(), listRequest)
	require.NoError(t, err)
	require.False(t, grants.IsEmpty(), "repository admins see every object of the kind")
}

func TestTooManyGroupsFailInsteadOfTruncating(t *testing.T) {
	t.Parallel()

	groups := make([]any, maxContextualTuples+1)
	for index := range groups {
		groups[index] = "group-" + string(rune('a'+index%26)) + string(rune('a'+index/26))
	}
	coordinator, mock := testCoordinator(t, &fakeClient{})
	expectLookup(mock, "submodel", testSubmodelUUID)
	req := submodelRequest("alice")
	req.Claims["groups"] = groups
	_, err := coordinator.Resolve(t.Context(), req)
	require.ErrorIs(t, err, ErrTooManyContextualTuples)
}
