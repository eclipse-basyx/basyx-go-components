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

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestDPPAggregatePreservesRouteOnlyABACFallback(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/v1/dpps/{dppId}", func(http.ResponseWriter, *http.Request) {})
	router.Get("/submodels/{submodelIdentifier}", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"DEFATTRIBUTES":[{"name":"role","attributes":[{"CLAIM":"role"}]}],"DEFOBJECTS":[{"name":"route","objects":[{"ROUTE":"/v1/dpps/*"}]}],"DEFACLS":[{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],"DEFFORMULAS":[{"name":"viewer","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"viewer"}]}}],"rules":[{"USEACL":"read","USEOBJECTS":["route"],"USEFORMULA":"viewer"}]}}`), router, "")
	require.NoError(t, err)
	evaluation := model.AuthorizeWithFilterWithOptions(EvalInput{Method: http.MethodGet, Path: "/v1/dpps/demo", RoutePath: "/v1/dpps/demo", Claims: Claims{"role": "viewer"}}, grammar.DefaultSimplifyOptions())
	require.True(t, evaluation.Allowed, "source evaluation: %+v", evaluation)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/check")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":false}`))
	}))
	defer server.Close()
	client, err := rebac.NewClient(rebac.Config{URL: server.URL, StoreID: "store", ModelID: "model", Timeout: time.Second})
	require.NoError(t, err)
	for _, sourceRoute := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectBegin()
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		mock.ExpectQuery(`SELECT .* FROM "rebac_resource"`).WillReturnRows(sqlmock.NewRows([]string{"resource_uuid", "kind", "identifier"}).AddRow("11111111-1111-4111-8111-111111111111", "submodel", "urn:sm:dpp"))
		security := &rebacSecurity{cfg: &common.Config{ReBAC: common.ReBACConfig{Scope: "test"}}, settings: ABACSettings{Model: model}, audit: &audit.Runtime{}, runtime: &rebac.Runtime{Client: client, State: &rebac.StateStore{DB: db, Scope: "test"}}}
		cache := &rebacReadGrantCache{}
		cache.once.Do(func() { cache.grants = map[SemanticResourceKind]ReBACReadGrantSet{} })
		request := &rebacRequest{actor: rebac.GrantActor{User: "user:reader"}, route: ReBACRoute{Kind: ReBACRouteKindSubmodel, Identifier: "urn:sm:dpp", Action: ReBACRouteActionRead}, readGrants: cache}
		if sourceRoute {
			request.fallbackMethod = http.MethodGet
			request.fallbackPath = "/v1/dpps/demo"
		}
		ctx := context.WithValue(t.Context(), ClaimsKey, Claims{"role": "viewer"})
		incoming := httptest.NewRequest(http.MethodGet, "/submodels/"+common.EncodeString("urn:sm:dpp"), nil).WithContext(ctx)
		result, err := security.authorizeRequest(incoming, tx, request)
		if sourceRoute {
			require.NoError(t, err)
			require.NotNil(t, AuthorizationSessionFromContext(result.Context()))
		} else {
			require.ErrorIs(t, err, rebac.ErrForbidden)
		}
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestReBACExcludedRoutesRemainOnABACMiddleware(t *testing.T) {
	paths := []string{"/v1/dppsByIdAndDate/passport", "/events", "/submodels/" + common.EncodeString("urn:sm") + "/$history"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				security := &rebacSecurity{cfg: &common.Config{}, settings: ABACSettings{Enabled: enabled}}
				called := false
				handler := security.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called = true
					require.Nil(t, r.Context().Value(rebacRequestKey{}))
					w.WriteHeader(http.StatusNoContent)
				}))
				ctx := context.WithValue(t.Context(), ClaimsKey, Claims{"role": "member"})
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
				if enabled {
					require.Equal(t, http.StatusForbidden, response.Code)
					require.False(t, called)
				} else {
					require.Equal(t, http.StatusNoContent, response.Code)
					require.True(t, called)
				}
			}
		})
	}
}

func TestReBACFingerprintSeparatesIntegrationRuntimeSettings(t *testing.T) {
	cfg := &common.Config{ReBAC: common.ReBACConfig{Enabled: true, Scope: "deployment", ModelID: "pinned", StoreID: "store"}}
	original, err := rebacConfigurationFingerprint(cfg)
	require.NoError(t, err)
	cfg.General.AASRegistryIntegration = true
	cfg.General.SubmodelRegistryIntegration = true
	cfg.General.DiscoveryIntegration = true
	cfg.ReBAC.Token = "rotated-credential"
	changedIntegrations, err := rebacConfigurationFingerprint(cfg)
	require.NoError(t, err)
	require.Equal(t, original, changedIntegrations)
	cfg.ReBAC.ModelID = "incompatible"
	changedModel, err := rebacConfigurationFingerprint(cfg)
	require.NoError(t, err)
	require.NotEqual(t, original, changedModel)
}
