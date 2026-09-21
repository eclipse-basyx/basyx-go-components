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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	aasp "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeedsetup"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	smp "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestEventFeedPreservesSourcePermissionsAfterAASDeletion(t *testing.T) {
	db, err := sql.Open("pgx", integrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cfg := eventfeed.DefaultConfig()
	cfg.Enabled = true
	module, err := eventfeed.NewModule(db, cfg)
	require.NoError(t, err)
	eventfeedsetup.Bind(module)
	t.Cleanup(module.Stop)
	previous := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previous) })
	aasdb, err := aasp.NewAssetAdministrationShellDatabaseFromDB(db, "off")
	require.NoError(t, err)
	smdb, err := smp.NewSubmodelDatabaseFromDB(db, nil, "off")
	require.NoError(t, err)
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	aasID := fmt.Sprintf("urn:example:event-security:%d", time.Now().UnixNano())
	smID, assetID := aasID+":sm", aasID+":private-asset"
	aas := types.NewAssetAdministrationShell(aasID, types.NewAssetInformation(types.AssetKindInstance))
	aas.AssetInformation().SetGlobalAssetID(&assetID)
	aas.SetSubmodels([]types.IReference{types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, smID)})})
	require.NoError(t, aasdb.CreateAssetAdministrationShell(ctx, aas))
	require.NoError(t, smdb.CreateSubmodel(ctx, types.NewSubmodel(smID)))
	require.NoError(t, aasdb.DeleteAssetAdministrationShellByID(ctx, aasID))
	t.Cleanup(func() { _ = smdb.DeleteSubmodel(context.WithoutCancel(ctx), smID) })
	_, err = module.Service.RunPublishAssignment(ctx)
	require.NoError(t, err)
	router, settings := eventSecurityRouter(t, module.Service)
	auth.BindEventFeedAuthorizer(settings)
	t.Cleanup(func() { eventfeed.SetRecordAuthorizer(nil) })
	filter := fmt.Sprintf("rsql:event.subject=in=(%s,%s,%s)", aasID, smID, assetID)
	for _, presentation := range []string{"REGULAR", "COMPACT"} {
		for _, role := range []string{"reader", "owner", "restricted-owner"} {
			t.Run(presentation+"/"+role, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/events?"+url.Values{"filter": {filter}, "presentation": {presentation}}.Encode(), nil)
				request = request.WithContext(context.WithValue(ctx, auth.ClaimsKey, auth.Claims{"role": role}))
				recorder := httptest.NewRecorder()
				auth.ABACMiddleware(settings)(router).ServeHTTP(recorder, request)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				var response eventfeed.FeedResponse
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				if role != "owner" {
					require.Empty(t, response.Records, recorder.Body.String())
					require.NotContains(t, recorder.Body.String(), assetID)
					return
				}
				require.Len(t, response.Records, 5, recorder.Body.String())
				require.Contains(t, recorder.Body.String(), assetID)
			})
		}
	}
}

func eventSecurityRouter(t *testing.T, svc *eventfeed.Service) (chi.Router, auth.ABACSettings) {
	t.Helper()
	router := chi.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/shells/{aasIdentifier}", noop)
	router.Get("/submodels/{submodelIdentifier}", noop)
	router.Get("/lookup/shells", noop)
	eventfeed.RegisterRoutes(router, svc)
	model, err := auth.ParseAccessModel([]byte(`{"AllAccessPermissionRules":{
 "DEFATTRIBUTES":[{"name":"role","attributes":[{"CLAIM":"role"}]}],
 "DEFOBJECTS":[
  {"name":"sm","objects":[{"IDENTIFIABLE":"$sm(\"*\")"}]},
  {"name":"aas","objects":[{"IDENTIFIABLE":"$aas(\"*\")"}]},
  {"name":"lookup","objects":[{"ROUTE":"/lookup/shells"}]}
 ],
 "DEFACLS":[{"name":"read","acl":{"USEATTRIBUTES":"role","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
 "DEFFORMULAS":[
  {"name":"all","formula":{"$boolean":true}},
  {"name":"owner","formula":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"owner"}]}},
  {"name":"restricted","formula":{"$and":[
   {"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"restricted-owner"}]},
   {"$eq":[{"$field":"$aas#idShort"},{"$strVal":"visible"}]}
  ]}}
 ],
 "rules":[
  {"USEACL":"read","USEOBJECTS":["sm","lookup"],"USEFORMULA":"all"},
  {"USEACL":"read","USEOBJECTS":["aas"],"USEFORMULA":"owner"},
  {"USEACL":"read","USEOBJECTS":["aas"],"USEFORMULA":"restricted"}
 ]}}`), router, "")
	require.NoError(t, err)
	return router, auth.ABACSettings{Enabled: true, Model: model}
}
