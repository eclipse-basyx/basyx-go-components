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

package rebac

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckReturnsExplicitDenial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/stores/store/check" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("request did not carry configured bearer token")
		}
		var payload checkRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode check request: %v", err)
		}
		if payload.Consistency != higherConsistency {
			t.Fatalf("consistency = %q, want %q", payload.Consistency, higherConsistency)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"allowed":false}`))
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	allowed, err := client.Check(t.Context(), "user:alice", "viewer", "document:one", nil)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if allowed {
		t.Fatal("Check must preserve an explicit OpenFGA denial")
	}
}

func TestCheckDoesNotExposeOpenFGAErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("credential=server-secret"))
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	_, err := client.Check(t.Context(), "user:alice", "viewer", "document:one", nil)
	if err == nil {
		t.Fatal("Check accepted an OpenFGA server error")
	}
	if strings.Contains(err.Error(), "server-secret") {
		t.Fatalf("Check leaked the OpenFGA error body: %v", err)
	}
}

func TestCheckDoesNotFollowRedirectWithBearerToken(t *testing.T) {
	redirectTargetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectTargetCalls++
		if request.Header.Get("Authorization") != "" {
			t.Fatal("redirect target received bearer token")
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL)
		writer.WriteHeader(http.StatusFound)
	}))
	defer origin.Close()
	client := testClient(t, origin.URL)

	_, err := client.Check(t.Context(), "user:alice", "viewer", "document:one", nil)
	if err == nil || !strings.Contains(err.Error(), "REBAC-CLIENT-STATUS") {
		t.Fatalf("Check error = %v, want redirect status error", err)
	}
	if redirectTargetCalls != 0 {
		t.Fatal("client followed redirect")
	}
}

func TestNewClientRejectsSensitiveURLComponents(t *testing.T) {
	for _, endpoint := range []string{"https://token@example.test", "https://example.test?token=secret", "https://example.test#token"} {
		_, err := NewClient(Config{URL: endpoint, StoreID: "store", ModelID: "model", Timeout: time.Second})
		if err == nil || !strings.Contains(err.Error(), "REBAC-CLIENT-VALIDATE-URL") {
			t.Fatalf("NewClient(%q) error = %v, want URL validation error", endpoint, err)
		}
	}
}

func TestCheckFailsClosedForMalformedAllowedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	allowed, err := client.Check(t.Context(), "user:alice", "viewer", "document:one", nil)
	if err == nil {
		t.Fatal("Check accepted a response without allowed")
	}
	if allowed {
		t.Fatal("malformed response must fail closed")
	}
	if !strings.Contains(err.Error(), "REBAC-CLIENT-CHECK-RESPONSE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckPropagatesCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("canceled request must not reach OpenFGA")
	}))
	defer server.Close()
	client := testClient(t, server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	allowed, err := client.Check(ctx, "user:alice", "viewer", "document:one", nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Check error = %v, want context cancellation", err)
	}
	if allowed {
		t.Fatal("canceled request must fail closed")
	}
}

func TestBatchCheckRequiresEveryCorrelatedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"result":{"first":{"allowed":true}}}`))
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	results, err := client.BatchCheck(t.Context(), []BatchCheckRequest{
		{CorrelationID: "first", Tuple: Tuple{User: "user:alice", Relation: "viewer", Object: "document:one"}},
		{CorrelationID: "second", Tuple: Tuple{User: "user:bob", Relation: "viewer", Object: "document:two"}},
	})
	if err == nil {
		t.Fatal("BatchCheck accepted a partial result")
	}
	if results != nil {
		t.Fatalf("BatchCheck results = %#v, want nil on malformed response", results)
	}
}

func TestStreamedListObjectsReadsAllAuthorizedObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/stores/store/streamed-list-objects" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		var payload listObjectsRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.AuthorizationModelID != "model" || payload.Consistency != higherConsistency || payload.Type != "submodel" || payload.Relation != "read" || payload.User != "user:alice" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		_, _ = writer.Write([]byte("{\"result\":{\"object\":\"submodel:first\"}}\n{\"result\":{\"object\":\"submodel:second\"}}\n"))
	}))
	defer server.Close()

	objects, err := testClient(t, server.URL).StreamedListObjects(t.Context(), "user:alice", "read", "submodel", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0] != "first" || objects[1] != "second" {
		t.Fatalf("unexpected objects: %v", objects)
	}
}

func TestStreamedListObjectsRejectsUnexpectedType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("{\"result\":{\"object\":\"aas:other\"}}\n"))
	}))
	defer server.Close()

	objects, err := testClient(t, server.URL).StreamedListObjects(t.Context(), "user:alice", "read", "submodel", nil)
	if err == nil || objects != nil {
		t.Fatalf("unexpected result: %v, %v", objects, err)
	}
}

func TestReadModelRejectsUnexpectedPinnedModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"authorization_model":{"id":"other-model"}}`))
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	err := client.ReadModel(t.Context())
	if err == nil || !strings.Contains(err.Error(), "REBAC-CLIENT-MODEL-RESPONSE") {
		t.Fatalf("ReadModel error = %v, want pinned-model validation error", err)
	}
}

func TestReadModelValidatesPinnedAuthorizationModel(t *testing.T) {
	model, err := AuthorizationModelJSON()
	if err != nil {
		t.Fatalf("marshal expected model: %v", err)
	}
	var payload map[string]any
	if err = json.Unmarshal(model, &payload); err != nil {
		t.Fatalf("decode expected model: %v", err)
	}
	payload["id"] = "model"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{"authorization_model": payload}); err != nil {
			t.Fatalf("encode model response: %v", err)
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL)

	if err = client.ReadModel(t.Context()); err != nil {
		t.Fatalf("ReadModel rejected matching pinned model: %v", err)
	}
}

func testClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := NewClient(Config{URL: endpoint, StoreID: "store", ModelID: "model", Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func TestBatchCheckServerMapResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"correlated", `{"result":{"second":{"allowed":false},"first":{"allowed":true}}}`, false},
		{"item failure", `{"result":{"first":{"allowed":true},"second":{"allowed":false,"error":{"code":2000,"message":"unavailable"}}}}`, true},
		{"unknown key", `{"result":{"first":{"allowed":true},"other":{"allowed":false}}}`, true},
		{"missing decision", `{"result":{"first":{"allowed":true},"second":{}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := testClient(t, server.URL)
			result, err := client.BatchCheck(t.Context(), []BatchCheckRequest{
				{CorrelationID: "first", Tuple: Tuple{User: "user:alice", Relation: "read", Object: "aas:one"}},
				{CorrelationID: "second", Tuple: Tuple{User: "user:alice", Relation: "read", Object: "aas:two"}},
			})
			if tc.fail {
				if err == nil || result != nil {
					t.Fatalf("partial result accepted: %v %v", result, err)
				}
				return
			}
			if err != nil || len(result) != 2 || !result[0].Allowed || result[1].Allowed {
				t.Fatalf("incorrect correlation: %v %v", result, err)
			}
		})
	}
}

func TestReadTuplesUsesHighConsistencyAndContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path != "/stores/store/read" || payload["consistency"] != higherConsistency || payload["continuation_token"] != "previous" {
			t.Fatalf("unexpected read: %s %+v", request.URL.Path, payload)
		}
		_, _ = writer.Write([]byte(`{"tuples":[{"key":{"user":"user:alice","relation":"viewer","object":"aas:resource"}}],"continuation_token":"next"}`))
	}))
	defer server.Close()
	tuples, cursor, err := testClient(t, server.URL).ReadTuples(t.Context(), "previous")
	if err != nil || cursor != "next" || len(tuples) != 1 || tuples[0].Object != "aas:resource" {
		t.Fatalf("read result: %+v %q %v", tuples, cursor, err)
	}
}
