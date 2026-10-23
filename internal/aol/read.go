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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	return map[string]any{"source": path, "data": value, "untrusted": true}, nil
}

func (r Reader) pagePath(endpoint, cursor string) string {
	params := url.Values{"limit": {strconv.Itoa(r.PageSize)}}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	return endpoint + "?" + params.Encode()
}

// ReadPaged reads a collection representation and retains its continuation metadata.
func (r Reader) ReadPaged(ctx context.Context, endpoint, cursor string) (any, error) {
	return r.Read(ctx, r.pagePath(endpoint, cursor))
}

// QueryPage reads one collection page without silently discarding its cursor.
func (r Reader) QueryPage(ctx context.Context, endpoint, cursor string, query any) (Page, error) {
	method := http.MethodGet
	if query != nil {
		method = http.MethodPost
	}
	response, err := r.Dispatch.Do(ctx, method, r.pagePath(endpoint, cursor), query, nil)
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
	found := make([]map[string]any, 0)
	query = strings.ToLower(query)
	scanned := 0
	seen := map[string]bool{}
	for {
		page, err := r.QueryPage(ctx, "/shells", cursor, nil)
		if err != nil {
			return nil, err
		}
		scanned += len(page.Result)
		for _, shell := range page.Result {
			if matchesShell(shell, query) {
				found = append(found, map[string]any{"shell": shell, "source": "/shells/" + EncodeID(stringField(shell, "id")), "submodels": shell["submodels"]})
			}
		}
		cursor = page.Paging.Cursor
		if cursor == "" || len(found) >= r.PageSize || scanned >= r.MaxScan {
			return map[string]any{"result": found, "paging_metadata": page.Paging, "untrusted": true}, nil
		}
		if seen[cursor] {
			return nil, Failure("MCP-FIND-CURSOR", "API repeated a cursor", "Report this pagination error.")
		}
		seen[cursor] = true
	}
}
func matchesShell(shell map[string]any, query string) bool {
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
	response, err := r.Dispatch.Do(ctx, "GET", "/shells/"+EncodeID(id), nil, nil)
	if err != nil {
		return nil, err
	}
	var shell map[string]any
	if err = json.Unmarshal(response.Body, &shell); err != nil {
		return nil, err
	}
	refs, _ := shell["submodels"].([]any)
	submodels := []any{}
	omitted := []string{}
	concepts := map[string]any{}
	for _, ref := range refs {
		smID := referenceID(ref)
		if smID == "" {
			continue
		}
		summary, readErr := r.describeSubmodel(ctx, smID, concepts)
		if readErr != nil {
			omitted = append(omitted, "submodel content or metadata unavailable")
			continue
		}
		submodels = append(submodels, summary)
	}
	return map[string]any{"shell": shell, "source": "/shells/" + EncodeID(id), "submodels": submodels, "conceptDescriptions": concepts, "omissions": omitted, "partial": len(omitted) > 0, "untrusted": true}, nil
}
func (r Reader) describeSubmodel(ctx context.Context, id string, concepts map[string]any) (any, error) {
	path := "/submodels/" + EncodeID(id)
	response, err := r.Dispatch.Do(ctx, "GET", path, nil, nil)
	if err != nil {
		return r.Read(ctx, path+"/$value")
	}
	var submodel map[string]any
	if err = json.Unmarshal(response.Body, &submodel); err != nil {
		return nil, err
	}
	raw, _ := submodel["submodelElements"].([]any)
	values := []map[string]any{}
	flattenElements(raw, "", path, &values)
	for _, value := range values {
		semantic := referenceID(value["semanticId"])
		if semantic == "" {
			continue
		}
		if _, ok := concepts[semantic]; ok {
			continue
		}
		concept, readErr := r.Read(ctx, "/concept-descriptions/"+EncodeID(semantic))
		if readErr == nil {
			concepts[semantic] = concept
		}
	}
	return map[string]any{"id": id, "source": path, "values": values}, nil
}
func flattenElements(elements []any, prefix, base string, result *[]map[string]any) {
	for i, raw := range elements {
		element, _ := raw.(map[string]any)
		path := stringField(element, "idShort")
		if prefix != "" {
			if path == "" {
				path = fmt.Sprintf("%s[%d]", prefix, i)
			} else {
				path = prefix + "." + path
			}
		}
		model := stringField(element, "modelType")
		if model == "SubmodelElementCollection" || model == "SubmodelElementList" || model == "Entity" {
			children, _ := element[childField(model)].([]any)
			flattenElements(children, path, base, result)
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
