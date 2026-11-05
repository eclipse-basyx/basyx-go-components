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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const queryLanguageHint = `The query is an AAS Query Language JSON object, for example ` +
	`{"$condition":{"$eq":[{"$field":"$sm#idShort"},{"$strVal":"TechnicalData"}]}}. ` +
	`Operators include $eq, $ne, $gt, $ge, $lt, $le, $contains, $starts-with, $ends-with, $regex, $and, $or and $not. ` +
	`Fields use the form $aas#idShort, $aas#assetInformation.globalAssetId, $sm#semanticId or $sme.<idShortPath>#value.`

type pageInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of results in this page"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Cursor from paging_metadata of the previous page"`
}

type listShellsInput struct {
	IDShort string `json:"idShort,omitempty" jsonschema:"Only return shells with exactly this idShort"`
	pageInput
}

type shellInput struct {
	ShellID string `json:"shellId" jsonschema:"Identifier (id) of the Asset Administration Shell, not encoded"`
}

type listSubmodelsInput struct {
	IDShort    string `json:"idShort,omitempty" jsonschema:"Only return submodels with exactly this idShort"`
	SemanticID string `json:"semanticId,omitempty" jsonschema:"Only return submodels whose semanticId is an external reference with this global identifier"`
	pageInput
}

type submodelInput struct {
	SubmodelID string `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	ValueOnly  bool   `json:"valueOnly,omitempty" jsonschema:"Return the compact value-only representation"`
}

type elementInput struct {
	SubmodelID  string `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	IDShortPath string `json:"idShortPath" jsonschema:"Dot-separated idShort path, list items use [index], for example Nameplate.Address or Measurements[0]"`
	ValueOnly   bool   `json:"valueOnly,omitempty" jsonschema:"Return the compact value-only representation"`
}

type conceptDescriptionInput struct {
	ConceptDescriptionID string `json:"conceptDescriptionId" jsonschema:"Identifier (id) of the concept description, usually a semanticId value"`
}

type queryInput struct {
	Query map[string]any `json:"query" jsonschema:"AAS Query Language JSON object with a $condition"`
	pageInput
}

func readOnlyAnnotations() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
}

func boolPtr(value bool) *bool { return &value }

func (m *Module) addReadTools(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_shells",
		Description: "List Asset Administration Shells. Returns a page of shells and paging_metadata.cursor for the next page.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in listShellsInput) (*sdk.CallToolResult, any, error) {
		query := in.values()
		setIfPresent(query, "idShort", in.IDShort)
		return m.call(ctx, req, restCall{method: http.MethodGet, path: "/shells", query: query})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_shell",
		Description: "Read one Asset Administration Shell including its asset information and submodel references.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in shellInput) (*sdk.CallToolResult, any, error) {
		return m.call(ctx, req, restCall{method: http.MethodGet, path: shellPath(in.ShellID)})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_submodels",
		Description: "List submodels, optionally filtered by idShort or semanticId. Returns a page and paging_metadata.cursor for the next page.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in listSubmodelsInput) (*sdk.CallToolResult, any, error) {
		query := in.values()
		setIfPresent(query, "idShort", in.IDShort)
		if in.SemanticID != "" {
			reference, err := globalReferenceQueryValue(in.SemanticID)
			if err != nil {
				return nil, nil, err
			}
			query.Set("semanticId", reference)
		}
		return m.call(ctx, req, restCall{method: http.MethodGet, path: "/submodels", query: query})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_submodel",
		Description: "Read one submodel with all its elements. Use valueOnly for a compact form, or get_submodel_element for a single element of a large submodel. The result carries an etag for conditional writes.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in submodelInput) (*sdk.CallToolResult, any, error) {
		return m.call(ctx, req, restCall{method: http.MethodGet, path: withValueOnly(submodelPath(in.SubmodelID), in.ValueOnly)})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_submodel_element",
		Description: "Read one submodel element by its idShortPath. The result carries an etag for conditional writes.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in elementInput) (*sdk.CallToolResult, any, error) {
		return m.call(ctx, req, restCall{method: http.MethodGet, path: withValueOnly(elementPath(in.SubmodelID, in.IDShortPath), in.ValueOnly)})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_concept_description",
		Description: "Read a concept description, for example to resolve the meaning, unit or definition behind a semanticId.",
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in conceptDescriptionInput) (*sdk.CallToolResult, any, error) {
		return m.call(ctx, req, restCall{method: http.MethodGet, path: "/concept-descriptions/" + common.EncodeString(in.ConceptDescriptionID)})
	})
	m.addQueryTool(server, "query_shells", "Search Asset Administration Shells with an AAS Query Language condition. ", "/query/shells")
	m.addQueryTool(server, "query_submodels", "Search submodels with an AAS Query Language condition. ", "/query/submodels")
}

func (m *Module) addQueryTool(server *sdk.Server, name, description, path string) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        name,
		Description: description + queryLanguageHint,
		Annotations: readOnlyAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in queryInput) (*sdk.CallToolResult, any, error) {
		return m.call(ctx, req, restCall{method: http.MethodPost, path: path, query: in.values(), body: in.Query})
	})
}

func (p pageInput) values() url.Values {
	query := url.Values{}
	if p.Limit > 0 {
		query.Set("limit", strconv.Itoa(p.Limit))
	}
	setIfPresent(query, "cursor", p.Cursor)
	return query
}

func setIfPresent(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

// globalReferenceQueryValue encodes a plain semantic identifier as the serialized external reference the API expects.
func globalReferenceQueryValue(semanticID string) (string, error) {
	reference, err := json.Marshal(map[string]any{
		"type": "ExternalReference",
		"keys": []map[string]string{{"type": "GlobalReference", "value": semanticID}},
	})
	if err != nil {
		return "", fmt.Errorf("MCP-LISTSUBMODELS-ENCODESEMANTICID: %w", err)
	}
	return common.Encode(reference), nil
}

func withValueOnly(path string, valueOnly bool) string {
	if valueOnly {
		return path + "/$value"
	}
	return path
}

// call forwards the caller's credentials to the API and returns its result as tool output.
func (m *Module) call(ctx context.Context, req *sdk.CallToolRequest, call restCall) (*sdk.CallToolResult, any, error) {
	call.authorization = authorizationHeader(req)
	ctx, cancel := linkRequestCancellation(ctx)
	defer cancel()
	result, err := m.dispatch.do(ctx, call)
	if err != nil {
		return nil, nil, err
	}
	return nil, toolOutput(result), nil
}

func authorizationHeader(req *sdk.CallToolRequest) string {
	if req == nil || req.Extra == nil || req.Extra.Header == nil {
		return ""
	}
	return req.Extra.Header.Get("Authorization")
}

func toolOutput(result restResult) map[string]any {
	output := map[string]any{"status": result.status}
	if len(result.body) > 0 {
		output["data"] = result.body
	}
	if result.etag != "" {
		output["etag"] = result.etag
	}
	if result.location != "" {
		output["location"] = result.location
	}
	if result.bodyOmitted {
		output["bodyOmitted"] = true
		output["note"] = "The operation succeeded; its response exceeds maxResultBytes and is omitted. Read the resource in smaller parts if needed."
	}
	return output
}
