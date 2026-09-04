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
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	api "github.com/go-chi/chi/v5"
)

func TestEventReadPath(t *testing.T) {
	if got := eventReadPath(eventfeed.TypeAASCreated, "aas-1"); got != "/shells/"+common.EncodeString("aas-1") {
		t.Fatalf("aas path=%s", got)
	}
	if got := eventReadPath(eventfeed.TypeSubmodelUpdated, "sm-1"); got != "/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("sm path=%s", got)
	}
	if got := eventReadPath(eventfeed.TypePCN, "sm-1"); got != "/submodels/"+common.EncodeString("sm-1") {
		t.Fatalf("pcn path=%s", got)
	}
	if got := eventReadPath(eventfeed.TypeAssetDeleted, "asset-1"); got != "/lookup/shells" {
		t.Fatalf("asset path=%s", got)
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
	if !authorizer.Allow(context.Background(), eventfeed.TypeAASCreated, "aas-1") {
		t.Fatal("expected allow when ABAC is off")
	}
}
