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

package integration_tests

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
)

func TestOpenFGAModelPermissionsAndInheritance(t *testing.T) {
	client, endpoint, store, modelID := provisionOpenFGAModel(t)
	writeOpenFGATuples(t, endpoint, store, modelID, []rebac.Tuple{
		{User: "repository:root", Relation: "parent", Object: "aas:machine"},
		{User: "repository:root", Relation: "parent", Object: "submodel:temperature"},
		{User: "aas:machine", Relation: "aas_parent", Object: "submodel:temperature"},
		{User: "submodel:temperature", Relation: "parent", Object: "element:current"},
		{User: "user:viewer", Relation: "viewer", Object: "aas:machine"},
		{User: "user:editor", Relation: "editor", Object: "aas:machine"},
		{User: "user:executor", Relation: "executor", Object: "aas:machine"},
		{User: "user:owner", Relation: "owner", Object: "submodel:temperature"},
		{User: "user:aas-owner", Relation: "owner", Object: "aas:machine"},
	})

	assertOpenFGAPermission(t, client, "user:viewer", "read", "element:current", nil, true)
	assertOpenFGAPermission(t, client, "user:editor", "update", "element:current", nil, true)
	assertOpenFGAPermission(t, client, "user:executor", "execute", "element:current", nil, true)
	assertOpenFGAPermission(t, client, "user:owner", "delete", "element:current", nil, true)
	assertOpenFGAPermission(t, client, "user:owner", "manage", "element:current", nil, true)
	assertOpenFGAPermission(t, client, "user:aas-owner", "delete", "submodel:temperature", nil, false)
	assertOpenFGAPermission(t, client, "user:aas-owner", "manage", "submodel:temperature", nil, false)
}

func TestOpenFGAModelGeneratedSourceAndIssuerScopedGroups(t *testing.T) {
	client, endpoint, store, modelID := provisionOpenFGAModel(t)
	writeOpenFGATuples(t, endpoint, store, modelID, []rebac.Tuple{
		{User: "user:source-viewer", Relation: "viewer", Object: "aas:machine"},
		{User: "aas:machine", Relation: "source", Object: "aas_descriptor:generated"},
		{User: "aas_descriptor:generated", Relation: "source", Object: "discovery:generated"},
		{User: "group:issuer-a-operators#member", Relation: "viewer", Object: "aas:machine"},
	})

	assertOpenFGAPermission(t, client, "user:source-viewer", "read", "aas_descriptor:generated", nil, true)
	assertOpenFGAPermission(t, client, "user:source-viewer", "read", "discovery:generated", nil, true)
	assertOpenFGAPermission(t, client, "user:source-viewer", "update", "aas_descriptor:generated", nil, false)
	assertOpenFGAPermission(t, client, "user:source-viewer", "read", "aas_descriptor:independent", nil, false)

	issuerA := []rebac.Tuple{{User: "user:alice", Relation: "member", Object: "group:issuer-a-operators"}}
	issuerB := []rebac.Tuple{{User: "user:alice", Relation: "member", Object: "group:issuer-b-operators"}}
	assertOpenFGAPermission(t, client, "user:alice", "read", "aas:machine", issuerA, true)
	assertOpenFGAPermission(t, client, "user:alice", "read", "aas:machine", issuerB, false)
}

func provisionOpenFGAModel(t *testing.T) (*rebac.Client, string, string, string) {
	t.Helper()
	endpoint := os.Getenv("BASYX_REBAC_TEST_OPENFGA_URL")
	if endpoint == "" {
		t.Skip("BASYX_REBAC_TEST_OPENFGA_URL is required for live OpenFGA tests")
	}
	store := postJSON(t, endpoint+"/stores", map[string]string{"name": "basyx-model-" + uuid.NewString()})["id"].(string)
	model, err := rebac.AuthorizationModelJSON()
	if err != nil {
		t.Fatal(err)
	}
	modelID := postJSON(t, endpoint+"/stores/"+store+"/authorization-models", json.RawMessage(model))["authorization_model_id"].(string)
	client, err := rebac.NewClient(rebac.Config{URL: endpoint, Token: os.Getenv("BASYX_REBAC_TEST_OPENFGA_TOKEN"), StoreID: store, ModelID: modelID, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err = client.ReadModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	return client, endpoint, store, modelID
}

func writeOpenFGATuples(t *testing.T, endpoint, store, modelID string, tuples []rebac.Tuple) {
	t.Helper()
	postJSON(t, endpoint+"/stores/"+store+"/write", map[string]any{
		"authorization_model_id": modelID,
		"writes":                 map[string]any{"tuple_keys": tuples},
	})
}

func assertOpenFGAPermission(t *testing.T, client *rebac.Client, user, relation, object string, contextual []rebac.Tuple, want bool) {
	t.Helper()
	allowed, err := client.Check(t.Context(), user, relation, object, contextual)
	if err != nil || allowed != want {
		t.Fatalf("Check(%s, %s, %s) = %t, %v; want %t", user, relation, object, allowed, err, want)
	}
}
