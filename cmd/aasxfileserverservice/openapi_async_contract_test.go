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

package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const generatedAASXAPIDirectory = "../../pkg/aasxfileserverapi/go"

type generatedContractEvidence struct {
	identifiers       map[string]struct{}
	routes            map[string]struct{}
	interfaceMethods  map[string]map[string]string
	structFields      map[string]map[string]generatedStructField
	definedTypes      map[string]string
	typedStringValues map[string]map[string]struct{}
	asyncUploadBody   string
}

type generatedStructField struct {
	typeName string
	tag      string
}

func TestOpenAPIContainsAASXFileServerSSP002Contract(t *testing.T) {
	specificationBytes, err := os.ReadFile("openapi.yaml")
	require.NoError(t, err)

	var specification map[string]any
	require.NoError(t, yaml.Unmarshal(specificationBytes, &specification))
	info := mapValue(t, specification, "info")
	require.NotContains(t, info, "x-profile-identifier")
	profileIdentifiers := stringSlice(t, info["x-profile-identifiers"])
	require.Contains(t, profileIdentifiers, "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-001")
	require.Contains(t, profileIdentifiers, "https://admin-shell.io/aas/API/3/2/AasxFileServerServiceSpecification/SSP-002")

	paths := mapValue(t, specification, "paths")

	post := mapValue(t, mapValue(t, paths, "/packages-async"), "post")
	require.Equal(t, "PostAsyncAASXPackage", post["operationId"])
	multipartContent := mapValue(t, mapValue(t, mapValue(t, post, "requestBody"), "content"), "multipart/form-data")
	multipartSchema := mapValue(t, multipartContent, "schema")
	require.Contains(t, stringSlice(t, multipartSchema["required"]), "file")
	fileEncoding := mapValue(t, mapValue(t, multipartContent, "encoding"), "file")
	require.Equal(t, "application/asset-administration-shell-package", fileEncoding["contentType"])
	accepted := mapValue(t, mapValue(t, post, "responses"), "202")
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/OperationHandle", responseSchemaRef(t, accepted))
	require.Contains(t, mapValue(t, accepted, "headers"), "Location")

	status := mapValue(t, mapValue(t, paths, "/packages-async/status/{handleId}"), "get")
	require.Equal(t, "GetAasxAsyncStatus", status["operationId"])
	statusResponses := mapValue(t, status, "responses")
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/BaseOperationResult", responseSchemaRef(t, mapValue(t, statusResponses, "200")))
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/responses/found", mapValue(t, statusResponses, "302")["$ref"])

	result := mapValue(t, mapValue(t, paths, "/packages-async/result/{handleId}"), "get")
	require.Equal(t, "GetAasxAsyncResult", result["operationId"])
	require.Equal(t, "../Part2-API-Schemas/openapi.yaml#/components/schemas/BaseOperationResult", responseSchemaRef(t, mapValue(t, mapValue(t, result, "responses"), "200")))

	description := mapValue(t, mapValue(t, paths, "/description"), "get")
	descriptionResponse := mapValue(t, mapValue(t, description, "responses"), "200")
	descriptionExample := mapValue(t, mapValue(t, mapValue(t, descriptionResponse, "content"), "application/json"), "example")
	require.ElementsMatch(t, profileIdentifiers, stringSlice(t, descriptionExample["profiles"]))
}

func TestGeneratedAASXAPIContainsSSP002Contract(t *testing.T) {
	evidence := readGeneratedContractEvidence(t)
	for _, identifier := range []string{
		"PostAsyncAASXPackage",
		"GetAasxAsyncStatus",
		"GetAasxAsyncResult",
		"OperationHandle",
		"BaseOperationResult",
	} {
		_, found := evidence.identifiers[identifier]
		require.Truef(t, found, "generated AASX API is missing %s", identifier)
	}
	for _, route := range []string{
		"/packages-async",
		"/packages-async/status/{handleId}",
		"/packages-async/result/{handleId}",
	} {
		_, found := evidence.routes[route]
		require.Truef(t, found, "generated AASX API is missing route %s", route)
	}
	require.Equal(t, map[string]string{
		"PostAsyncAASXPackage": "func(http.ResponseWriter, *http.Request)",
	}, evidence.interfaceMethods["AASXAsyncFileServerAPIAPIRouter"])
	require.Equal(t, map[string]string{
		"PostAsyncAASXPackage": "func(context.Context, StagedUpload, []string, string) (ImplResponse, error)",
	}, evidence.interfaceMethods["AASXAsyncFileServerAPIAPIServicer"])
	require.Equal(t, map[string]string{
		"GetAasxAsyncStatus": "func(http.ResponseWriter, *http.Request)",
	}, evidence.interfaceMethods["AASXAsyncFileServerStatusAPIAPIRouter"])
	require.Equal(t, map[string]string{
		"GetAasxAsyncStatus": "func(context.Context, string) (ImplResponse, error)",
	}, evidence.interfaceMethods["AASXAsyncFileServerStatusAPIAPIServicer"])
	require.Equal(t, map[string]string{
		"GetAasxAsyncResult": "func(http.ResponseWriter, *http.Request)",
	}, evidence.interfaceMethods["AASXAsyncFileServerResultAPIAPIRouter"])
	require.Equal(t, map[string]string{
		"GetAasxAsyncResult": "func(context.Context, string) (ImplResponse, error)",
	}, evidence.interfaceMethods["AASXAsyncFileServerResultAPIAPIServicer"])
	require.Equal(t, map[string]generatedStructField{
		"HandleId": {typeName: "string", tag: `json:"handleId,omitempty"`},
	}, evidence.structFields["OperationHandle"])
	require.Equal(t, map[string]generatedStructField{
		"Messages":       {typeName: "[]map[string]interface{}", tag: `json:"messages,omitempty"`},
		"ExecutionState": {typeName: "ExecutionState", tag: `json:"executionState,omitempty"`},
		"Success":        {typeName: "bool", tag: `json:"success,omitempty"`},
	}, evidence.structFields["BaseOperationResult"])
	require.Equal(t, "string", evidence.definedTypes["ExecutionState"])
	require.Equal(t, map[string]struct{}{
		"Initiated": {},
		"Running":   {},
		"Completed": {},
		"Canceled":  {},
		"Failed":    {},
		"Timeout":   {},
	}, evidence.typedStringValues["ExecutionState"])
	require.NotEmpty(t, evidence.asyncUploadBody, "generated AASX API is missing the PostAsyncAASXPackage controller")
	require.True(
		t,
		strings.Contains(evidence.asyncUploadBody, "readPackageUpload") || strings.Contains(evidence.asyncUploadBody, "ReadMultipartUpload"),
		"PostAsyncAASXPackage must use the shared streaming multipart stager",
	)
	require.Contains(t, evidence.asyncUploadBody, "upload.File", "PostAsyncAASXPackage must hand only the staged upload to its service")
	require.Contains(t, evidence.asyncUploadBody, "uploadFileName(upload)", "PostAsyncAASXPackage must forward the multipart source filename to its service")
	for _, forbidden := range []string{"io.ReadAll", "ParseMultipartForm", "ReadForm("} {
		require.NotContainsf(t, evidence.asyncUploadBody, forbidden, "PostAsyncAASXPackage must not buffer multipart input with %s", forbidden)
	}
}

func TestAASXAsyncUploadPathDoesNotUseFullBufferAPIs(t *testing.T) {
	forbidden := map[string]struct{}{
		"bytes.Buffer":    {},
		"bytes.NewBuffer": {},
		"bytes.NewReader": {},
		"io.ReadAll":      {},
		"os.ReadFile":     {},
	}
	violations := make([]string, 0)
	asyncImplementationFound := false
	for _, root := range []string{generatedAASXAPIDirectory, "../../internal/aasxfileserver"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			require.NoError(t, walkErr)
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			standardLibraryImports := make(map[string]string)
			for _, imported := range file.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				require.NoError(t, err)
				if importPath != "bytes" && importPath != "io" && importPath != "os" {
					continue
				}
				alias := importPath
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				standardLibraryImports[alias] = importPath
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if declaration, ok := node.(*ast.FuncDecl); ok && declaration.Name.Name == "PostAsyncAASXPackage" && root != generatedAASXAPIDirectory {
					asyncImplementationFound = true
				}
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				packageName, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				importPath, imported := standardLibraryImports[packageName.Name]
				if !imported {
					return true
				}
				name := importPath + "." + selector.Sel.Name
				if _, blocked := forbidden[name]; blocked {
					violations = append(violations, fileSet.Position(selector.Pos()).String()+": "+name)
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}
	require.True(t, asyncImplementationFound, "handwritten async upload implementation is missing")
	require.Empty(t, violations, "async AASX production code must not use complete-file buffer APIs")
}

func readGeneratedContractEvidence(t *testing.T) generatedContractEvidence {
	t.Helper()
	evidence := generatedContractEvidence{
		identifiers:       make(map[string]struct{}),
		routes:            make(map[string]struct{}),
		interfaceMethods:  make(map[string]map[string]string),
		structFields:      make(map[string]map[string]generatedStructField),
		definedTypes:      make(map[string]string),
		typedStringValues: make(map[string]map[string]struct{}),
	}
	entries, err := os.ReadDir(generatedAASXAPIDirectory)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		collectGeneratedContractEvidence(t, filepath.Join(generatedAASXAPIDirectory, entry.Name()), &evidence)
	}
	return evidence
}

func collectGeneratedContractEvidence(t *testing.T, path string, evidence *generatedContractEvidence) {
	t.Helper()
	//nolint:gosec // Paths come only from the fixed generated API directory enumerated by this test.
	source, err := os.ReadFile(path)
	require.NoError(t, err)
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
	require.NoError(t, err)
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			evidence.identifiers[value.Name] = struct{}{}
		case *ast.BasicLit:
			collectGeneratedRoute(value, evidence.routes)
		case *ast.FuncDecl:
			if value.Name.Name == "PostAsyncAASXPackage" && value.Body != nil {
				start := fileSet.Position(value.Body.Pos()).Offset
				end := fileSet.Position(value.Body.End()).Offset
				evidence.asyncUploadBody = string(source[start:end])
			}
		case *ast.TypeSpec:
			collectGeneratedTypeEvidence(t, fileSet, value, evidence)
		case *ast.ValueSpec:
			collectGeneratedTypedStringValues(t, fileSet, value, evidence)
		}
		return true
	})
}

func collectGeneratedTypeEvidence(t *testing.T, fileSet *token.FileSet, specification *ast.TypeSpec, evidence *generatedContractEvidence) {
	t.Helper()
	switch definition := specification.Type.(type) {
	case *ast.InterfaceType:
		methods := make(map[string]string)
		for _, field := range definition.Methods.List {
			if len(field.Names) != 1 {
				continue
			}
			methods[field.Names[0].Name] = formatNode(t, fileSet, field.Type)
		}
		evidence.interfaceMethods[specification.Name.Name] = methods
	case *ast.StructType:
		fields := make(map[string]generatedStructField)
		for _, field := range definition.Fields.List {
			if len(field.Names) != 1 {
				continue
			}
			tag := ""
			if field.Tag != nil {
				unquoted, err := strconv.Unquote(field.Tag.Value)
				require.NoError(t, err)
				tag = unquoted
			}
			fields[field.Names[0].Name] = generatedStructField{typeName: formatNode(t, fileSet, field.Type), tag: tag}
		}
		evidence.structFields[specification.Name.Name] = fields
	default:
		evidence.definedTypes[specification.Name.Name] = formatNode(t, fileSet, specification.Type)
	}
}

func collectGeneratedTypedStringValues(t *testing.T, fileSet *token.FileSet, specification *ast.ValueSpec, evidence *generatedContractEvidence) {
	t.Helper()
	if specification.Type == nil {
		return
	}
	typeName := formatNode(t, fileSet, specification.Type)
	for _, value := range specification.Values {
		literal, ok := value.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		unquoted, err := strconv.Unquote(literal.Value)
		require.NoError(t, err)
		if evidence.typedStringValues[typeName] == nil {
			evidence.typedStringValues[typeName] = make(map[string]struct{})
		}
		evidence.typedStringValues[typeName][unquoted] = struct{}{}
	}
}

func formatNode(t *testing.T, fileSet *token.FileSet, node ast.Node) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, format.Node(&output, fileSet, node))
	return output.String()
}

func collectGeneratedRoute(literal *ast.BasicLit, routes map[string]struct{}) {
	if literal.Kind != token.STRING {
		return
	}
	value, err := strconv.Unquote(literal.Value)
	if err == nil && strings.HasPrefix(value, "/packages-async") {
		routes[value] = struct{}{}
	}
}

func responseSchemaRef(t *testing.T, response map[string]any) string {
	t.Helper()
	content := mapValue(t, response, "content")
	applicationJSON := mapValue(t, content, "application/json")
	reference, valid := mapValue(t, applicationJSON, "schema")["$ref"].(string)
	require.True(t, valid, "OpenAPI response schema must contain a string $ref")
	return reference
}

func mapValue(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, found := parent[key]
	require.Truef(t, found, "OpenAPI field %q is missing", key)
	result, valid := value.(map[string]any)
	require.Truef(t, valid, "OpenAPI field %q has type %T, expected object", key, value)
	return result
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, valid := value.([]any)
	require.Truef(t, valid, "OpenAPI field has type %T, expected array", value)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, valid := item.(string)
		require.Truef(t, valid, "OpenAPI array item has type %T, expected string", item)
		result = append(result, text)
	}
	return result
}
