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
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	api "github.com/go-chi/chi/v5"
)

func TestEventReadPath(t *testing.T) {
	if got := eventReadPath("", eventfeed.TypeAASCreated, "aas-1"); got != "/shells/"+common.EncodeString("aas-1") {
		t.Fatalf("aas path=%s", got)
	}
	if got := eventReadPath("", eventfeed.TypeSubmodelUpdated, "sm-1"); got != "/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("sm path=%s", got)
	}
	if got := eventReadPath("", eventfeed.TypePCN, "sm-1"); got != "/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("pcn path=%s", got)
	}
	if got := eventReadPath("", eventfeed.TypeAssetDeleted, "asset-1"); got != "/lookup/shells" {
		t.Fatalf("asset path=%s", got)
	}
	if got := eventReadPath("/api/v3", eventfeed.TypeAASCreated, "aas-1"); got != "/api/v3/shells/"+common.EncodeString("aas-1") {
		t.Fatalf("context path aas=%s", got)
	}
	if got := eventReadPath("/api/v3", eventfeed.TypeSubmodelUpdated, "sm-1"); got != "/api/v3/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("context path sm=%s", got)
	}
	if got := eventReadPath("/api/v3", eventfeed.TypePCN, "sm-1"); got != "/api/v3/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("context path pcn=%s", got)
	}
	if got := eventReadPath("/api/v3", eventfeed.TypeAssetDeleted, "asset-1"); got != "/api/v3/lookup/shells" {
		t.Fatalf("context path asset=%s", got)
	}
}

func TestEventRecordAuthorizerRequiresClaims(t *testing.T) {
	router := api.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/shells/{aasIdentifier}", noop)
	router.Get("/submodels/{submodelIdentifier}", noop)
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "sub_claim", "attributes": [ { "CLAIM": "sub" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "shells_api", "objects": [ { "IDENTIFIABLE": "$aas(\"*\")" } ] }
			],
			"DEFACLS": [
				{ "name": "read_access", "acl": { "USEATTRIBUTES": "sub_claim", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "always_true", "formula": { "$boolean": true } }
			],
			"rules": [
				{ "USEACL": "read_access", "USEOBJECTS": [ "shells_api" ], "USEFORMULA": "always_true" }
			]
		}
	}`), router, "")
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: true, Model: model}}
	if authorizer.Allow(context.Background(), eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected deny without claims")
	}
	ctx := context.WithValue(context.Background(), ClaimsKey, Claims{"sub": "user"})
	if !authorizer.Allow(ctx, eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected allow with claims for /shells/*")
	}
	if authorizer.Allow(ctx, eventfeed.TypeSubmodelCreated, "sm-1") {
		t.Fatal("expected deny for submodel when policy only covers AAS")
	}
}

func TestEventRecordAuthorizerDisabledAllows(t *testing.T) {
	authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: false}}
	require.True(t, authorizer.AllowEvent(t.Context(), eventfeed.FeedEvent{Type: eventfeed.TypeAssetUpdated, Subject: "asset-without-provenance"}))
	if !authorizer.Allow(context.Background(), eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected allow when ABAC is off")
	}
}

func TestEventRecordAuthorizerFailsClosedOnRemainingQueryFilter(t *testing.T) {
	router := api.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/shells/{aasIdentifier}", noop)
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "sub_claim", "attributes": [ { "CLAIM": "sub" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "shells_api", "objects": [ { "IDENTIFIABLE": "$aas(\"*\")" } ] }
			],
			"DEFACLS": [
				{ "name": "read_access", "acl": { "USEATTRIBUTES": "sub_claim", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "id_short_eq", "formula": { "$eq": [ { "$field": "$aas#idShort" }, { "$strVal": "visible" } ] } }
			],
			"rules": [
				{ "USEACL": "read_access", "USEOBJECTS": [ "shells_api" ], "USEFORMULA": "id_short_eq" }
			]
		}
	}`), router, "/api/v3")
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: true, Model: model}}
	ctx := context.WithValue(context.Background(), ClaimsKey, Claims{"sub": "user"})
	if authorizer.Allow(ctx, eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected deny when allow still requires a remaining QueryFilter")
	}
}

func TestEventRecordAuthorizerUsesContextPath(t *testing.T) {
	router := api.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/shells/{aasIdentifier}", noop)
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "sub_claim", "attributes": [ { "CLAIM": "sub" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "shells_api", "objects": [ { "IDENTIFIABLE": "$aas(\"*\")" } ] }
			],
			"DEFACLS": [
				{ "name": "read_access", "acl": { "USEATTRIBUTES": "sub_claim", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "always_true", "formula": { "$boolean": true } }
			],
			"rules": [
				{ "USEACL": "read_access", "USEOBJECTS": [ "shells_api" ], "USEFORMULA": "always_true" }
			]
		}
	}`), router, "/api/v3")
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: true, Model: model}}
	ctx := context.WithValue(context.Background(), ClaimsKey, Claims{"sub": "user"})
	if !authorizer.Allow(ctx, eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected allow when policy matches /api/v3/shells/*")
	}
}

// TestEventRecordAuthorizerUsesContextPathForSubmodelRoutes covers the
// submodel and PCN routes behind a configured server.contextPath: the
// synthetic read request must carry the same prefix the IDENTIFIABLE policy
// mapping does, otherwise an authorized caller sees an empty feed.
func TestEventRecordAuthorizerUsesContextPathForSubmodelRoutes(t *testing.T) {
	router := api.NewRouter()
	noop := func(http.ResponseWriter, *http.Request) {}
	router.Get("/submodels/{submodelIdentifier}", noop)
	model, err := ParseAccessModel([]byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "sub_claim", "attributes": [ { "CLAIM": "sub" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "submodels_api", "objects": [ { "IDENTIFIABLE": "$sm(\"*\")" } ] }
			],
			"DEFACLS": [
				{ "name": "read_access", "acl": { "USEATTRIBUTES": "sub_claim", "RIGHTS": [ "READ" ], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "always_true", "formula": { "$boolean": true } }
			],
			"rules": [
				{ "USEACL": "read_access", "USEOBJECTS": [ "submodels_api" ], "USEFORMULA": "always_true" }
			]
		}
	}`), router, "/api/v3")
	if err != nil {
		t.Fatalf("model: %v", err)
	}
	authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: true, Model: model}}
	ctx := context.WithValue(context.Background(), ClaimsKey, Claims{"sub": "user"})
	if !authorizer.Allow(ctx, eventfeed.TypeSubmodelUpdated, "sm-1") {
		t.Fatal("expected allow for submodel event when policy matches /api/v3/submodels/*")
	}
	if !authorizer.Allow(ctx, eventfeed.TypePCN, "sm-1") {
		t.Fatal("expected allow for PCN event when policy matches /api/v3/submodels/*")
	}
}

func TestEventAuthorizationRequiresHistoricalAASOwnership(t *testing.T) {
	for _, tc := range []struct {
		name      string
		objects   string
		eventType string
		owners    []string
		allowed   bool
	}{
		{"submodel private owner", `$sm("*")`, eventfeed.TypeSubmodelUpdated, []string{"private-aas"}, false},
		{"pcn private owner", `$sm("*")`, eventfeed.TypePCN, []string{"private-aas"}, false},
		{"unattached submodel", `$sm("*")`, eventfeed.TypeSubmodelUpdated, []string{}, true},
		{"unknown submodel provenance", `$sm("*")`, eventfeed.TypeSubmodelUpdated, nil, false},
		{"lookup cannot read asset owner", "lookup", eventfeed.TypeAssetUpdated, []string{"private-aas"}, false},
		{"asset owner visible", `$aas("*")`, eventfeed.TypeAssetUpdated, []string{"public-aas"}, true},
		{"only one owner visible", `$aas("public-aas")`, eventfeed.TypeAssetUpdated, []string{"public-aas", "private-aas"}, false},
		{"asset without owner", `$aas("*")`, eventfeed.TypeAssetUpdated, []string{}, false},
		{"unknown asset provenance", `$aas("*")`, eventfeed.TypeAssetUpdated, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := api.NewRouter()
			noop := func(http.ResponseWriter, *http.Request) {}
			router.Get("/shells/{aasIdentifier}", noop)
			router.Get("/submodels/{submodelIdentifier}", noop)
			router.Get("/lookup/shells", noop)
			router.Get("/.well-known/event-feed/schemas/{schema}", noop)
			object := map[string]string{"IDENTIFIABLE": tc.objects}
			if tc.objects == "lookup" {
				object = map[string]string{"ROUTE": "/lookup/shells"}
			}
			encoded, err := json.Marshal(object)
			require.NoError(t, err)
			model, err := ParseAccessModel([]byte(fmt.Sprintf(`{"AllAccessPermissionRules":{
				"DEFATTRIBUTES":[{"name":"user","attributes":[{"CLAIM":"sub"}]}],
				"DEFOBJECTS":[{"name":"objects","objects":[%s]}],
				"DEFACLS":[{"name":"read","acl":{"USEATTRIBUTES":"user","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],
				"DEFFORMULAS":[{"name":"all","formula":{"$boolean":true}}],
				"rules":[{"USEACL":"read","USEOBJECTS":["objects"],"USEFORMULA":"all"}]}}`, encoded)), router, "/api/v3")
			require.NoError(t, err)
			authorizer := EventRecordAuthorizer{Settings: ABACSettings{Enabled: true, Model: model}}
			ctx := context.WithValue(t.Context(), ClaimsKey, Claims{"sub": "reader"})
			event := eventfeed.FeedEvent{Type: tc.eventType, Subject: "sm-1", AuthorizationAASIDs: tc.owners}
			allowed := authorizer.AllowEvent(ctx, event)
			require.Equal(t, tc.allowed, allowed)
			if tc.objects != "lookup" {
				schemaAllowed, _, _ := model.AuthorizeWithFilter(EvalInput{
					Method: http.MethodGet,
					Path:   "/api/v3/.well-known/event-feed/schemas/metamodel-submodelChangeEvent.v1.schema.json",
					Claims: Claims{"sub": "reader"},
				})
				require.True(t, schemaAllowed, "existing identifiable readers can retrieve advertised schemas")
			}
		})
	}
}
