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
	}
	assertBooleanFormulaPointer(t, qf.Formula, false)
	assertFormulaByRightBoolean(t, qf, grammar.RightsEnumCREATE, false)
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
	}
	if qf.FormulasByRight == nil {
		t.Fatalf("expected FormulasByRight map")
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
