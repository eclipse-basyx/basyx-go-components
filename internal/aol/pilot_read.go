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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func referenceCount(shell map[string]any) int {
	refs, _ := shell["submodels"].([]any)
	return len(refs)
}
func shellSummary(shell map[string]any) map[string]any {
	summary := map[string]any{"summary": true}
	for _, key := range []string{"id", "idShort", "semanticId"} {
		if value, ok := shell[key]; ok {
			summary[key] = value
		}
	}
	if names, ok := shell["displayName"].([]any); ok {
		labels := []any{}
		for _, raw := range names[:min(len(names), 2)] {
			name, _ := raw.(map[string]any)
			label := stringField(name, "text")
			if len(label) > 160 {
				label = strings.ToValidUTF8(label[:160], "") + "…"
			}
			labels = append(labels, map[string]any{"language": name["language"], "text": label})
		}
		summary["displayName"] = labels
	}
	if info, ok := shell["assetInformation"].(map[string]any); ok {
		selected := map[string]any{}
		for _, key := range []string{"assetKind", "globalAssetId"} {
			if value, exists := info[key]; exists {
				selected[key] = value
			}
		}
		summary["assetInformation"] = selected
	}
	return summary
}
func previewValue(value any) any {
	raw, _ := json.Marshal(value)
	if len(raw) > 512 {
		return map[string]any{"preview": strings.ToValidUTF8(string(raw[:512]), ""), "truncated": true, "hint": "Use get_element for the complete value."}
	}
	return value
}
func fitsResponse(value any, budget int) bool {
	raw, _ := json.Marshal(value)
	text, _ := json.Marshal(string(raw))
	return len(raw)+len(text)+512 <= budget
}

// ResolveShell accepts a canonical identifier or an unambiguous readable asset alias.
func (r Reader) ResolveShell(ctx context.Context, input string) (map[string]any, error) {
	value, err := r.Read(ctx, "/shells/"+EncodeID(input))
	if err == nil {
		shell, _ := value.(map[string]any)["data"].(map[string]any)
		return shell, nil
	}
	if !isNotFound(err) {
		return nil, err
	}
	matches, err := r.matchingShells(ctx, "/shells?idShort="+url.QueryEscape(input), input)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		matches, err = r.matchingShells(ctx, "/shells", input)
		if err != nil {
			return nil, err
		}
	}

	if len(matches) == 0 {
		return nil, Failure("MCP-ASSET-NOTFOUND", "no readable asset matched", "Check the asset name or ID with find_asset.")
	}
	if len(matches) != 1 {
		return nil, Failure("MCP-ASSET-AMBIGUOUS", "more than one readable asset matched", "Call find_asset and select one returned shell ID.")
	}

	shell := matches[0]
	value, err = r.Read(ctx, "/shells/"+EncodeID(stringField(shell, "id")))
	if err != nil {
		return nil, err
	}
	shell, _ = value.(map[string]any)["data"].(map[string]any)
	return shell, nil
}

func (r Reader) matchingShells(ctx context.Context, endpoint, input string) ([]map[string]any, error) {
	candidates, err := r.Scan(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}
	matches := []map[string]any{}
	for _, candidate := range candidates {
		if matchesShell(candidate, input) {
			matches = append(matches, candidate)
		}
	}
	return matches, nil
}

// DescribePage returns bounded previews and explicit continuation links for large assets.
func (r Reader) DescribePage(ctx context.Context, input, cursor string) (any, error) {
	shell, err := r.ResolveShell(ctx, input)
	if err != nil {
		return nil, err
	}
	id := stringField(shell, "id")
	path := "/shells/" + EncodeID(id)
	pageReader := r
	pageReader.PageSize = min(r.PageSize, 3)
	refs, err := pageReader.QueryPage(ctx, path+"/submodel-refs", cursor, nil)
	if err != nil {
		return nil, err
	}
	models := []any{}
	omissions := []string{}
	for _, ref := range refs.Result {
		smID := referenceID(ref)
		if smID == "" {
			continue
		}
		summary, readErr := pageReader.describeSubmodel(ctx, smID)
		if readErr != nil {
			omissions = append(omissions, "submodel content or metadata unavailable; use list_submodels_of_shell to select details")
			continue
		}
		models = append(models, summary)
	}
	return map[string]any{"shell": shellSummary(shell), "source": path, "submodels": models, "paging_metadata": refs.Paging, "partial": len(omissions) > 0 || refs.Paging.Cursor != "", "omissions": omissions, "untrusted": true, "retrieved_at": time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

type elementMatch struct {
	Submodel   string `json:"submodel"`
	Path       string `json:"path"`
	ModelType  string `json:"modelType"`
	Name       string `json:"name"`
	SemanticID any    `json:"semanticId,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Source     string `json:"source"`
}
type elementCursor struct {
	Offset int    `json:"offset"`
	Scope  string `json:"scope"`
}

func cursorScope(shell, query, semantic string) string {
	sum := sha256.Sum256([]byte(shell + "\x00" + query + "\x00" + semantic))
	return hex.EncodeToString(sum[:8])
}
func readElementCursor(raw, scope string, maximum int) (int, error) {
	if raw == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	var cursor elementCursor
	if err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.Scope != scope || cursor.Offset < 0 || cursor.Offset > maximum {
		return 0, Failure("MCP-ELEMENTS-CURSOR", "invalid search cursor", "Use a cursor returned for the same asset and search.")
	}
	return cursor.Offset, nil
}
func nextElementCursor(offset int, scope string) string {
	raw, _ := json.Marshal(elementCursor{Offset: offset, Scope: scope})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// FindElements discovers authorized element metadata without returning unrequested values.
func (r Reader) FindElements(ctx context.Context, input, query, semantic, cursor string) (any, error) {
	if strings.TrimSpace(query) == "" && semantic == "" {
		return nil, Failure("MCP-MEASUREMENT-QUERY", "an element name or semanticId is required", "Provide a name such as Temperature or an exact semanticId.")
	}
	scope := cursorScope(input, query, semantic)
	offset, err := readElementCursor(cursor, scope, r.MaxScan)
	if err != nil {
		return nil, err
	}
	matches, err := r.matchingElements(ctx, input, query, semantic)
	if err != nil {
		return nil, err
	}
	if offset > len(matches) {
		return nil, Failure("MCP-ELEMENTS-CURSOR", "search results changed", "Restart the search without a cursor.")
	}
	end := min(len(matches), offset+min(r.PageSize, 20))
	for {
		next := ""
		if end < len(matches) {
			next = nextElementCursor(end, scope)
		}
		result := map[string]any{"result": matches[offset:end], "paging_metadata": map[string]any{"cursor": next}, "complete": next == "", "untrusted": true}
		if fitsResponse(result, r.Dispatch.MaxBytes) {
			return result, nil
		}
		if end-offset <= 1 {
			return nil, Failure("MCP-ELEMENTS-BYTELIMIT", "one element summary exceeds the response budget", "Ask the administrator to raise maxResultBytes.")
		}
		end = offset + (end-offset)/2
	}
}

func (r Reader) matchingElements(ctx context.Context, input, query, semantic string) ([]elementMatch, error) {
	shell, err := r.ResolveShell(ctx, input)
	if err != nil {
		return nil, err
	}
	refs, err := r.Scan(ctx, "/shells/"+EncodeID(stringField(shell, "id"))+"/submodel-refs", nil)
	if err != nil {
		return nil, err
	}
	matches := []elementMatch{}
	scanned := 0
	for _, ref := range refs {
		id := referenceID(ref)
		if id == "" {
			continue
		}
		elements, readErr := r.Scan(ctx, "/submodels/"+EncodeID(id)+"/submodel-elements/$metadata", nil)
		if readErr != nil {
			if deniedRead(readErr) {
				continue
			}
			return nil, readErr
		}
		for _, element := range elements {
			if err = collectMatches(element, "", id, query, semantic, &matches, &scanned, r.MaxScan); err != nil {
				return nil, err
			}
		}
	}
	return matches, nil
}
func deniedRead(err error) bool {
	failure, ok := err.(*Error)
	return ok && (failure.Status == 403 || failure.Status == 404)
}
func collectMatches(element map[string]any, path, id, query, semantic string, result *[]elementMatch, scanned *int, maximum int) error {
	*scanned++
	if *scanned > maximum {
		return Failure("MCP-ELEMENTS-SCANLIMIT", "element scan limit exceeded", "Select a narrower submodel with get_submodel_value or use a concrete element path.")
	}
	name := stringField(element, "idShort")
	if path == "" {
		path = name
	}
	model := stringField(element, "modelType")
	if elementMatches(element, query, semantic) && model != "SubmodelElementCollection" && model != "SubmodelElementList" && model != "Entity" {
		*result = append(*result, elementMatch{Submodel: id, Path: path, Name: name, ModelType: model, SemanticID: element["semanticId"], Unit: embeddedUnit(element), Source: "/submodels/" + EncodeID(id) + "/submodel-elements/" + url.PathEscape(path) + "/$value"})
	}
	return collectChildMatches(element, path, id, query, semantic, result, scanned, maximum)
}
func collectChildMatches(element map[string]any, path, id, query, semantic string, result *[]elementMatch, scanned *int, maximum int) error {
	model := stringField(element, "modelType")
	if model != "SubmodelElementCollection" && model != "SubmodelElementList" && model != "Entity" && model != "AnnotatedRelationshipElement" {
		return nil
	}
	children, _ := element[childField(model)].([]any)
	for i, raw := range children {
		child, _ := raw.(map[string]any)
		childPath := path + "." + stringField(child, "idShort")
		if model == "SubmodelElementList" {
			childPath = fmt.Sprintf("%s[%d]", path, i)
		}
		if err := collectMatches(child, childPath, id, query, semantic, result, scanned, maximum); err != nil {
			return err
		}
	}
	return nil
}
func elementMatches(element map[string]any, query, semantic string) bool {
	if semantic != "" && referenceID(element["semanticId"]) != semantic {
		return false
	}
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	for _, field := range []string{"idShort", "displayName", "description"} {
		raw, _ := json.Marshal(element[field])
		if strings.Contains(strings.ToLower(string(raw)), query) {
			return true
		}
	}
	return false
}

// Measurement reads only an unambiguous scalar, preserving provenance and unknown sample age.
func (r Reader) Measurement(ctx context.Context, shell, query, semantic string) (any, error) {
	if strings.TrimSpace(query) == "" && semantic == "" {
		return nil, Failure("MCP-MEASUREMENT-QUERY", "an element name or semanticId is required", "Provide Temperature or an exact semanticId.")
	}
	matches, err := r.matchingElements(ctx, shell, query, semantic)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return map[string]any{"status": "not_found", "candidates": matches, "untrusted": true}, nil
	}
	if len(matches) > 1 {
		return map[string]any{"status": "ambiguous", "total_candidates": len(matches), "candidates": matches[:min(len(matches), 20)], "hint": "Select one candidate with get_element, or narrow the name/semanticId.", "untrusted": true}, nil
	}
	match := matches[0]
	if match.ModelType != "Property" && match.ModelType != "Range" && match.ModelType != "MultiLanguageProperty" {
		return nil, Failure("MCP-MEASUREMENT-TYPE", "matched element is not a measurement value", "Use get_element for the returned path.")
	}
	value, err := r.Element(ctx, match.Submodel, match.Path)
	if err != nil {
		return nil, err
	}
	result := value.(map[string]any)
	result["status"] = "found"
	result["submodel"] = match.Submodel
	result["path"] = match.Path
	result["measurement_time"] = nil
	result["freshness"] = "sample time unknown; retrieved_at records the repository read, not the sensor observation"
	return result, nil
}
