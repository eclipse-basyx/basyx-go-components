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

package aol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// Reader exposes bounded, source-labelled API compositions.
type Reader struct {
	Dispatch          Dispatcher
	PageSize, MaxScan int
}

// Page is a REST-compatible bounded collection.
type Page struct {
	Result []map[string]any `json:"result"`
	Paging struct {
		Cursor string `json:"cursor,omitempty"`
	} `json:"paging_metadata"`
}

// Read reads one server-selected API route.
func (r Reader) Read(ctx context.Context, path string) (any, error) {
	response, err := r.Dispatch.Do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var value any
	if err = json.Unmarshal(response.Body, &value); err != nil {
		return nil, Failure("MCP-READ-DECODE", "invalid API JSON", "Report the API response format to the administrator.")
	}
	result := map[string]any{"source": path, "data": value, "untrusted": true, "retrieved_at": time.Now().UTC().Format(time.RFC3339Nano)}
	if tag := response.Header.Get("ETag"); tag != "" {
		result["etag"] = tag
	}
	return result, nil
}

func (r Reader) pagePath(endpoint, cursor string) string {
	endpoint, existing, _ := strings.Cut(endpoint, "?")
	params, _ := url.ParseQuery(existing)
	params.Set("limit", strconv.Itoa(r.PageSize))
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	return endpoint + "?" + params.Encode()
}

// ReadPaged reads a collection representation and retains its continuation metadata.
func (r Reader) ReadPaged(ctx context.Context, endpoint, cursor string) (any, error) {
	return r.collectionValue(ctx, http.MethodGet, endpoint, cursor, nil, true)
}

// QueryPage reads one collection page without silently discarding its cursor.
func (r Reader) QueryPage(ctx context.Context, endpoint, cursor string, query any) (Page, error) {
	method := http.MethodGet
	if query != nil {
		method = http.MethodPost
	}
	response, err := r.readPageResponse(ctx, method, endpoint, cursor, query)
	if err != nil {
		return Page{}, err
	}

	var page Page
	err = json.Unmarshal(response.Body, &page)
	if err != nil {
		return Page{}, Failure("MCP-READ-DECODE", "invalid collection response", "Check the API version.")
	}
	return page, nil
}

func (r Reader) readPageResponse(ctx context.Context, method, endpoint, cursor string, query any) (Response, error) {
	for size := r.PageSize; size >= 1; size = max(1, size/2) {
		pageReader := r
		pageReader.PageSize = size
		response, err := r.Dispatch.Do(ctx, method, pageReader.pagePath(endpoint, cursor), query, nil)
		var failure *Error
		if err == nil || !errors.As(err, &failure) || failure.Code != "MCP-DISPATCH-BYTELIMIT" || size == 1 {
			return response, err
		}
	}
	return Response{}, Failure("MCP-READ-PAGE", "invalid page size", "Use a positive page size.")
}

// Scan consumes authorized pages up to a hard limit, never returning partial aggregates.
func (r Reader) Scan(ctx context.Context, endpoint string, query any) ([]map[string]any, error) {
	var all []map[string]any
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := r.QueryPage(ctx, endpoint, cursor, query)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Result...)
		if len(all) > r.MaxScan {
			return nil, Failure("MCP-READ-SCANLIMIT", "scan limit exceeded", "Narrow the AASQL condition.")
		}
		cursor = page.Paging.Cursor
		if cursor == "" {
			return all, nil
		}
		if seen[cursor] {
			return nil, Failure("MCP-READ-CURSOR", "API repeated a cursor", "Report this pagination error.")
		}
		seen[cursor] = true
	}
}

// FindAsset searches the visible shell representation; predicates cannot inspect hidden fields.
func (r Reader) FindAsset(ctx context.Context, query, cursor string) (any, error) {
	query = strings.ToLower(query)
	scanned := 0
	seen := map[string]bool{}
	for {
		page, result, err := r.findAssetPage(ctx, query, cursor)
		if err != nil {
			return nil, err
		}
		scanned += len(page.Result)
		cursor = page.Paging.Cursor
		if cursor == "" || len(result["result"].([]map[string]any)) > 0 || scanned >= r.MaxScan {
			return result, nil
		}
		if seen[cursor] {
			return nil, Failure("MCP-FIND-CURSOR", "API repeated a cursor", "Report this pagination error.")
		}
		seen[cursor] = true
	}
}
func (r Reader) findAssetPage(ctx context.Context, query, cursor string) (Page, map[string]any, error) {
	for {
		page, err := r.QueryPage(ctx, "/shells", cursor, nil)
		if err != nil {
			return Page{}, nil, err
		}
		found := make([]map[string]any, 0)
		for _, shell := range page.Result {
			if matchesShell(shell, query) {
				found = append(found, map[string]any{"shell": shellSummary(shell), "source": "/shells/" + EncodeID(stringField(shell, "id")), "submodel_count": referenceCount(shell)})
			}
		}
		result := map[string]any{"result": found, "paging_metadata": page.Paging, "untrusted": true}
		if fitsResponse(result, r.Dispatch.MaxBytes) {
			return page, result, nil
		}
		if len(page.Result) <= 1 {
			return Page{}, nil, Failure("MCP-FIND-BYTELIMIT", "one asset summary exceeds the response budget", "Ask the administrator to raise maxResultBytes.")
		}
		r.PageSize = max(1, min(r.PageSize/2, len(page.Result)/2))
	}
}

func matchesShell(shell map[string]any, query string) bool {
	query = strings.ToLower(query)
	for _, key := range []string{"id", "idShort"} {
		if strings.Contains(strings.ToLower(stringField(shell, key)), query) {
			return true
		}
	}
	for _, key := range []string{"displayName", "assetInformation"} {
		raw, _ := json.Marshal(shell[key])
		if strings.Contains(strings.ToLower(string(raw)), query) {
			return true
		}
	}
	return false
}
func stringField(value map[string]any, key string) string { s, _ := value[key].(string); return s }

// Element returns value-only data plus separately authorized unit metadata.
func (r Reader) Element(ctx context.Context, id, path string) (any, error) {
	var field grammar.ModelStringPattern
	encoded, _ := json.Marshal("$sme." + path + "#value")
	if err := json.Unmarshal(encoded, &field); err != nil || strings.Contains(path, "[]") {
		return nil, Failure("MCP-ELEMENT-PATH", "invalid concrete idShortPath", "Use dot-separated element names and numeric list indices.")
	}
	base := "/submodels/" + EncodeID(id) + "/submodel-elements/" + url.PathEscape(path)
	value, err := r.Read(ctx, base+"/$value")
	if err != nil {
		return nil, err
	}
	result := value.(map[string]any)
	metadata, err := r.Dispatch.Do(ctx, "GET", base+"/$metadata", nil, nil)
	if err == nil {
		var element map[string]any
		if json.Unmarshal(metadata.Body, &element) == nil {
			result["semanticId"] = element["semanticId"]
			result["unit"] = embeddedUnit(element)
		}
	}
	return result, nil
}
func embeddedUnit(element map[string]any) string {
	specs, _ := element["embeddedDataSpecifications"].([]any)
	for _, spec := range specs {
		m, _ := spec.(map[string]any)
		content, _ := m["dataSpecificationContent"].(map[string]any)
		if unit := stringField(content, "unit"); unit != "" {
			return unit
		}
	}
	return ""
}

// Describe composes authorized shell, submodel values and local concept descriptions.
func (r Reader) Describe(ctx context.Context, id string) (any, error) {
	return r.DescribePage(ctx, id, "")
}
func (r Reader) describeSubmodel(ctx context.Context, id string) (any, error) {
	path := "/submodels/" + EncodeID(id)
	metadata, err := r.Read(ctx, path+"/$metadata")
	if err != nil {
		return nil, err
	}
	pageReader := r
	pageReader.PageSize = min(r.PageSize, 5)
	page, err := pageReader.QueryPage(ctx, path+"/submodel-elements/$metadata", "", nil)
	if err != nil {
		return nil, err
	}
	raw := make([]any, 0, len(page.Result))
	for _, element := range page.Result {
		raw = append(raw, element)
	}
	values := []map[string]any{}
	flattenElements(raw, "", path, &values)
	values = values[:min(len(values), max(1, min(5, r.Dispatch.MaxBytes/16384)))]
	concepts := r.describeValues(ctx, values)
	return map[string]any{"values_preview": true, "paths_tool": "find_elements", "concept_descriptions": concepts, "id": id, "source": path, "metadata": shellSummary(metadata.(map[string]any)["data"].(map[string]any)), "values": values, "paging_metadata": page.Paging, "details_tool": "get_submodel_value"}, nil
}

func (r Reader) describeValues(ctx context.Context, values []map[string]any) []any {
	concepts := []any{}
	seen := map[string]bool{}
	for _, value := range values {
		read, err := r.Read(ctx, stringField(value, "source"))
		if err != nil {
			value["unavailable"] = true
		} else {
			value["value"] = previewValue(read.(map[string]any)["data"])
		}
		id := referenceID(value["semanticId"])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		concept, err := r.Read(ctx, "/concept-descriptions/"+EncodeID(id))
		if err == nil {
			data, _ := concept.(map[string]any)["data"].(map[string]any)
			concepts = append(concepts, map[string]any{"metadata": shellSummary(data), "unit": embeddedUnit(data), "source": "/concept-descriptions/" + EncodeID(id)})
		}
	}
	return concepts
}

func flattenElements(elements []any, prefix, base string, result *[]map[string]any) {
	flattenElementChildren(elements, prefix, base, false, result)
}
func elementPath(prefix, name string, index int, list bool) string {
	if list {
		return fmt.Sprintf("%s[%d]", prefix, index)
	}
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
func flattenElementChildren(elements []any, prefix, base string, list bool, result *[]map[string]any) {
	for i, raw := range elements {
		element, _ := raw.(map[string]any)
		path := elementPath(prefix, stringField(element, "idShort"), i, list)
		model := stringField(element, "modelType")
		if model == "SubmodelElementCollection" || model == "SubmodelElementList" || model == "Entity" {
			children, _ := element[childField(model)].([]any)
			flattenElementChildren(children, path, base, model == "SubmodelElementList", result)
			continue
		}
		*result = append(*result, map[string]any{"path": path, "source": base + "/submodel-elements/" + url.PathEscape(path) + "/$value", "value": element["value"], "valueType": element["valueType"], "semanticId": element["semanticId"], "unit": embeddedUnit(element)})
	}
}

func referenceID(raw any) string {
	m, _ := raw.(map[string]any)
	keys, _ := m["keys"].([]any)
	if len(keys) == 0 {
		return ""
	}
	key, _ := keys[len(keys)-1].(map[string]any)
	return stringField(key, "value")
}
func resourcePath(kind, id string) string { return fmt.Sprintf("/%s/%s", kind, EncodeID(id)) }

// QueryValue preserves both object and identifier-only projections from AASQL.
func (r Reader) QueryValue(ctx context.Context, endpoint, cursor string, query any) (any, error) {
	return r.collectionValue(ctx, http.MethodPost, endpoint, cursor, query, false)
}
func (r Reader) collectionValue(ctx context.Context, method, endpoint, cursor string, query any, wrapped bool) (any, error) {
	for {
		response, err := r.readPageResponse(ctx, method, endpoint, cursor, query)
		if err != nil {
			return nil, err
		}
		var page map[string]any
		if json.Unmarshal(response.Body, &page) != nil {
			return nil, Failure("MCP-READ-DECODE", "invalid collection response", "Check the API version.")
		}
		var result map[string]any
		if wrapped {
			result = map[string]any{"source": r.pagePath(endpoint, cursor), "data": page}
		} else {
			result = page
		}
		result["untrusted"] = true
		result["retrieved_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		if fitsResponse(result, r.Dispatch.MaxBytes) {
			return result, nil
		}
		items, _ := page["result"].([]any)
		if len(items) <= 1 {
			return nil, Failure("MCP-READ-BYTELIMIT", "one collection item exceeds the response budget", "Use get_element for a concrete path or ask the administrator to raise maxResultBytes.")
		}
		r.PageSize = max(1, min(r.PageSize/2, len(items)/2))
	}
}
