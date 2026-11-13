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
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // registers the postgres dialect
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database driver
	"github.com/stretchr/testify/require"
)

// routeScopeFixture holds a Submodel that shells A and B both reference. The
// viewer reads it under different ROUTE rules.
type routeScopeFixture struct {
	adminToken          string
	viewerToken         string
	basePolicyVersionID int64
	aasAID              string
	aasBID              string
	submodelID          string
	submodelIDShort     string
	encodedAASA         string
	encodedAASB         string
	encodedSubmodel     string
	propValue           string
	otherValue          string
	siblingValue        string
	legacyValue         string
}

func TestRouteRulesGrantTheSameDataDirectlyAndInQueries(t *testing.T) {
	fixture := newRouteScopeFixture(t, "route-scope")

	t.Run("element route grants the element but not its parent or siblings", func(t *testing.T) {
		fixture.activateViewerRoutes(t, fixture.submodelPath("/submodel-elements/Coll.Prop"))

		fixture.assertViewerReads(t, fixture.submodelPath("/submodel-elements/Coll.Prop"), fixture.propValue)
		fixture.assertViewerDenied(t, fixture.submodelPath(""))
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll"))
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll.Other"))
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Sibling"))

		fixture.assertQueryMatches(t, "$sme.Coll.Prop#value", fixture.propValue, fixture.aasAID, fixture.aasBID)
		fixture.assertQueryMatches(t, "$sme.Coll.Other#value", fixture.otherValue)
		fixture.assertQueryMatches(t, "$sme.Sibling#value", fixture.siblingValue)
		fixture.assertQueryMatches(t, "$sme.Coll#idShort", "Coll")
		fixture.assertQueryMatches(t, "$sm#idShort", fixture.submodelIDShort)
	})

	t.Run("collection route grants the subtree of the collection", func(t *testing.T) {
		fixture.activateViewerRoutes(t, fixture.submodelPath("/submodel-elements/Coll"))

		fixture.assertViewerReads(t, fixture.submodelPath("/submodel-elements/Coll"), fixture.propValue, fixture.otherValue)
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Sibling"))

		fixture.assertQueryMatches(t, "$sme.Coll.Prop#value", fixture.propValue, fixture.aasAID, fixture.aasBID)
		fixture.assertQueryMatches(t, "$sme.Coll.Other#value", fixture.otherValue, fixture.aasAID, fixture.aasBID)
		fixture.assertQueryMatches(t, "$sme.Sibling#value", fixture.siblingValue)
	})

	t.Run("representation routes grant no element values in queries", func(t *testing.T) {
		fixture.activateViewerRoutes(t,
			fixture.submodelPath("/submodel-elements/Coll.Prop/$value"),
			fixture.submodelPath("/submodel-elements/Coll.Other/$metadata"),
		)

		fixture.assertViewerReads(t, fixture.submodelPath("/submodel-elements/Coll.Prop/$value"), fixture.propValue)
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll.Prop"))
		metadata := fixture.assertViewerReads(t, fixture.submodelPath("/submodel-elements/Coll.Other/$metadata"))
		require.NotContains(t, metadata, fixture.otherValue)
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll.Other"))

		fixture.assertQueryMatches(t, "$sme.Coll.Prop#value", fixture.propValue)
		fixture.assertQueryMatches(t, "$sme.Coll.Other#value", fixture.otherValue)
	})

	t.Run("superpath route grants the element only within its shell", func(t *testing.T) {
		fixture.activateViewerRoutes(t,
			"/shells/"+fixture.encodedAASA,
			"/shells/"+fixture.encodedAASB,
			fixture.superpath(fixture.encodedAASA, "/submodel-elements/Coll.Prop"),
		)

		fixture.assertViewerReads(t, fixture.superpath(fixture.encodedAASA, "/submodel-elements/Coll.Prop"), fixture.propValue)
		fixture.assertViewerDenied(t, fixture.superpath(fixture.encodedAASB, "/submodel-elements/Coll.Prop"))
		fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll.Prop"))

		fixture.assertQueryMatches(t, "$sme.Coll.Prop#value", fixture.propValue, fixture.aasAID)
		fixture.assertQueryMatches(t, "$sme.Coll.Other#value", fixture.otherValue)
	})
}

func TestAttachmentRouteDoesNotGrantElementsWhoseIDShortContainsAnEncodedSlash(t *testing.T) {
	fixture := newRouteScopeFixture(t, "legacy-idshort")
	fixture.renameElementInDatabase(t, "Coll.Legacy", "x%2Fattachment", "Coll.x%2Fattachment")
	fixture.activateViewerRoutes(t, "/submodels/*/submodel-elements/*/attachment")

	fixture.assertViewerDenied(t, fixture.submodelPath("/submodel-elements/Coll.x%2Fattachment"))
	assertStatus(t, http.MethodGet, testBaseURL+fixture.submodelPath("/submodel-elements/Coll.Missing/attachment"), "", fixture.viewerToken, http.StatusNotFound)

	assertStatus(t, http.MethodGet, testBaseURL+fixture.submodelPath("/submodel-elements/Coll.x%2Fattachment"), "", fixture.adminToken, http.StatusBadRequest)
	status, body := doAuthorizedRequest(t, http.MethodGet, testBaseURL+fixture.submodelPath(""), "", fixture.adminToken)
	require.Equalf(t, http.StatusOK, status, "read Submodel with legacy element failed: %s", body)
	require.Contains(t, body, fixture.legacyValue)
}

func newRouteScopeFixture(t *testing.T, scenario string) *routeScopeFixture {
	t.Helper()

	tokenProvider := testenv.NewPasswordGrantTokenProvider(testKeycloakTokenURL, "basyx-ui", 10*time.Second)
	adminToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	viewerToken, err := tokenProvider.GetAccessToken(&testenv.TokenCredentials{User: "usera", Password: "pwd"})
	require.NoError(t, err)

	suffix := time.Now().UnixNano()
	fixture := &routeScopeFixture{
		adminToken:          adminToken,
		viewerToken:         viewerToken,
		basePolicyVersionID: activePolicyVersionID(t, adminToken),
		aasAID:              fmt.Sprintf("urn:test:aas:%s-a:%d", scenario, suffix),
		aasBID:              fmt.Sprintf("urn:test:aas:%s-b:%d", scenario, suffix),
		submodelID:          fmt.Sprintf("urn:test:sm:%s:%d", scenario, suffix),
		submodelIDShort:     fmt.Sprintf("RouteScope%d", suffix),
		propValue:           fmt.Sprintf("prop-%d", suffix),
		otherValue:          fmt.Sprintf("other-%d", suffix),
		siblingValue:        fmt.Sprintf("sibling-%d", suffix),
		legacyValue:         fmt.Sprintf("legacy-%d", suffix),
	}
	fixture.encodedAASA = base64.RawURLEncoding.EncodeToString([]byte(fixture.aasAID))
	fixture.encodedAASB = base64.RawURLEncoding.EncodeToString([]byte(fixture.aasBID))
	fixture.encodedSubmodel = base64.RawURLEncoding.EncodeToString([]byte(fixture.submodelID))
	t.Cleanup(func() { fixture.activateViewerRoutes(t) })

	createStoryAAS(t, fixture.aasAID, "RouteScopeA", adminToken)
	createStoryAAS(t, fixture.aasBID, "RouteScopeB", adminToken)
	assertStatus(t, http.MethodPut, testBaseURL+fixture.superpath(fixture.encodedAASA, ""), fixture.submodelBody(), adminToken, http.StatusCreated)
	submodelReference := fmt.Sprintf(`{"type":"ModelReference","keys":[{"type":"Submodel","value":%q}]}`, fixture.submodelID)
	assertStatus(t, http.MethodPost, testBaseURL+"/shells/"+fixture.encodedAASB+"/submodel-refs", submodelReference, adminToken, http.StatusCreated)
	return fixture
}

func (f *routeScopeFixture) submodelBody() string {
	property := func(idShort string, value string) string {
		return fmt.Sprintf(`{"idShort":%q,"modelType":"Property","valueType":"xs:string","value":%q}`, idShort, value)
	}
	return fmt.Sprintf(`{
		"id":%q,
		"idShort":%q,
		"modelType":"Submodel",
		"kind":"Instance",
		"submodelElements":[
			{"idShort":"Coll","modelType":"SubmodelElementCollection","value":[%s,%s,%s]},
			%s
		]
	}`, f.submodelID, f.submodelIDShort,
		property("Prop", f.propValue), property("Other", f.otherValue), property("Legacy", f.legacyValue),
		property("Sibling", f.siblingValue))
}

func (f *routeScopeFixture) submodelPath(suffix string) string {
	return "/submodels/" + f.encodedSubmodel + suffix
}

func (f *routeScopeFixture) superpath(encodedAASID string, suffix string) string {
	return "/shells/" + encodedAASID + "/submodels/" + f.encodedSubmodel + suffix
}

// activateViewerRoutes activates the base policy plus one viewer READ rule
// per route, so every step starts from the same policy.
func (f *routeScopeFixture) activateViewerRoutes(t *testing.T, routes ...string) {
	t.Helper()

	versionID := clonePolicyVersion(t, f.basePolicyVersionID, f.adminToken)
	for _, route := range routes {
		createRoleRouteReadRule(t, versionID, "viewer", route, f.adminToken)
	}
	validatePolicyVersion(t, versionID, f.adminToken)
	activatePolicyVersion(t, versionID, f.adminToken)
}

func (f *routeScopeFixture) assertViewerReads(t *testing.T, path string, expectedValues ...string) string {
	t.Helper()

	status, body := doAuthorizedRequest(t, http.MethodGet, testBaseURL+path, "", f.viewerToken)
	require.Equalf(t, http.StatusOK, status, "GET %s: %s", path, body)
	for _, value := range expectedValues {
		require.Containsf(t, body, value, "GET %s", path)
	}
	return body
}

func (f *routeScopeFixture) assertViewerDenied(t *testing.T, path string) {
	t.Helper()

	assertStatus(t, http.MethodGet, testBaseURL+path, "", f.viewerToken, http.StatusForbidden)
}

func (f *routeScopeFixture) assertQueryMatches(t *testing.T, field string, value string, expectedAASIDs ...string) {
	t.Helper()

	assertAuthorizedAASQueryIDs(t, f.viewerToken, equalsQueryCondition(field, value), expectedAASIDs...)
}

// renameElementInDatabase gives an element an idShort that the API no longer
// accepts, as data stored by older versions may contain.
func (f *routeScopeFixture) renameElementInDatabase(t *testing.T, idShortPath string, newIDShort string, newIDShortPath string) {
	t.Helper()

	db, err := sql.Open("pgx", testDatabaseURL)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	dialect := goqu.Dialect("postgres")
	submodelRowID := dialect.From("submodel").Select("id").Where(goqu.C("submodel_identifier").Eq(f.submodelID))
	query, args, err := dialect.
		Update("submodel_element").
		Set(goqu.Record{"id_short": newIDShort, "idshort_path": newIDShortPath}).
		Where(goqu.C("submodel_id").In(submodelRowID), goqu.C("idshort_path").Eq(idShortPath)).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	result, err := db.Exec(query, args...)
	require.NoError(t, err)
	affected, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, affected)
}
