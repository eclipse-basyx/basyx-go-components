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

package testenv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func conformanceRequest(t *testing.T, method, endpoint string, body any, expected int, headers ...http.Header) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, endpoint, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) > 0 {
		request.Header = headers[0].Clone()
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected {
		t.Fatalf("%s %s: status=%d body=%s", method, endpoint, response.StatusCode, data)
	}
	var result map[string]any
	if len(data) > 0 && expected != http.StatusNoContent {
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func referenceFixture(value string, fragments ...string) map[string]any {
	keys := []map[string]any{{"type": "GlobalReference", "value": value}}
	for _, fragment := range fragments {
		keys = append(keys, map[string]any{"type": "FragmentReference", "value": fragment})
	}
	return map[string]any{"type": "ExternalReference", "keys": keys}
}

func encodedReferenceFixture(t *testing.T, reference map[string]any) string {
	t.Helper()
	data, err := json.MarshalIndent(reference, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return common.EncodeString(string(data))
}

func conformancePageIDs(t *testing.T, endpoint string, query url.Values) []string {
	t.Helper()
	query.Set("limit", "1")
	ids := []string{}
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		response := conformanceRequest(t, http.MethodGet, endpoint+"?"+query.Encode(), nil, http.StatusOK)
		result, ok := response["result"].([]any)
		if !ok {
			t.Fatalf("missing array result: %#v", response)
		}
		for _, item := range result {
			ids = append(ids, item.(map[string]any)["id"].(string))
		}
		metadata, ok := response["paging_metadata"].(map[string]any)
		if !ok {
			t.Fatal("missing paging_metadata")
		}
		cursor, present := metadata["cursor"]
		if !present {
			sort.Strings(ids)
			return ids
		}
		token, ok := cursor.(string)
		if !ok || token == "" || seen[token] {
			t.Fatalf("invalid/repeated cursor: %#v", cursor)
		}
		if _, err := common.DecodeAPIString(token); err != nil {
			t.Fatal(err)
		}
		seen[token] = true
		query.Set("cursor", token)
	}
	t.Fatal("pagination did not terminate")
	return nil
}

// RunSubmodelReferenceConformance verifies exact matches and supplemental lookup before pagination.
func RunSubmodelReferenceConformance(t *testing.T, baseURL string) {
	t.Helper()
	prefix := fmt.Sprintf("urn:conformance:reference:%d", time.Now().UnixNano())
	reference := referenceFixture(prefix, "first", "last")
	nested := referenceFixture(prefix, "first", "last")
	nested["referredSemanticId"] = referenceFixture(prefix + ":nested")
	variants := []struct {
		name         string
		semantic     map[string]any
		supplemental []any
		match        bool
	}{
		{"primary-a", reference, nil, true}, {"primary-b", reference, nil, true},
		{"supplemental", referenceFixture(prefix + ":other"), []any{reference}, true},
		{"reordered", referenceFixture(prefix, "last", "first"), nil, false},
		{"extra", referenceFixture(prefix, "first", "last", "extra"), nil, false},
		{"partial", referenceFixture(prefix, "first"), nil, false},
		{"different", referenceFixture(prefix, "first", "different"), nil, false},
		{"nested", nested, nil, false},
		{"supplemental-nested", referenceFixture(prefix + ":other"), []any{nested}, false},
		{"model-reference", map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "ConceptDescription", "value": prefix}}}, nil, false},
	}
	wanted := []string{}
	for _, variant := range variants {
		id := prefix + ":" + variant.name
		body := map[string]any{"modelType": "Submodel", "id": id, "idShort": "Conformance", "semanticId": variant.semantic}
		if variant.supplemental != nil {
			body["supplementalSemanticIds"] = variant.supplemental
		}
		conformanceRequest(t, http.MethodPost, baseURL+"/submodels", body, http.StatusCreated)
		t.Cleanup(func() {
			conformanceRequest(t, http.MethodDelete, baseURL+"/submodels/"+common.EncodeString(id), nil, http.StatusNoContent)
		})
		if variant.match {
			wanted = append(wanted, id)
		}
	}
	got := conformancePageIDs(t, baseURL+"/submodels", url.Values{"semanticId": {encodedReferenceFixture(t, reference)}})
	sort.Strings(wanted)
	if !reflect.DeepEqual(got, wanted) {
		t.Fatalf("matches=%v wanted=%v", got, wanted)
	}
	nestedMatches := conformancePageIDs(t, baseURL+"/submodels", url.Values{"semanticId": {encodedReferenceFixture(t, nested)}})
	if !reflect.DeepEqual(nestedMatches, []string{prefix + ":nested", prefix + ":supplemental-nested"}) {
		t.Fatalf("nested matches=%v", nestedMatches)
	}
	requireQueryCursorRoundTrip(t, baseURL, len(variants))
}

// RunConceptDescriptionReferenceConformance checks both reference filters against complete objects.
func RunConceptDescriptionReferenceConformance(t *testing.T, baseURL string) {
	t.Helper()
	prefix := fmt.Sprintf("urn:conformance:cd:%d", time.Now().UnixNano())
	reference := referenceFixture(prefix, "first", "last")
	wanted := []string{}
	for index, ref := range []map[string]any{reference, reference, referenceFixture(prefix, "first", "other")} {
		id := fmt.Sprintf("%s:%d", prefix, index)
		body := map[string]any{"modelType": "ConceptDescription", "id": id, "isCaseOf": []any{ref}}
		conformanceRequest(t, http.MethodPost, baseURL+"/concept-descriptions", body, http.StatusCreated)
		t.Cleanup(func() {
			conformanceRequest(t, http.MethodDelete, baseURL+"/concept-descriptions/"+common.EncodeString(id), nil, http.StatusNoContent)
		})
		if index < 2 {
			wanted = append(wanted, id)
		}
	}
	got := conformancePageIDs(t, baseURL+"/concept-descriptions", url.Values{"isCaseOf": {encodedReferenceFixture(t, reference)}})
	if !reflect.DeepEqual(got, wanted) {
		t.Fatalf("matches=%v wanted=%v", got, wanted)
	}
}

// EncodeExternalReference encodes a one-key external reference for an API filter.
func EncodeExternalReference(value string) string {
	data, _ := json.Marshal(referenceFixture(value))
	return common.EncodeString(string(data))
}

// RunReferenceVisibilityConformance ensures API reference filters cannot observe a hidden primary reference.
func RunReferenceVisibilityConformance(t *testing.T, baseURL string, admin, reader http.Header, visibleSemantic string) {
	prefix := fmt.Sprintf("urn:conformance:visibility:%d", time.Now().UnixNano())
	for index, value := range []string{visibleSemantic, prefix} {
		id := fmt.Sprintf("%s:%d", prefix, index)
		body := map[string]any{"modelType": "Submodel", "id": id, "idShort": "ConformanceVisibility", "semanticId": referenceFixture(value)}
		conformanceRequest(t, http.MethodPost, baseURL+"/submodels", body, http.StatusCreated, admin)
		t.Cleanup(func() {
			conformanceRequest(t, http.MethodDelete, baseURL+"/submodels/"+common.EncodeString(id), nil, http.StatusNoContent, admin)
		})
	}
	for _, test := range []struct {
		name, value string
		headers     http.Header
		want        int
	}{
		{"visible", visibleSemantic, reader, 1}, {"hidden", prefix, reader, 0}, {"admin", prefix, admin, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := url.Values{"semanticId": {EncodeExternalReference(test.value)}, "idShort": {"ConformanceVisibility"}, "limit": {"1"}}
			response := conformanceRequest(t, http.MethodGet, baseURL+"/submodels?"+query.Encode(), nil, http.StatusOK, test.headers)
			if len(response["result"].([]any)) != test.want {
				t.Fatalf("unexpected visibility: %#v", response)
			}
		})
	}
}

// RunAssetPairConformance checks conjunction, pair correlation and continuation across pages.
func RunAssetPairConformance(t *testing.T, baseURL, path string, registry bool) {
	prefix := fmt.Sprintf("urn:conformance:assets:%d", time.Now().UnixNano())
	first := map[string]any{"name": "serial", "value": prefix}
	second := map[string]any{"name": "maker", "value": "manufacturer"}
	wanted := []string{}
	for index, assets := range [][]any{
		{first}, {first, second}, {second, first},
		{map[string]any{"name": "serial", "value": "manufacturer"}, map[string]any{"name": "maker", "value": prefix}},
	} {
		id := fmt.Sprintf("%s:%d", prefix, index)
		body := map[string]any{"id": id, "idShort": "ConformanceAssets"}
		if registry {
			body["assetKind"] = "Instance"
			body["specificAssetIds"] = assets
			body["endpoints"] = []any{map[string]any{"interface": "AAS-3.0", "protocolInformation": map[string]any{"href": "https://example.org/aas"}}}
		} else {
			body["modelType"] = "AssetAdministrationShell"
			body["assetInformation"] = map[string]any{"assetKind": "Instance", "specificAssetIds": assets}
		}
		conformanceRequest(t, http.MethodPost, baseURL+path, body, http.StatusCreated)
		t.Cleanup(func() {
			conformanceRequest(t, http.MethodDelete, baseURL+path+"/"+common.EncodeString(id), nil, http.StatusNoContent)
		})
		if index == 1 || index == 2 {
			wanted = append(wanted, id)
		}
	}
	encoded := []string{encodedReferenceFixture(t, first), encodedReferenceFixture(t, second)}
	for _, values := range [][]string{encoded, {encoded[1], encoded[0]}} {
		got := conformancePageIDs(t, baseURL+path, url.Values{"assetIds": values})
		if !reflect.DeepEqual(got, wanted) {
			t.Fatalf("asset matches=%v wanted=%v", got, wanted)
		}
	}
}

func requireQueryCursorRoundTrip(t *testing.T, baseURL string, want int) {
	t.Helper()
	body := map[string]any{"$condition": map[string]any{"$eq": []any{map[string]any{"$field": "$sm#idShort"}, map[string]any{"$strVal": "Conformance"}}}}
	query := url.Values{"limit": {"1"}}
	seen := map[string]bool{}
	for page := 0; page <= want; page++ {
		response := conformanceRequest(t, http.MethodPost, baseURL+"/query/submodels?"+query.Encode(), body, http.StatusOK)
		for _, item := range response["result"].([]any) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate query result %s", id)
			}
			seen[id] = true
		}
		token, present := response["paging_metadata"].(map[string]any)["cursor"]
		if !present {
			if len(seen) != want {
				t.Fatalf("query results=%d want=%d", len(seen), want)
			}
			return
		}
		if _, err := common.DecodeAPIString(token.(string)); err != nil {
			t.Fatal(err)
		}
		query.Set("cursor", token.(string))
	}
	t.Fatal("query pagination did not terminate")
}
