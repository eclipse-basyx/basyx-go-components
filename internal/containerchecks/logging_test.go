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
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServiceCodeDoesNotUseLegacyStandardLogger(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	for _, directory := range []string{"cmd", "internal", "pkg"} {
		root := filepath.Join(repositoryRoot, directory)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			assertNoLegacyLogger(t, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func assertNoLegacyLogger(t *testing.T, path string) {
	t.Helper()
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, path, nil, 0)
	if err != nil {
		t.Errorf("parse %s: %v", path, err)
		return
	}
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			t.Errorf("parse import in %s: %v", path, err)
			continue
		}
		if importPath == "log" {
			position := fileset.Position(importSpec.Pos())
			t.Errorf("%s:%d imports the legacy standard log package; use log/slog", path, position.Line)
		}
	}
	if filepath.Base(path) == "logger.go" && strings.Contains(path, string(filepath.Separator)+"pkg"+string(filepath.Separator)) {
		assertGeneratedLoggerIsNoOp(t, fileset, file, path)
	}

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if identifier.Name == "log" {
			switch selector.Sel.Name {
			case "Print", "Printf", "Println", "Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln":
				position := fileset.Position(call.Pos())
				t.Errorf("%s:%d uses log.%s; use log/slog", path, position.Line, selector.Sel.Name)
			}
		}
		if identifier.Name == "middleware" && selector.Sel.Name == "Logger" {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d uses Chi's independent stdout logger", path, position.Line)
		}
		if identifier.Name == "slog" && (selector.Sel.Name == "Error" || selector.Sel.Name == "ErrorContext") && !hasStringArgument(call, "error.code") {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d uses slog.%s without error.code", path, position.Line, selector.Sel.Name)
		}
		if identifier.Name == "slog" && containsSensitiveLogExpression(call) {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d includes request data or SQL in a slog call", path, position.Line)
		}
		if identifier.Name == "slog" && containsFormattedLogMessage(call) {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d formats dynamic values into a slog message; use attributes", path, position.Line)
		}
		if identifier.Name == "slog" && containsTemplateLogMessage(selector.Sel.Name, call) {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d contains template markers in a slog message; use attributes", path, position.Line)
		}
		if identifier.Name == "slog" && hasGenericLogErrorCode(call) {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d uses a generic -LOG error code", path, position.Line)
		}
		if identifier.Name == "slog" &&
			hasStringArgument(call, "HTTP request completed") &&
			!strings.HasSuffix(path, filepath.Join("internal", "common", "logging", "http_middleware.go")) {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d emits an independent HTTP access event", path, position.Line)
		}
		return true
	})
}

func assertGeneratedLoggerIsNoOp(t *testing.T, fileset *token.FileSet, file *ast.File, path string) {
	t.Helper()
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Logger" {
			continue
		}
		if generatedLoggerReturnsHandler(function) {
			return
		}
		position := fileset.Position(function.Pos())
		t.Errorf("%s:%d generated Logger must remain a compatibility no-op", path, position.Line)
	}
}

func generatedLoggerReturnsHandler(function *ast.FuncDecl) bool {
	if function.Type.Params == nil ||
		len(function.Type.Params.List) == 0 ||
		len(function.Type.Params.List[0].Names) == 0 ||
		function.Body == nil ||
		len(function.Body.List) != 1 {
		return false
	}
	result, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 {
		return false
	}
	identifier, ok := result.Results[0].(*ast.Ident)
	return ok && identifier.Name == function.Type.Params.List[0].Names[0].Name
}

func containsSensitiveLogExpression(call *ast.CallExpr) bool {
	sensitive := false
	ast.Inspect(call, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			if value.Sel.Name == "RawQuery" {
				sensitive = true
			}
		case *ast.BasicLit:
			if value.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(value.Value)
			if err == nil && (strings.Contains(text, "%+v") || text == "query") {
				sensitive = true
			}
		}
		return !sensitive
	})
	return sensitive
}

func containsFormattedLogMessage(call *ast.CallExpr) bool {
	formatted := false
	ast.Inspect(call, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if ok && identifier.Name == "fmt" && (selector.Sel.Name == "Sprintf" || selector.Sel.Name == "Sprint") {
			formatted = true
			return false
		}
		return true
	})
	return formatted
}

func containsTemplateLogMessage(method string, call *ast.CallExpr) bool {
	messageIndex := 0
	if strings.HasSuffix(method, "Context") {
		messageIndex = 1
	}
	if messageIndex >= len(call.Args) {
		return false
	}
	message, ok := stringArgument(call.Args[messageIndex])
	return ok && strings.Contains(message, "{") && strings.Contains(message, "}")
}

func hasGenericLogErrorCode(call *ast.CallExpr) bool {
	for index, argument := range call.Args {
		value, ok := stringArgument(argument)
		if !ok || value != "error.code" || index+1 >= len(call.Args) {
			continue
		}
		code, ok := stringArgument(call.Args[index+1])
		return ok && strings.HasSuffix(code, "-LOG")
	}
	return false
}

func stringArgument(argument ast.Expr) (string, bool) {
	literal, ok := argument.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func hasStringArgument(call *ast.CallExpr, expected string) bool {
	for _, argument := range call.Args {
		literal, ok := argument.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && value == expected {
			return true
		}
	}
	return false
}
