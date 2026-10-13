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
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const querySelectConformanceObjects = 3

// QuerySelectConformance describes a query endpoint whose objects are created
// below CollectionPath and matched by their idShort through IDShortField.
type QuerySelectConformance struct {
	BaseURL        string
	CollectionPath string
	QueryPath      string
	IDShortField   string
	NewObject      func(id string, idShort string) map[string]any
}

// RunQuerySelectConformance verifies that "$select": "id" returns the
// identifiers of the matching objects on every page, that queries without
// $select still return objects, and that other $select values are rejected.
func RunQuerySelectConformance(t *testing.T, spec QuerySelectConformance) {
	t.Helper()
	runID := time.Now().UnixNano()
	idShort := fmt.Sprintf("QuerySelect%d", runID)
	ids := make([]string, 0, querySelectConformanceObjects)
	for index := 0; index < querySelectConformanceObjects; index++ {
		id := fmt.Sprintf("urn:conformance:select:%d:%d", runID, index)
		conformanceRequest(t, http.MethodPost, spec.BaseURL+spec.CollectionPath, spec.NewObject(id, idShort), http.StatusCreated)
		t.Cleanup(func() {
			conformanceRequest(t, http.MethodDelete, spec.BaseURL+spec.CollectionPath+"/"+common.EncodeString(id), nil, http.StatusNoContent)
		})
		ids = append(ids, id)
	}
	condition := map[string]any{"$eq": []any{
		map[string]any{"$field": spec.IDShortField},
		map[string]any{"$strVal": idShort},
	}}

	for _, limit := range []int{1, 2, querySelectConformanceObjects + 1} {
		t.Run(fmt.Sprintf("identifiers_limit_%d", limit), func(t *testing.T) {
			query := map[string]any{"$select": "id", "$condition": condition}
			got := collectQuerySelectPages(t, spec, query, limit, requireIdentifierQueryPage)
			if !reflect.DeepEqual(got, ids) {
				t.Fatalf("identifiers=%v wanted=%v", got, ids)
			}
		})
	}
	t.Run("objects_without_select", func(t *testing.T) {
		query := map[string]any{"$condition": condition}
		got := collectQuerySelectPages(t, spec, query, querySelectConformanceObjects+1, requireObjectQueryPage)
		if !reflect.DeepEqual(got, ids) {
			t.Fatalf("objects=%v wanted=%v", got, ids)
		}
	})
	for _, invalid := range []any{[]any{"$aas#id"}, "idShort", ""} {
		t.Run(fmt.Sprintf("rejects_select_%v", invalid), func(t *testing.T) {
			query := map[string]any{"$select": invalid, "$condition": condition}
			conformanceRequest(t, http.MethodPost, spec.BaseURL+spec.QueryPath, query, http.StatusBadRequest)
		})
	}
}

func collectQuerySelectPages(
	t *testing.T,
	spec QuerySelectConformance,
	query map[string]any,
	limit int,
	pageIDs func(*testing.T, map[string]any) []string,
) []string {
	t.Helper()
	parameters := url.Values{"limit": {fmt.Sprint(limit)}}
	collected := []string{}
	for pageCount := 0; pageCount < 1000; pageCount++ {
		page := conformanceRequest(t, http.MethodPost, spec.BaseURL+spec.QueryPath+"?"+parameters.Encode(), query, http.StatusOK)
		collected = append(collected, pageIDs(t, page)...)
		metadata, _ := page["paging_metadata"].(map[string]any)
		cursor, _ := metadata["cursor"].(string)
		if cursor == "" {
			return collected
		}
		parameters.Set("cursor", cursor)
	}
	t.Fatal("query pagination did not terminate")
	return nil
}

func requireIdentifierQueryPage(t *testing.T, page map[string]any) []string {
	t.Helper()
	metadata, _ := page["paging_metadata"].(map[string]any)
	if metadata["resultType"] != "Identifier" {
		t.Fatalf("paging_metadata=%v wanted resultType Identifier", metadata)
	}
	result, ok := page["result"].([]any)
	if !ok {
		t.Fatalf("result=%v wanted array", page["result"])
	}
	ids := make([]string, 0, len(result))
	for _, item := range result {
		id, isString := item.(string)
		if !isString {
			t.Fatalf("result item=%v wanted identifier string", item)
		}
		ids = append(ids, id)
	}
	return ids
}

func requireObjectQueryPage(t *testing.T, page map[string]any) []string {
	t.Helper()
	metadata, _ := page["paging_metadata"].(map[string]any)
	if _, found := metadata["resultType"]; found {
		t.Fatalf("paging_metadata=%v wanted no resultType", metadata)
	}
	result, _ := page["result"].([]any)
	ids := make([]string, 0, len(result))
	for _, item := range result {
		object, _ := item.(map[string]any)
		id, isString := object["id"].(string)
		if !isString {
			t.Fatalf("result item=%v wanted object with id", item)
		}
		ids = append(ids, id)
	}
	return ids
}
