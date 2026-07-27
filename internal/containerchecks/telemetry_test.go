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

package containerchecks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

func TestEveryHTTPCommandConfiguresTelemetry(t *testing.T) {
	serviceNames := []string{
		"aasenvironmentservice",
		"aasregistryservice",
		"aasrepositoryservice",
		"aasxfileserverservice",
		"companylookupservice",
		"conceptdescriptionrepositoryservice",
		"digitaltwinregistryservice",
		"discoveryservice",
		"dppapiservice",
		"submodelregistryservice",
		"submodelrepositoryservice",
	}
	for _, serviceName := range serviceNames {
		t.Run(serviceName, func(t *testing.T) {
			path := filepath.Join("..", "..", "cmd", serviceName, "main.go")
			file := parseTelemetryFile(t, path)
			if !callsTelemetryConfigure(file, serviceName) {
				t.Fatalf("%s does not configure telemetry with service name %q", path, serviceName)
			}
		})
	}
}

func TestHTTPTracingMiddlewareRemainsCentralized(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "common", "http_server.go")
	file := parseTelemetryFile(t, path)
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, _ := call.Fun.(*ast.SelectorExpr)
		identifier, identifierOK := selectorIdentifier(selector)
		if identifierOK && identifier.Name == "telemetry" && selector.Sel.Name == "HTTPMiddleware" {
			count++
		}
		return true
	})
	if count != 1 {
		t.Fatalf("%s must install exactly one shared telemetry middleware, got %d", path, count)
	}
}

func callsTelemetryConfigure(file *ast.File, serviceName string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, _ := call.Fun.(*ast.SelectorExpr)
		identifier, identifierOK := selectorIdentifier(selector)
		if !identifierOK || identifier.Name != "telemetry" || selector.Sel.Name != "Configure" || len(call.Args) != 2 {
			return true
		}
		name, ok := telemetryStringArgument(call.Args[1])
		found = ok && name == serviceName
		return !found
	})
	return found
}

func selectorIdentifier(selector *ast.SelectorExpr) (*ast.Ident, bool) {
	if selector == nil {
		return nil, false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return identifier, ok
}

func telemetryStringArgument(argument ast.Expr) (string, bool) {
	literal, ok := argument.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func parseTelemetryFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}
