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
	"errors"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type createShellInput struct {
	Shell map[string]any `json:"shell" jsonschema:"Asset Administration Shell in AAS JSON serialization with id, idShort and assetInformation"`
}

type createSubmodelInput struct {
	Submodel  map[string]any `json:"submodel" jsonschema:"Submodel in AAS JSON serialization with id, idShort and submodelElements"`
	ShellID   string         `json:"shellId,omitempty" jsonschema:"Identifier of an existing shell that should reference the new submodel"`
	ShellETag string         `json:"shellEtag,omitempty" jsonschema:"ETag of the shell from get_shell; adding the reference fails if the shell changed since"`
}

type addElementInput struct {
	SubmodelID string         `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	ParentPath string         `json:"parentPath,omitempty" jsonschema:"idShortPath of a SubmodelElementCollection or SubmodelElementList to add the element to; empty adds it at submodel level"`
	Element    map[string]any `json:"element" jsonschema:"Submodel element in AAS JSON serialization including modelType and idShort"`
	ETag       string         `json:"etag,omitempty" jsonschema:"ETag of the submodel or parent element from a previous read; the write fails if it changed since"`
}

type setValueInput struct {
	SubmodelID  string `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	IDShortPath string `json:"idShortPath" jsonschema:"idShortPath of the element to update"`
	Value       any    `json:"value" jsonschema:"New value in value-only representation, for example \"42\" for a Property or an object for a collection"`
	ETag        string `json:"etag,omitempty" jsonschema:"ETag returned by a previous read; the write fails if the resource changed since"`
}

type deleteShellInput struct {
	ShellID string `json:"shellId" jsonschema:"Identifier (id) of the Asset Administration Shell, not encoded"`
	ETag    string `json:"etag,omitempty" jsonschema:"ETag returned by a previous read; the write fails if the resource changed since"`
}

type deleteSubmodelInput struct {
	SubmodelID string `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	ETag       string `json:"etag,omitempty" jsonschema:"ETag returned by a previous read; the write fails if the resource changed since"`
}

type deleteElementInput struct {
	SubmodelID  string `json:"submodelId" jsonschema:"Identifier (id) of the submodel, not encoded"`
	IDShortPath string `json:"idShortPath" jsonschema:"idShortPath of the element to delete"`
	ETag        string `json:"etag,omitempty" jsonschema:"ETag returned by a previous read; the write fails if the resource changed since"`
}

func additiveAnnotations() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
}

func destructiveAnnotations() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true, OpenWorldHint: boolPtr(false)}
}

func (m *Module) addWriteTools(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "create_shell",
		Description: "Create a new Asset Administration Shell. Fails if the identifier already exists.",
		Annotations: additiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in createShellInput) (*sdk.CallToolResult, any, error) {
		return m.write(ctx, req, restCall{method: http.MethodPost, path: "/shells", body: in.Shell})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "create_submodel",
		Description: "Create a new submodel and optionally reference it from an existing shell. Fails if the identifier already exists.",
		Annotations: additiveAnnotations(),
	}, m.createSubmodel)
	sdk.AddTool(server, &sdk.Tool{
		Name:        "add_submodel_element",
		Description: "Add a new element to a submodel or to a collection or list inside it.",
		Annotations: additiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in addElementInput) (*sdk.CallToolResult, any, error) {
		return m.write(ctx, req, restCall{method: http.MethodPost, path: elementPath(in.SubmodelID, in.ParentPath), body: in.Element, ifMatch: in.ETag})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "set_submodel_element_value",
		Description: "Replace the value of an existing submodel element using its value-only representation.",
		Annotations: destructiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in setValueInput) (*sdk.CallToolResult, any, error) {
		path := withValueOnly(elementPath(in.SubmodelID, in.IDShortPath), true)
		return m.write(ctx, req, restCall{method: http.MethodPatch, path: path, body: in.Value, ifMatch: in.ETag})
	})
	m.addDeleteTools(server)
}

func (m *Module) addDeleteTools(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "delete_shell",
		Description: "Delete an Asset Administration Shell. Its submodels are not deleted.",
		Annotations: destructiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in deleteShellInput) (*sdk.CallToolResult, any, error) {
		return m.write(ctx, req, restCall{method: http.MethodDelete, path: shellPath(in.ShellID), ifMatch: in.ETag})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "delete_submodel",
		Description: "Delete a submodel with all its elements.",
		Annotations: destructiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in deleteSubmodelInput) (*sdk.CallToolResult, any, error) {
		return m.write(ctx, req, restCall{method: http.MethodDelete, path: submodelPath(in.SubmodelID), ifMatch: in.ETag})
	})
	sdk.AddTool(server, &sdk.Tool{
		Name:        "delete_submodel_element",
		Description: "Delete one submodel element, including all elements nested in it.",
		Annotations: destructiveAnnotations(),
	}, func(ctx context.Context, req *sdk.CallToolRequest, in deleteElementInput) (*sdk.CallToolResult, any, error) {
		return m.write(ctx, req, restCall{method: http.MethodDelete, path: elementPath(in.SubmodelID, in.IDShortPath), ifMatch: in.ETag})
	})
}

func (m *Module) createSubmodel(ctx context.Context, req *sdk.CallToolRequest, in createSubmodelInput) (*sdk.CallToolResult, any, error) {
	_, created, err := m.write(ctx, req, restCall{method: http.MethodPost, path: "/submodels", body: in.Submodel})
	if err != nil || in.ShellID == "" {
		return nil, created, err
	}
	submodelID, _ := in.Submodel["id"].(string)
	reference := map[string]any{
		"type": "ModelReference",
		"keys": []map[string]string{{"type": "Submodel", "value": submodelID}},
	}
	_, _, err = m.write(ctx, req, restCall{method: http.MethodPost, path: shellPath(in.ShellID) + "/submodel-refs", body: reference, ifMatch: in.ShellETag})
	if err != nil {
		return nil, nil, partialSubmodelCreation(err)
	}
	return nil, created, nil
}

// write dispatches a change; its result reports success even when the response body is too large to return.
func (m *Module) write(ctx context.Context, req *sdk.CallToolRequest, call restCall) (*sdk.CallToolResult, any, error) {
	call.mutation = true
	return m.call(ctx, req, call)
}

func partialSubmodelCreation(err error) error {
	linkErr := &restError{code: "MCP-CREATESUBMODEL-ADDREFERENCE", message: err.Error()}
	var restErr *restError
	if errors.As(err, &restErr) {
		linkErr.status = restErr.status
	}
	linkErr.message = "the submodel was created but the shell reference could not be added: " + linkErr.message
	linkErr.hint = "Delete the new submodel with delete_submodel, then call create_submodel again, with a current shellEtag from get_shell if needed"
	return linkErr
}
