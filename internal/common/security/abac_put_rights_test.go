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

package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	"github.com/go-chi/chi/v5"
)

func TestAuthorizeWithFilter_PutRightsAlternativesOrAndFormulasByRight(t *testing.T) {
	t.Parallel()

	createModel := mustParsePUTAccessModelWithSingleRight(t, grammar.RightsEnumCREATE)
	ok, reason, qf := createModel.AuthorizeWithFilter(EvalInput{
		Method: "PUT",
		Path:   "/shell-descriptors/abc",
		Claims: Claims{},
	})
	if !ok || reason != DecisionAllow {
		t.Fatalf("expected CREATE-only model to allow PUT, got ok=%v reason=%s", ok, reason)
	}
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumCREATE, true)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumUPDATE, false)

	updateModel := mustParsePUTAccessModelWithSingleRight(t, grammar.RightsEnumUPDATE)
	ok, reason, qf = updateModel.AuthorizeWithFilter(EvalInput{
		Method: "PUT",
		Path:   "/shell-descriptors/abc",
		Claims: Claims{},
	})
	if !ok || reason != DecisionAllow {
		t.Fatalf("expected UPDATE-only model to allow PUT, got ok=%v reason=%s", ok, reason)
	}
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumCREATE, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumUPDATE, true)
}

func TestSelectPutFormulaByExistence_SelectsRightSpecificFormula(t *testing.T) {
	t.Parallel()

	createExpr := boolExpression(true)
	updateExpr := boolExpression(false)
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumCREATE: createExpr,
			grammar.RightsEnumUPDATE: updateExpr,
		},
	})

	createCtx := SelectPutFormulaByExistence(ctx, false)
	createQF := GetQueryFilter(createCtx)
	assertBooleanFormulaPointer(t, createQF.Formula, true)

	updateCtx := SelectPutFormulaByExistence(ctx, true)
	updateQF := GetQueryFilter(updateCtx)
	assertBooleanFormulaPointer(t, updateQF.Formula, false)
}

func TestSelectPutFormulaByExistence_DefaultsToFalseIfMissing(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{},
	})
	updateCtx := SelectPutFormulaByExistence(ctx, true)
	qf := GetQueryFilter(updateCtx)
	if qf == nil {
		t.Fatalf("expected query filter in context")
		return
	}
	assertBooleanFormulaPointer(t, qf.Formula, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumUPDATE, false)
}

func TestSelectPutFormulaByExistence_DefaultsToFalseIfMapIsNil(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{})
	createCtx := SelectPutFormulaByExistence(ctx, false)
	qf := GetQueryFilter(createCtx)
	if qf == nil {
		t.Fatalf("expected query filter in context")
		return
	}
	assertBooleanFormulaPointer(t, qf.Formula, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumCREATE, false)
}

func TestSelectPutFormulaByExistence_FailsClosedOnCloneError(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		Formula: invalidCloneFormula(),
	})
	updateCtx := SelectPutFormulaByExistence(ctx, true)
	qf := GetQueryFilter(updateCtx)
	if qf == nil {
		t.Fatalf("expected query filter in context")
		return
	}
	assertBooleanFormulaPointer(t, qf.Formula, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumUPDATE, false)
}

func TestMergeQueryFilter_FailsClosedOnCloneError(t *testing.T) {
	t.Parallel()

	queryExpr := boolExpression(true)
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		Formula: invalidCloneFormula(),
	})
	mergedCtx := MergeQueryFilter(ctx, grammar.Query{Condition: &queryExpr})
	qf := GetQueryFilter(mergedCtx)
	if qf == nil {
		t.Fatalf("expected query filter in context")
		return
	}
	assertBooleanFormulaPointer(t, qf.Formula, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumREAD, false)
}

func invalidCloneFormula() *grammar.LogicalExpression {
	expr := grammar.LogicalExpression{And: []grammar.LogicalExpression{boolExpression(true)}}
	return &expr
}

func mustParsePUTAccessModelWithSingleRight(t *testing.T, right grammar.RightsEnum) *AccessModel {
	t.Helper()

	modelJSON := fmt.Sprintf(`{
  "AllAccessPermissionRules": {
    "DEFATTRIBUTES": [
      { "name": "anonymous", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] }
    ],
    "DEFOBJECTS": [
      { "name": "put_shell", "objects": [ { "ROUTE": "/shell-descriptors/*" } ] }
    ],
    "DEFACLS": [
      { "name": "single_right", "acl": { "USEATTRIBUTES": "anonymous", "RIGHTS": ["%s"], "ACCESS": "ALLOW" } }
    ],
    "DEFFORMULAS": [
      { "name": "always_true", "formula": { "$boolean": true } }
    ],
    "rules": [
      { "USEACL": "single_right", "USEOBJECTS": ["put_shell"], "USEFORMULA": "always_true" }
    ]
  }
}`, right)

	router := chi.NewRouter()
	ctrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range ctrl.Routes() {
		router.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	model, err := ParseAccessModel([]byte(modelJSON), router, "")
	if err != nil {
		t.Fatalf("parse model failed: %v", err)
	}
	return model
}

func assertFormulaByRightBoolean(t *testing.T, qf *QueryFilter, right grammar.RightsEnum, want bool) {
	t.Helper()
	if qf == nil {
		t.Fatalf("expected query filter")
		return
	}
	if qf.FormulasByRight == nil {
		t.Fatalf("expected FormulasByRight map")
		return
	}
	expr, ok := qf.FormulasByRight[right]
	if !ok {
		t.Fatalf("expected right %q in FormulasByRight", right)
	}
	if expr.Boolean == nil {
		t.Fatalf("expected boolean expression for right %q, got %#v", right, expr)
	}
	if *expr.Boolean != want {
		t.Fatalf("expected right %q to be %v, got %v", right, want, *expr.Boolean)
	}
}

func assertBooleanFormulaPointer(t *testing.T, expr *grammar.LogicalExpression, want bool) {
	t.Helper()
	if expr == nil || expr.Boolean == nil {
		t.Fatalf("expected boolean formula pointer, got %#v", expr)
	}
	if *expr.Boolean != want {
		t.Fatalf("expected formula boolean %v, got %v", want, *expr.Boolean)
	}
}
