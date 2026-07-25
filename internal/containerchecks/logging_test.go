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
		if identifier.Name == "slog" && (selector.Sel.Name == "Error" || selector.Sel.Name == "ErrorContext") && !hasStringArgument(call, "error.code") {
			position := fileset.Position(call.Pos())
			t.Errorf("%s:%d uses slog.%s without error.code", path, position.Line, selector.Sel.Name)
		}
		return true
	})
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
