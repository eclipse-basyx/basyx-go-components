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

package common

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

type resourceAccessOperation struct{ suffix, method, summary, body string }

var resourceAccessOperations = []resourceAccessOperation{
	{"", "get", "Inspect direct access and effective resource policy", ""},
	{"/policy", "get", "Read the directly bound resource policy", ""},
	{"/policy", "put", "Replace the directly bound resource policy", "ResourceBoundPolicy"},
	{"/policy", "delete", "Remove the local policy and restore inheritance", ""},
	{"/grants", "post", "Add a managed principal grant", "ResourceAccessGrantInput"},
	{"/grants/{grantId}", "put", "Replace a managed principal grant", "ResourceAccessGrantInput"},
	{"/grants/{grantId}", "delete", "Remove a managed principal grant", ""},
	{"/managers", "put", "Replace direct managers", "ResourceAccessPrincipals"},
	{"/owners", "put", "Replace direct owners; at least one owner must remain", "ResourceAccessPrincipals"},
}

func injectResourceBoundAPI(content []byte) ([]byte, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("SWAGGER-REBAC-DECODE %w", err)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("SWAGGER-REBAC-PATHS missing paths")
	}
	roots := []string{"/shells", "/submodels", "/shells/{aasIdentifier}", "/submodels/{submodelIdentifier}", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}"}
	for _, root := range roots {
		if _, exists := paths[root]; !exists {
			continue
		}
		for _, operation := range resourceAccessOperations {
			path := root + "/$access" + operation.suffix
			item, _ := paths[path].(map[string]any)
			if item == nil {
				item = map[string]any{}
			}
			item[operation.method] = resourceAccessOpenAPIOperation(path, operation)
			paths[path] = item
		}
	}
	components, _ := document["components"].(map[string]any)
	if components == nil {
		components = map[string]any{}
	}
	schemas, _ := components["schemas"].(map[string]any)
	if schemas == nil {
		schemas = map[string]any{}
	}
	for name, schema := range resourceAccessSchemas() {
		schemas[name] = schema
	}
	components["schemas"] = schemas
	document["components"] = components
	result, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("SWAGGER-REBAC-ENCODE %w", err)
	}
	return result, nil
}

func resourceAccessOpenAPIOperation(path string, operation resourceAccessOperation) map[string]any {
	parameters := []any{}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parameters = append(parameters, map[string]any{"name": strings.Trim(part, "{}"), "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
		}
	}
	if operation.method != "get" {
		parameters = append(parameters, map[string]any{"name": "If-Match", "in": "header", "required": true, "schema": map[string]any{"type": "string"}, "description": "ETag from this resource's access overview."})
	}
	responses := map[string]any{}
	for code, description := range map[string]string{"200": "Access state or updated value", "201": "Managed grant created", "204": "Removed", "400": "Invalid policy or principals", "401": "Authentication required", "403": "Access administration denied", "404": "Resource, local policy or grant not found", "409": "Explicit local policy required or protected manager rule changed", "412": "Stale access revision", "428": "If-Match required"} {
		responses[code] = map[string]any{"description": description}
	}
	responseSchema := "ResourceAccessOverview"
	switch operation.suffix {
	case "/policy":
		responseSchema = "ResourceBoundPolicy"
	case "/managers", "/owners":
		responseSchema = "ResourceAccessPrincipals"
	case "/grants", "/grants/{grantId}":
		responseSchema = "ResourceAccessGrant"
	}
	response := map[string]any{"description": "Result", "headers": map[string]any{"ETag": map[string]any{"schema": map[string]any{"type": "string"}}}, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + responseSchema}}}}
	responses["200"] = response
	if operation.method == "post" {
		responses["201"] = response
	}

	result := map[string]any{"tags": []string{"Resource Access"}, "summary": operation.summary, "parameters": parameters, "responses": responses, "description": "BaSyx resource-bound access administration. Requires direct ownership or a manager assignment from the effective policy. Ordinary data rights do not authorize policy administration."}
	if operation.body != "" {
		result["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + operation.body}}}}
	}
	return result
}

func resourceAccessSchemas() map[string]any {
	principal := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"issuer", "subject"}, "properties": map[string]any{"issuer": map[string]any{"type": "string", "minLength": 1}, "subject": map[string]any{"type": "string", "minLength": 1}}}
	rights := map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "enum": []string{"CREATE", "READ", "UPDATE", "DELETE", "EXECUTE", "VIEW", "ALL"}}}
	return map[string]any{
		"ResourceAccessOverview": map[string]any{"type": "object", "required": []string{"revision", "resource", "localPolicy", "owners", "managers", "grants", "effectivePolicy"}, "properties": map[string]any{
			"revision":        map[string]any{"type": "integer", "format": "int64"},
			"resource":        map[string]any{"type": "object"},
			"localPolicy":     map[string]any{"nullable": true, "allOf": []any{map[string]any{"$ref": "#/components/schemas/ResourceBoundPolicy"}}},
			"effectivePolicy": map[string]any{"nullable": true, "description": "RESOURCE identifies the inheritance origin.", "allOf": []any{map[string]any{"$ref": "#/components/schemas/ResourceBoundPolicy"}}},
			"owners":          map[string]any{"$ref": "#/components/schemas/ResourceAccessPrincipals"},
			"managers":        map[string]any{"$ref": "#/components/schemas/ResourceAccessPrincipals"},
			"grants":          map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ResourceAccessGrant"}},
		}},
		"ResourceAccessGrant":      map[string]any{"type": "object", "required": []string{"id", "principal", "rights"}, "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "principal": map[string]any{"$ref": "#/components/schemas/ResourceAccessPrincipal"}, "rights": rights}},
		"ResourceAccessPrincipal":  principal,
		"ResourceAccessPrincipals": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"$ref": "#/components/schemas/ResourceAccessPrincipal"}},
		"ResourceAccessGrantInput": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"principal", "rights"}, "example": map[string]any{"principal": map[string]string{"issuer": "https://issuer.example", "subject": "bridge-inspector"}, "rights": []string{"READ"}}, "properties": map[string]any{
			"principal": map[string]any{"$ref": "#/components/schemas/ResourceAccessPrincipal"},
			"rights":    rights,
		}},
		"ResourceBoundPolicy": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"RESOURCE", "rules"}, "example": map[string]any{"RESOURCE": map[string]string{"IDENTIFIABLE": "$sm(\"urn:bridge:inspection\")"}, "rules": []any{}}, "description": "Single resource-bound model from Part 4 PR #108 (07c8bb6). RESOURCE must identify the addressed resource. Definitions are local; rules prohibit OBJECTS and USEOBJECTS.", "properties": map[string]any{
			"RESOURCE":      map[string]any{"type": "object", "minProperties": 1, "maxProperties": 1, "additionalProperties": false, "properties": map[string]any{"ROUTE": map[string]any{"type": "string"}, "IDENTIFIABLE": map[string]any{"type": "string"}, "REFERABLE": map[string]any{"type": "string"}}},
			"rules":         map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"DEFATTRIBUTES": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"DEFACLS":       map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"DEFFORMULAS":   map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		}},
	}
}
